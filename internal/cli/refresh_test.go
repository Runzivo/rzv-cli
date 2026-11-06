package cli

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Runzivo/rzv-cli/internal/config"
)

// expireAccessToken replaces the stored access token with an expired one,
// keeping the refresh token.
func expireAccessToken(t *testing.T, h *harness) *config.Credentials {
	t.Helper()
	creds, err := h.store.Read()
	if err != nil || creds == nil || creds.RefreshToken == "" {
		t.Fatalf("no stored refresh token: %v", err)
	}
	claims := h.provider.Claims("rzv-customer-api")
	claims["aud"], claims["typ"] = []string{"rzv-customer-api", "account"}, "Bearer"
	claims["exp"] = time.Now().Add(-time.Minute).Unix()
	creds.AccessToken = h.provider.Sign(claims)
	if err := h.store.Write(creds); err != nil {
		t.Fatal(err)
	}
	return creds
}

func credentialsMode(t *testing.T, h *harness) os.FileMode {
	t.Helper()
	files, _ := os.ReadDir(h.store.Dir + "/profiles")
	path := h.store.Dir + "/credentials.json"
	if len(files) > 0 {
		path = h.store.Dir + "/profiles/" + files[0].Name()
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestExpiredAccessTokenRenewsSilently(t *testing.T) {
	h := newHarness(t)
	h.login()
	h.json("GET /v1/customer/memberships", 200, `{"memberships":[]}`)
	old := expireAccessToken(t, h)
	code, out, errOut := h.run("workspaces", "list")
	if code != 0 {
		t.Fatalf("renewal: %d %q", code, errOut)
	}
	creds, _ := h.store.Read()
	if h.provider.RefreshCalls != 1 || creds.AccessToken == old.AccessToken || creds.RefreshToken == old.RefreshToken || creds.RefreshExpiresAt.IsZero() {
		t.Fatalf("tokens not rotated: %d calls", h.provider.RefreshCalls)
	}
	if strings.Contains(out+errOut, creds.RefreshToken) || strings.Contains(out+errOut, old.RefreshToken) || strings.Contains(out+errOut, creds.AccessToken) {
		t.Fatal("a token was printed")
	}
	if mode := credentialsMode(t, h); mode != 0o600 {
		t.Fatalf("credentials file mode %v", mode)
	}
	// A fresh token is used as it is, with no second refresh.
	if code, _, _ := h.run("workspaces", "list"); code != 0 || h.provider.RefreshCalls != 1 {
		t.Fatalf("refreshed again: %d calls", h.provider.RefreshCalls)
	}
}

// Parallel rzv runs (an SSH proxy, an Orca step, an editor) must spend the
// rotating refresh token once; a second use would end the session.
func TestConcurrentRunsRefreshOnce(t *testing.T) {
	h := newHarness(t)
	h.login()
	h.json("GET /v1/customer/memberships", 200, `{"memberships":[]}`)
	expireAccessToken(t, h)
	var wg sync.WaitGroup
	codes := make([]int, 8)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i], _, _ = h.run("workspaces", "list")
		}()
	}
	wg.Wait()
	for i, code := range codes {
		if code != 0 {
			t.Fatalf("run %d exit %d", i, code)
		}
	}
	if h.provider.RefreshCalls != 1 || h.provider.ValidRefreshTokens() != 1 {
		t.Fatalf("refresh calls %d, valid tokens %d", h.provider.RefreshCalls, h.provider.ValidRefreshTokens())
	}
}

func TestEndedSessionAsksForLoginAndKeepsProfile(t *testing.T) {
	h := newHarness(t)
	h.login()
	before := expireAccessToken(t, h)
	h.provider.EndSession()
	code, _, errOut := h.run("workspaces", "list")
	if code != exitAuth || !strings.Contains(errOut, "rzv login") {
		t.Fatalf("ended session: %d %q", code, errOut)
	}
	creds, err := h.store.Read()
	if err != nil || creds == nil || creds.RefreshToken != "" || creds.APIBaseURL != before.APIBaseURL || creds.Issuer != before.Issuer {
		t.Fatalf("profile after an ended session: %+v %v", creds, err)
	}
	calls := h.provider.RefreshCalls
	if code, _, _ := h.run("workspaces", "list"); code != exitAuth || h.provider.RefreshCalls != calls {
		t.Fatal("an ended session was retried")
	}
}

func TestRefreshRefusesAnotherAccount(t *testing.T) {
	h := newHarness(t)
	h.login()
	expireAccessToken(t, h)
	h.provider.RefreshSubject = "someone-else"
	if code, _, errOut := h.run("workspaces", "list"); code != exitAuth || !strings.Contains(errOut, "different account") {
		t.Fatalf("subject change: %d %q", code, errOut)
	}
}

func TestLogoutRevokesTheSession(t *testing.T) {
	h := newHarness(t)
	h.login()
	creds, _ := h.store.Read()
	if code, _, errOut := h.run("logout"); code != 0 {
		t.Fatal(errOut)
	}
	if h.provider.Revocations != 1 || h.provider.ValidRefreshTokens() != 0 {
		t.Fatalf("revocations %d, valid %d", h.provider.Revocations, h.provider.ValidRefreshTokens())
	}
	if c, _ := h.store.Read(); c != nil {
		t.Fatal("credentials kept after logout")
	}
	// A copied file no longer renews.
	_ = h.store.Write(creds)
	expireAccessToken(t, h)
	if code, _, _ := h.run("workspaces", "list"); code != exitAuth {
		t.Fatal("a revoked refresh token still worked")
	}

	// With the provider unreachable, logout still removes local credentials.
	h.login()
	h.provider.Server.Close()
	code, _, errOut := h.run("logout")
	if code != 0 || !strings.Contains(errOut, "did not confirm the session ended") {
		t.Fatalf("offline logout: %d %q", code, errOut)
	}
	if c, _ := h.store.Read(); c != nil {
		t.Fatal("credentials kept after offline logout")
	}
}

// TestHelperHoldsLockAndDies is run as a child process by
// TestLockOfAKilledProcessIsReleased; it exits while holding the lock.
func TestHelperHoldsLockAndDies(t *testing.T) {
	dir := os.Getenv("RZV_TEST_LOCK_DIR")
	if dir == "" {
		t.Skip("helper process only")
	}
	if _, err := (&config.Store{Dir: dir}).Lock(time.Second); err != nil {
		os.Exit(3)
	}
	os.Exit(0) // no unlock: the operating system must release it
}

func TestLockOfAKilledProcessIsReleased(t *testing.T) {
	store := &config.Store{Dir: t.TempDir()}
	unlock, err := store.Lock(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Lock(100 * time.Millisecond); err == nil {
		t.Fatal("a held lock was taken")
	}
	unlock()
	child := exec.Command(os.Args[0], "-test.run=^TestHelperHoldsLockAndDies$")
	child.Env = append(os.Environ(), "RZV_TEST_LOCK_DIR="+store.Dir)
	if out, err := child.CombinedOutput(); err != nil {
		t.Fatalf("helper: %v %s", err, out)
	}
	again, err := store.Lock(200 * time.Millisecond)
	if err != nil {
		t.Fatal("the lock of an exited process was not released")
	}
	again()
}

func TestLogoutWaitsForARefreshInFlight(t *testing.T) {
	h := newHarness(t)
	h.login()
	unlock, err := h.store.ForProfile("default").Lock(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan int)
	go func() { code, _, _ := h.run("logout"); done <- code }()
	select {
	case <-done:
		t.Fatal("logout did not wait for the credential lock")
	case <-time.After(300 * time.Millisecond):
	}
	if c, _ := h.store.Read(); c == nil {
		t.Fatal("logout removed the file while another process held the lock")
	}
	unlock()
	if code := <-done; code != 0 || h.provider.Revocations != 1 {
		t.Fatalf("logout after the lock: %d, %d revocations", code, h.provider.Revocations)
	}
}

func TestSelectingAWorkspaceKeepsARotatedToken(t *testing.T) {
	h := newHarness(t)
	h.login()
	h.json("GET /v1/customer/memberships", 200, `{"memberships":[{"tenant_id":"t-1","tenant_slug":"demo","tenant_name":"Demo","role":"owner"}]}`)
	// Another rzv process rotated the refresh token after this one started.
	creds, _ := h.store.Read()
	creds.RefreshToken = "rotated-elsewhere"
	_ = h.store.Write(creds)
	if code, _, errOut := h.run("workspaces", "select", "t-1"); code != 0 {
		t.Fatal(errOut)
	}
	if after, _ := h.store.Read(); after.RefreshToken != "rotated-elsewhere" || after.SelectedWorkspace != "t-1" {
		t.Fatalf("select wrote a stale token back: %+v", after)
	}
}
