package oidc

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Runzivo/rzv-cli/internal/oidctest"
)

func options(p *oidctest.Provider, open func(string) error) Options {
	return Options{
		Issuer: p.Issuer(), ClientID: "rzv-cli", Audience: "rzv-customer-api", BrokerClaim: "identity_provider",
		AllowInsecureLoopback: true, CallbackPort: -1, Timeout: 5 * time.Second, OpenBrowser: open,
	}
}

func TestLoginSuccess(t *testing.T) {
	p := oidctest.New(t)
	var shown string
	opts := options(p, p.Browser("good-code", false))
	opts.ShowURL = func(u string) { shown = u }
	result, err := Login(context.Background(), opts)
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if result.AccessToken == "" || result.Claims.String("sub") != "user-1" || result.Claims.String("preferred_username") != "octocat" {
		t.Fatalf("unexpected result claims: %v", result.Claims)
	}
	u, err := url.Parse(shown)
	if err != nil || !strings.HasPrefix(shown, p.Issuer()+"/auth?") {
		t.Fatalf("authorize URL not shown: %q", shown)
	}
	if u.Query().Get("kc_idp_hint") != "github" || !strings.HasPrefix(u.Query().Get("redirect_uri"), "http://127.0.0.1:") {
		t.Fatalf("unexpected authorize parameters: %v", u.Query())
	}
}

func TestLoginRejectsTamperedState(t *testing.T) {
	p := oidctest.New(t)
	_, err := Login(context.Background(), options(p, p.Browser("good-code", true)))
	if err == nil || !strings.Contains(err.Error(), "callback validation failed") {
		t.Fatalf("want callback failure, got %v", err)
	}
}

func TestLoginRejectsBadClaims(t *testing.T) {
	cases := map[string]func(kind string, c map[string]any){
		"wrong audience": func(kind string, c map[string]any) {
			if kind == "access" {
				c["aud"] = "something-else"
			}
		},
		"missing broker claim": func(_ string, c map[string]any) { delete(c, "identity_provider") },
		"wrong azp":            func(_ string, c map[string]any) { c["azp"] = "other-client" },
		"wrong issuer":         func(_ string, c map[string]any) { c["iss"] = "https://evil.example.test" },
		"not bearer": func(kind string, c map[string]any) {
			if kind == "access" {
				c["typ"] = "ID"
			}
		},
		"wrong nonce": func(kind string, c map[string]any) {
			if kind == "id" {
				c["nonce"] = "other"
			}
		},
		"expired": func(_ string, c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() },
		"subject mismatch": func(kind string, c map[string]any) {
			if kind == "access" {
				c["sub"] = "user-2"
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := oidctest.New(t)
			p.Mutate = mutate
			if _, err := Login(context.Background(), options(p, p.Browser("good-code", false))); err == nil {
				t.Fatal("login accepted a bad token")
			}
		})
	}
}

func TestVerifyRejectsOtherKeyAndAlgNone(t *testing.T) {
	p := oidctest.New(t)
	other := oidctest.New(t)
	set, err := fetchKeySet(context.Background(), http.DefaultClient, p.Issuer()+"/certs")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := set.verifyRS256(p.Sign(p.Claims("x"))); err != nil {
		t.Fatalf("own token rejected: %v", err)
	}
	if _, err := set.verifyRS256(other.Sign(other.Claims("x"))); err == nil {
		t.Fatal("token signed by another key verified")
	}
	parts := strings.Split(p.Sign(p.Claims("x")), ".")
	none := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + parts[1] + "."
	if _, err := set.verifyRS256(none); err == nil {
		t.Fatal("alg none verified")
	}
}

func TestLoginRejectsBadCode(t *testing.T) {
	p := oidctest.New(t)
	_, err := Login(context.Background(), options(p, p.Browser("bad-code", false)))
	if err == nil || !strings.Contains(err.Error(), "rejected sign-in") {
		t.Fatalf("want rejected sign-in, got %v", err)
	}
}

func TestLoginTimesOut(t *testing.T) {
	p := oidctest.New(t)
	opts := options(p, func(string) error { return nil })
	opts.Timeout = 50 * time.Millisecond
	if _, err := Login(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("want timeout, got %v", err)
	}
}

func TestNormalizeURLRequiresHTTPS(t *testing.T) {
	if _, err := NormalizeURL("http://example.test", "Issuer", false); err == nil {
		t.Fatal("http accepted")
	}
	if _, err := NormalizeURL("http://127.0.0.1:8080", "Issuer", false); err == nil {
		t.Fatal("loopback http accepted without the flag")
	}
	if _, err := NormalizeURL("http://127.0.0.1:8080", "Issuer", true); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeURL("https://user:pw@example.test", "Issuer", false); err == nil {
		t.Fatal("credentials accepted")
	}
}
