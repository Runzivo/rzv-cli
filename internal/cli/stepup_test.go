package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

const stepUpChallengeID = "0b7c3f5e-6a51-4c1e-9f0e-2d1f6c7a8b90"

// stepUpServer is a fake step-up service in front of one gated route. The
// gated route answers reauth_required until a code is verified (or always,
// with alwaysReauth).
type stepUpServer struct {
	mu           sync.Mutex
	gated        int
	availability int
	sends        int
	verifies     int
	codes        []string
	categories   []string
	bodies       []string
	verified     bool
	alwaysReauth bool
	// verifyAnswers are the verify responses in order, as "status body";
	// after them every right code (123456) succeeds.
	verifyAnswers []string
}

func (s *stepUpServer) install(h *harness, route, okBody, availabilityBody string, availabilityStatus int) {
	h.routes[route] = func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.gated++
		s.bodies = append(s.bodies, string(body))
		pass := s.verified && !s.alwaysReauth
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !pass {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"error":{"code":"reauth_required"}}`))
			return
		}
		_, _ = w.Write([]byte(okBody))
	}
	h.routes["GET /v1/customer/step-up/email-code"] = func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.availability++
		s.categories = append(s.categories, r.URL.Query().Get("category"))
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(availabilityStatus)
		_, _ = w.Write([]byte(availabilityBody))
	}
	h.routes["POST /v1/customer/step-up/email-code"] = func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		s.mu.Lock()
		s.sends++
		s.categories = append(s.categories, in["category"])
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		_, _ = w.Write([]byte(`{"challenge_id":"` + stepUpChallengeID + `","category":"` + in["category"] + `","masked_email":"d…@example.test","expires_at":"2026-11-18T10:10:00Z","resend_after":"2026-11-18T10:01:00Z"}`))
	}
	h.routes["POST /v1/customer/step-up/email-code/verify"] = func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.verifies++
		s.codes = append(s.codes, in["code"])
		w.Header().Set("Content-Type", "application/json")
		if in["challenge_id"] != stepUpChallengeID {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"code":"invalid_request"}}`))
			return
		}
		if len(s.verifyAnswers) > 0 {
			answer := s.verifyAnswers[0]
			s.verifyAnswers = s.verifyAnswers[1:]
			status, body, _ := strings.Cut(answer, " ")
			code := map[string]int{"400": 400, "410": 410, "429": 429, "403": 403}[status]
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
			return
		}
		if in["code"] != "123456" {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"code":"step_up_code_invalid"}}`))
			return
		}
		s.verified = true
		_, _ = w.Write([]byte(`{"category":"billing","verified_at":"2026-11-18T10:02:00Z","step_up_expires_at":"2026-11-18T10:17:00Z"}`))
	}
}

const availableJSON = `{"available":true,"code_only":true,"masked_email":"d…@example.test"}`
const countryRoute = "PUT /v1/customer/tenants/t-1/billing/country"

func stepUpHarness(t *testing.T, input string) *harness {
	t.Helper()
	h := selectedHarness(t)
	h.tty = true
	h.stdin = strings.NewReader(input)
	return h
}

func noCode(t *testing.T, out, errOut string, codes ...string) {
	t.Helper()
	for _, code := range codes {
		if code != "" && (strings.Contains(out, code) || strings.Contains(errOut, code)) {
			t.Fatalf("the code %s was printed: %q %q", code, out, errOut)
		}
	}
}

func TestStepUpHappyPathRetriesOnce(t *testing.T) {
	h := stepUpHarness(t, "123456\n")
	s := &stepUpServer{}
	s.install(h, countryRoute, `{"billing_country":"US"}`, availableJSON, 200)
	code, out, errOut := h.run("billing", "country", "set", "US")
	if code != 0 || !strings.Contains(out, "US") {
		t.Fatalf("country set: %d %q %q", code, out, errOut)
	}
	if s.gated != 2 || s.availability != 1 || s.sends != 1 || s.verifies != 1 {
		t.Fatalf("calls: gated %d availability %d send %d verify %d", s.gated, s.availability, s.sends, s.verifies)
	}
	if s.bodies[0] != s.bodies[1] {
		t.Fatalf("the retry changed the request: %q vs %q", s.bodies[0], s.bodies[1])
	}
	for _, category := range s.categories {
		if category != "billing" {
			t.Fatalf("category = %q; want billing", category)
		}
	}
	if !strings.Contains(errOut, "d…@example.test") || !strings.Contains(errOut, "6-digit code") {
		t.Fatalf("prompt = %q", errOut)
	}
	noCode(t, out, errOut, "123456")
}

func TestStepUpRetryKeepsIdempotencyKey(t *testing.T) {
	h := stepUpHarness(t, "123456\n")
	s := &stepUpServer{}
	s.install(h, "POST /v1/customer/tenants/t-1/api-keys", `{"api_key":{"id":"k-1","name":"ci","permissions":["jobs:read"]},"secret":"rzv_secret"}`, availableJSON, 200)
	code, _, errOut := h.run("api-keys", "create", "--name", "ci", "--permissions", "jobs:read", "--idempotency-key", "idem-key-1")
	if code != 0 {
		t.Fatalf("create: %d %q", code, errOut)
	}
	if s.gated != 2 || !strings.Contains(s.bodies[1], `"idempotency_key":"idem-key-1"`) || s.bodies[0] != s.bodies[1] {
		t.Fatalf("retry bodies = %q", s.bodies)
	}
	if s.categories[0] != "api_keys" {
		t.Fatalf("category = %q; want api_keys", s.categories[0])
	}
}

func TestStepUpAccountSecurityCategory(t *testing.T) {
	h := stepUpHarness(t, "123456\n")
	s := &stepUpServer{}
	s.install(h, "DELETE /v1/customer/tenants/t-1/teams/team-1", ``, availableJSON, 200)
	if code, _, errOut := h.run("teams", "delete", "team-1"); code != 0 {
		t.Fatalf("teams delete: %d %q", code, errOut)
	}
	if s.categories[0] != "account_security" || s.gated != 2 {
		t.Fatalf("category %q gated %d", s.categories[0], s.gated)
	}
}

func TestStepUpWrongCodeThenRightCode(t *testing.T) {
	h := stepUpHarness(t, "111111\n12ab\n123456\n")
	s := &stepUpServer{}
	s.install(h, countryRoute, `{"billing_country":"US"}`, availableJSON, 200)
	code, out, errOut := h.run("billing", "country", "set", "US")
	if code != 0 {
		t.Fatalf("country set: %d %q %q", code, out, errOut)
	}
	if s.verifies != 2 || s.gated != 2 || s.sends != 1 {
		t.Fatalf("verify %d gated %d send %d; the malformed code must not be sent", s.verifies, s.gated, s.sends)
	}
	if !strings.Contains(errOut, "That code is not right") || !strings.Contains(errOut, "The code is 6 digits") {
		t.Fatalf("stderr = %q", errOut)
	}
	noCode(t, out, errOut, "111111", "123456", "12ab")
}

func TestStepUpAttemptsExceeded(t *testing.T) {
	h := stepUpHarness(t, "111111\n222222\n123456\n")
	s := &stepUpServer{verifyAnswers: []string{
		`400 {"error":{"code":"step_up_code_invalid"}}`,
		`429 {"error":{"code":"step_up_attempts_exceeded"}}`,
	}}
	s.install(h, countryRoute, `{"billing_country":"US"}`, availableJSON, 200)
	code, out, errOut := h.run("billing", "country", "set", "US")
	if code != exitAuth || !strings.Contains(errOut, "Too many wrong codes") {
		t.Fatalf("attempts exceeded: %d %q", code, errOut)
	}
	if s.gated != 1 || s.verifies != 2 {
		t.Fatalf("gated %d verify %d; want no retry and no third code", s.gated, s.verifies)
	}
	noCode(t, out, errOut, "111111", "222222")
}

func TestStepUpExpiredCodeOffersOneNewCode(t *testing.T) {
	h := stepUpHarness(t, "111111\ny\n123456\n")
	s := &stepUpServer{verifyAnswers: []string{`410 {"error":{"code":"step_up_code_expired"}}`}}
	s.install(h, countryRoute, `{"billing_country":"US"}`, availableJSON, 200)
	code, out, errOut := h.run("billing", "country", "set", "US")
	if code != 0 || s.sends != 2 || s.gated != 2 || !strings.Contains(errOut, "Send a new code?") {
		t.Fatalf("resend: %d sends %d gated %d %q %q", code, s.sends, s.gated, out, errOut)
	}

	h = stepUpHarness(t, "111111\ny\n222222\n")
	s = &stepUpServer{verifyAnswers: []string{`410 {"error":{"code":"step_up_code_expired"}}`, `410 {"error":{"code":"step_up_code_expired"}}`}}
	s.install(h, countryRoute, `{"billing_country":"US"}`, availableJSON, 200)
	code, _, errOut = h.run("billing", "country", "set", "US")
	if code != exitAuth || s.sends != 2 || !strings.Contains(errOut, "The code expired. Run the command again") {
		t.Fatalf("second expiry: %d sends %d %q", code, s.sends, errOut)
	}
}

func TestStepUpNoVerifiedEmail(t *testing.T) {
	h := stepUpHarness(t, "123456\n")
	s := &stepUpServer{}
	s.install(h, countryRoute, `{}`, `{"available":false,"code_only":true,"reason":"no_verified_email"}`, 200)
	code, _, errOut := h.run("billing", "country", "set", "US")
	if code != exitAuth || !strings.Contains(errOut, "Sign-in methods") || strings.Contains(errOut, "6-digit") {
		t.Fatalf("no verified email: %d %q", code, errOut)
	}
	if s.sends != 0 || s.gated != 1 {
		t.Fatalf("sends %d gated %d", s.sends, s.gated)
	}
}

func TestStepUpNotAvailableUsesGeneralAdvice(t *testing.T) {
	h := stepUpHarness(t, "123456\n")
	s := &stepUpServer{}
	s.install(h, countryRoute, `{}`, `{"available":false,"code_only":false}`, 200)
	code, _, errOut := h.run("billing", "country", "set", "US")
	if code != exitAuth || !strings.Contains(errOut, "run `rzv login`, then retry") || s.sends != 0 {
		t.Fatalf("not available: %d sends %d %q", code, s.sends, errOut)
	}
}

func TestStepUpOlderServiceFallsBack(t *testing.T) {
	for _, status := range []int{404, 503} {
		h := stepUpHarness(t, "123456\n")
		s := &stepUpServer{}
		s.install(h, countryRoute, `{}`, `{"error":{"code":"not_found"}}`, status)
		code, _, errOut := h.run("billing", "country", "set", "US")
		if code != exitAuth || !strings.Contains(errOut, "do it in the Runzivo portal") || strings.Contains(errOut, "6-digit") {
			t.Fatalf("availability %d: %d %q", status, code, errOut)
		}
		if s.sends != 0 || s.gated != 1 {
			t.Fatalf("availability %d: sends %d gated %d", status, s.sends, s.gated)
		}
	}
}

func TestStepUpNotInATerminal(t *testing.T) {
	h := stepUpHarness(t, "123456\n")
	h.tty = false
	s := &stepUpServer{}
	s.install(h, countryRoute, `{}`, availableJSON, 200)
	code, _, errOut := h.run("billing", "country", "set", "US")
	if code != exitAuth || !strings.Contains(errOut, "run the command again in a terminal") || strings.Contains(errOut, "6-digit") {
		t.Fatalf("no TTY: %d %q", code, errOut)
	}
	if s.availability != 0 || s.sends != 0 || s.gated != 1 {
		t.Fatalf("availability %d sends %d gated %d", s.availability, s.sends, s.gated)
	}
}

func TestStepUpJSONNeverPrompts(t *testing.T) {
	h := stepUpHarness(t, "123456\n")
	s := &stepUpServer{}
	s.install(h, countryRoute, `{}`, availableJSON, 200)
	code, out, errOut := h.run("billing", "country", "set", "US", "--json")
	if code != exitAuth || out != "" || s.availability != 0 || s.sends != 0 {
		t.Fatalf("--json: %d %q %q availability %d", code, out, errOut, s.availability)
	}
	var e map[string]string
	if err := json.Unmarshal([]byte(errOut), &e); err != nil || e["error"] != "reauth_required" || !strings.Contains(e["message"], "in a terminal") {
		t.Fatalf("--json error = %q (%v)", errOut, err)
	}
}

func TestStepUpAPIKeyNeverPrompts(t *testing.T) {
	h := newHarness(t)
	h.env["RZV_API_KEY"] = "rzv_test_key"
	h.env["RZV_API_URL"] = h.api.URL
	h.tty = true
	h.stdin = strings.NewReader("123456\n")
	s := &stepUpServer{}
	s.install(h, "PUT /v1/customer/tenants/t-5/billing/country", `{}`, availableJSON, 200)
	code, _, errOut := h.run("billing", "country", "set", "US", "--workspace", "t-5")
	if code != exitAuth || strings.Contains(errOut, "in a terminal") || strings.Contains(errOut, "6-digit") {
		t.Fatalf("API key: %d %q", code, errOut)
	}
	if s.availability != 0 || s.sends != 0 || s.gated != 1 {
		t.Fatalf("availability %d sends %d gated %d", s.availability, s.sends, s.gated)
	}
}

func TestStepUpRetriesOnlyOnce(t *testing.T) {
	h := stepUpHarness(t, "123456\n123456\n")
	s := &stepUpServer{alwaysReauth: true}
	s.install(h, countryRoute, `{}`, availableJSON, 200)
	code, out, errOut := h.run("billing", "country", "set", "US")
	if code != exitAuth || !strings.Contains(errOut, "rzv login") {
		t.Fatalf("second refusal: %d %q", code, errOut)
	}
	if s.gated != 2 || s.availability != 1 || s.sends != 1 || s.verifies != 1 {
		t.Fatalf("gated %d availability %d send %d verify %d; want one step-up and one retry", s.gated, s.availability, s.sends, s.verifies)
	}
	noCode(t, out, errOut, "123456")
}

func TestStepUpNoCodeEntered(t *testing.T) {
	h := stepUpHarness(t, "")
	s := &stepUpServer{}
	s.install(h, countryRoute, `{}`, availableJSON, 200)
	code, _, errOut := h.run("billing", "country", "set", "US")
	if code != exitAuth || !strings.Contains(errOut, "No code was entered") || s.verifies != 0 || s.gated != 1 {
		t.Fatalf("empty input: %d %q verify %d", code, errOut, s.verifies)
	}
}
