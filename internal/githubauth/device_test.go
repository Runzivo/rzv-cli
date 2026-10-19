package githubauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func fakeGitHub(t *testing.T, polls []string) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	n := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != "Iv1.test" {
			w.WriteHeader(400)
			return
		}
		_, _ = w.Write([]byte(`{"device_code":"dc","user_code":"ABCD-1234","verification_uri":"` + server.URL + `/login/device","expires_in":900,"interval":5}`))
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("device_code") != "dc" {
			w.WriteHeader(400)
			return
		}
		result := polls[n]
		n++
		if result == "ok" {
			_, _ = w.Write([]byte(`{"access_token":"gh-user-token","token_type":"bearer"}`))
			return
		}
		_, _ = w.Write([]byte(`{"error":"` + result + `"}`))
	})
	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func options(base string) (Options, *[]time.Duration) {
	var waits []time.Duration
	return Options{
		ClientID: "Iv1.test", BaseURL: base,
		Sleep: func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil },
	}, &waits
}

func TestAuthorizeSucceedsAfterPendingAndSlowDown(t *testing.T) {
	server := fakeGitHub(t, []string{"authorization_pending", "slow_down", "ok"})
	opts, waits := options(server.URL)
	var prompted string
	opts.Prompt = func(uri, code string) { prompted = uri + " " + code }
	token, err := Authorize(context.Background(), opts)
	if err != nil || token != "gh-user-token" {
		t.Fatalf("got %q, %v", token, err)
	}
	if prompted != server.URL+"/login/device ABCD-1234" {
		t.Fatalf("prompt: %q", prompted)
	}
	if len(*waits) != 3 || (*waits)[2] != 10*time.Second {
		t.Fatalf("waits: %v", *waits)
	}
}

func TestAuthorizeDenied(t *testing.T) {
	server := fakeGitHub(t, []string{"access_denied"})
	opts, _ := options(server.URL)
	_, err := Authorize(context.Background(), opts)
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeDenied {
		t.Fatalf("got %v", err)
	}
}

func TestAuthorizeRejectsForeignVerificationURI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"device_code":"dc","user_code":"X","verification_uri":"https://evil.example.test/","expires_in":900,"interval":5}`))
	}))
	t.Cleanup(server.Close)
	opts, _ := options(server.URL)
	_, err := Authorize(context.Background(), opts)
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeFailed {
		t.Fatalf("got %v", err)
	}
}

func TestAuthorizeExpires(t *testing.T) {
	server := fakeGitHub(t, []string{"authorization_pending", "authorization_pending"})
	opts, _ := options(server.URL)
	start := time.Now()
	calls := 0
	opts.Now = func() time.Time {
		calls++
		return start.Add(time.Duration(calls) * 10 * time.Minute)
	}
	_, err := Authorize(context.Background(), opts)
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeExpired {
		t.Fatalf("got %v", err)
	}
}
