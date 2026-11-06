// Package oidctest is a minimal OpenID provider for tests. It signs tokens
// with a throwaway RSA key generated per test. Nothing here ships in rzv.
package oidctest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// Provider serves discovery, keys and the token endpoint. Only the code
// "good-code" is exchanged, and only with the PKCE verifier that matches the
// challenge the browser saw.
type Provider struct {
	t         *testing.T
	Server    *httptest.Server
	key       *rsa.PrivateKey
	mu        sync.Mutex
	nonce     string
	challenge string
	// Mutate, when set, edits claims before signing. kind is "id" or "access".
	Mutate func(kind string, claims map[string]any)
	// TokenLifetime defaults to five minutes.
	TokenLifetime time.Duration
	// Refresh tokens rotate on every use, like the Runzivo realms: each one
	// works once, and reusing a spent one ends the whole session.
	refresh      map[string]bool // token -> still valid
	sessionEnded bool
	// RefreshCalls and Revocations count those requests.
	RefreshCalls int
	Revocations  int
	// RefreshSubject, when set, is the subject of renewed access tokens.
	RefreshSubject string
}

// New starts a provider whose issuer is Issuer().
func New(t *testing.T) *Provider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &Provider{t: t, key: key, TokenLifetime: 5 * time.Minute, refresh: map[string]bool{}}
	mux := http.NewServeMux()
	p.Server = httptest.NewServer(mux)
	t.Cleanup(p.Server.Close)
	mux.HandleFunc("/realms/test/.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("/realms/test/certs", p.certs)
	mux.HandleFunc("/realms/test/token", p.token)
	mux.HandleFunc("/realms/test/revoke", p.revoke)
	return p
}

// Issuer is the provider's issuer URL.
func (p *Provider) Issuer() string { return p.Server.URL + "/realms/test" }

func (p *Provider) discovery(w http.ResponseWriter, _ *http.Request) {
	issuer := p.Issuer()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                           issuer,
		"authorization_endpoint":           issuer + "/auth",
		"token_endpoint":                   issuer + "/token",
		"revocation_endpoint":              issuer + "/revoke",
		"jwks_uri":                         issuer + "/certs",
		"response_modes_supported":         []string{"query", "form_post"},
		"code_challenge_methods_supported": []string{"S256"},
		"response_types_supported":         []string{"code"},
	})
}

func (p *Provider) certs(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": "k1", "use": "sig", "alg": "RS256",
		"n": base64.RawURLEncoding.EncodeToString(p.key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(p.key.E)).Bytes()),
	}}})
}

func (p *Provider) newRefreshToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token := "rt-" + base64.RawURLEncoding.EncodeToString(b)
	p.refresh[token] = true
	return token
}

// ValidRefreshTokens is how many refresh tokens would still be accepted.
func (p *Provider) ValidRefreshTokens() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, ok := range p.refresh {
		if ok && !p.sessionEnded {
			n++
		}
	}
	return n
}

// EndSession makes every refresh token stop working, as an expired or
// revoked session does.
func (p *Provider) EndSession() {
	p.mu.Lock()
	p.sessionEnded = true
	p.mu.Unlock()
}

func (p *Provider) refreshGrant(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.RefreshCalls++
	token := r.Form.Get("refresh_token")
	valid, known := p.refresh[token]
	if r.Form.Get("client_id") != "rzv-cli" || p.sessionEnded || !known || !valid {
		if known && !valid {
			p.sessionEnded = true // reuse of a spent token ends the session
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Token is not active"}`))
		return
	}
	p.refresh[token] = false
	access := p.Claims("rzv-customer-api")
	access["aud"], access["typ"] = []string{"rzv-customer-api", "account"}, "Bearer"
	if p.RefreshSubject != "" {
		access["sub"] = p.RefreshSubject
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": p.Sign(access), "refresh_token": p.newRefreshToken(), "refresh_expires_in": 1209600, "token_type": "Bearer"})
}

func (p *Provider) revoke(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Revocations++
	if _, ok := p.refresh[r.Form.Get("token")]; ok && r.Form.Get("client_id") == "rzv-cli" {
		p.sessionEnded = true
	}
}

func (p *Provider) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if r.Form.Get("grant_type") == "refresh_token" {
		p.refreshGrant(w, r)
		return
	}
	p.mu.Lock()
	challenge, nonce := p.challenge, p.nonce
	p.mu.Unlock()
	sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
	if r.Form.Get("code") != "good-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		return
	}
	id := p.Claims("rzv-cli")
	id["nonce"], id["preferred_username"] = nonce, "octocat"
	access := p.Claims("rzv-customer-api")
	access["aud"], access["typ"] = []string{"rzv-customer-api", "account"}, "Bearer"
	if p.Mutate != nil {
		p.Mutate("id", id)
		p.Mutate("access", access)
	}
	p.mu.Lock()
	p.sessionEnded = false
	refresh := p.newRefreshToken()
	p.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": p.Sign(access), "id_token": p.Sign(id), "refresh_token": refresh, "refresh_expires_in": 1209600, "token_type": "Bearer"})
}

// Claims returns a valid claim set for audience.
func (p *Provider) Claims(audience string) map[string]any {
	now := time.Now()
	return map[string]any{
		"iss": p.Issuer(), "sub": "user-1", "azp": "rzv-cli", "aud": audience,
		"iat": now.Unix(), "exp": now.Add(p.TokenLifetime).Unix(), "identity_provider": "github",
	}
}

// Sign returns an RS256 JWT for claims.
func (p *Provider) Sign(claims map[string]any) string {
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
	payload, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, digest[:])
	if err != nil {
		p.t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// Browser plays the person: it reads the authorize URL and posts the
// callback form as the provider would. tamperState sends a wrong state.
func (p *Provider) Browser(code string, tamperState bool) func(string) error {
	return func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		q := u.Query()
		p.mu.Lock()
		p.nonce, p.challenge = q.Get("nonce"), q.Get("code_challenge")
		p.mu.Unlock()
		if q.Get("code_challenge_method") != "S256" || q.Get("response_mode") != "form_post" || q.Get("scope") != "openid" {
			p.t.Errorf("unexpected authorize parameters: %v", q)
		}
		state := q.Get("state")
		if tamperState {
			state = "wrong"
		}
		go func() {
			form := url.Values{"code": {code}, "state": {state}}
			resp, err := http.Post(q.Get("redirect_uri"), "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
			if err == nil {
				resp.Body.Close()
			}
		}()
		return nil
	}
}
