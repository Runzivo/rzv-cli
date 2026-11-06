package cli

import (
	"os"
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
	if code != 0 || !strings.Contains(errOut, "Could not reach the identity provider") {
		t.Fatalf("offline logout: %d %q", code, errOut)
	}
	if c, _ := h.store.Read(); c != nil {
		t.Fatal("credentials kept after offline logout")
	}
}

func TestLockTakesOverAStaleLock(t *testing.T) {
	store := &config.Store{Dir: t.TempDir()}
	unlock, err := store.Lock(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Lock(100 * time.Millisecond); err == nil {
		t.Fatal("a held lock was taken")
	}
	unlock()
	unlock, err = store.Lock(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	stale := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(store.Dir+"/credentials.json.lock", stale, stale); err != nil {
		t.Fatal(err)
	}
	if again, err := store.Lock(time.Second); err != nil {
		t.Fatal("a stale lock was not taken over")
	} else {
		again()
	}
}
