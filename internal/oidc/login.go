package oidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultCallbackPort is registered on the rzv-cli client as
	// http://127.0.0.1:43821/oauth/callback. It must not change without the
	// identity realm changing too.
	DefaultCallbackPort = 43821
	callbackPath        = "/oauth/callback"
	callbackMaxBytes    = 16 * 1024
	networkTimeout      = 10 * time.Second
	defaultLoginTimeout = 5 * time.Minute
)

// Options configures one sign-in.
type Options struct {
	Issuer   string
	ClientID string
	Audience string
	// BrokerClaim names the claim that must equal "github" on both tokens.
	// Empty skips the check and the kc_idp_hint=github authorize hint.
	BrokerClaim string
	// AllowInsecureLoopback permits http:// issuer URLs on loopback, for
	// local testing only.
	AllowInsecureLoopback bool
	// CallbackPort is the loopback port; 0 picks DefaultCallbackPort.
	// Tests use -1 for an ephemeral port.
	CallbackPort int
	Timeout      time.Duration
	HTTPClient   *http.Client
	// OpenBrowser opens the authorization URL. When it fails, the URL has
	// already been passed to ShowURL, so the person can open it by hand.
	OpenBrowser func(string) error
	// ShowURL prints the authorization URL (to stderr).
	ShowURL func(string)
	Now     func() time.Time
}

// Result is a verified sign-in. AccessToken must never be printed.
type Result struct {
	Issuer      string
	AccessToken string
	Claims      Claims
}

type discovery struct {
	Issuer                       string   `json:"issuer"`
	AuthorizationEndpoint        string   `json:"authorization_endpoint"`
	TokenEndpoint                string   `json:"token_endpoint"`
	JWKSURI                      string   `json:"jwks_uri"`
	ResponseModesSupported       []string `json:"response_modes_supported"`
	CodeChallengeMethods         []string `json:"code_challenge_methods_supported"`
	ResponseTypesSupported       []string `json:"response_types_supported"`
	AuthorizationResponseIssuerP *bool    `json:"authorization_response_iss_parameter_supported"`
}

// Login runs the browser sign-in and returns a verified access token.
func Login(ctx context.Context, opts Options) (*Result, error) {
	issuerURL, err := normalizedURL(opts.Issuer, "Issuer", opts.AllowInsecureLoopback)
	if err != nil {
		return nil, err
	}
	issuer := strings.TrimSuffix(issuerURL.String(), "/")
	if strings.TrimSpace(opts.ClientID) == "" || strings.TrimSpace(opts.Audience) == "" {
		return nil, configError("Client ID and audience are required.")
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: networkTimeout}
	}
	client = noRedirects(client)
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = defaultLoginTimeout
	}

	meta, err := discover(ctx, client, issuer, opts.AllowInsecureLoopback)
	if err != nil {
		return nil, err
	}
	state, nonce, verifier := randomString(), randomString(), randomString()

	port := opts.CallbackPort
	if port == 0 {
		port = DefaultCallbackPort
	} else if port < 0 {
		port = 0
	}
	requireIssuer := meta.AuthorizationResponseIssuerP != nil && *meta.AuthorizationResponseIssuerP
	receiver, err := listenForCallback(port, state, issuer, requireIssuer)
	if err != nil {
		return nil, err
	}
	defer receiver.close()

	params := url.Values{
		"client_id":             {opts.ClientID},
		"redirect_uri":          {receiver.redirectURI},
		"response_type":         {"code"},
		"response_mode":         {"form_post"},
		"scope":                 {"openid"},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {challenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	if opts.BrokerClaim != "" {
		params.Set("kc_idp_hint", "github")
	}
	authorize, _ := url.Parse(meta.AuthorizationEndpoint)
	authorize.RawQuery = params.Encode()
	if opts.ShowURL != nil {
		opts.ShowURL(authorize.String())
	}
	if opts.OpenBrowser != nil {
		// A browser that cannot open is not fatal: the URL is already shown.
		_ = opts.OpenBrowser(authorize.String())
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var code string
	select {
	case result := <-receiver.results:
		if result.err != nil {
			return nil, result.err
		}
		code = result.code
	case <-waitCtx.Done():
		if ctx.Err() != nil {
			return nil, authError("Sign-in was cancelled.")
		}
		return nil, authError("Sign-in timed out. Run `rzv login` again.")
	}

	accessToken, idToken, err := exchange(ctx, client, meta.TokenEndpoint, opts.ClientID, receiver.redirectURI, code, verifier)
	if err != nil {
		return nil, err
	}
	set, err := fetchKeySet(ctx, client, meta.JWKSURI)
	if err != nil {
		return nil, unavailable("Identity provider signing keys are unavailable.")
	}
	base := tokenCheck{issuer: issuer, clientID: opts.ClientID, brokerClaim: opts.BrokerClaim, now: now(), allowedSkew: 30 * time.Second}
	idCheck := base
	idCheck.audience, idCheck.nonce, idCheck.kindForError = opts.ClientID, nonce, "ID"
	idClaims, err := idCheck.verify(set, idToken)
	if err != nil {
		return nil, err
	}
	accessCheck := base
	accessCheck.audience, accessCheck.requireTyp, accessCheck.kindForError = opts.Audience, true, "access"
	accessClaims, err := accessCheck.verify(set, accessToken)
	if err != nil {
		return nil, err
	}
	if idClaims.String("sub") != accessClaims.String("sub") {
		return nil, authError("Identity provider returned mismatched tokens. Sign in again.")
	}
	// Name and email come from the ID token; the access token may omit them.
	for _, name := range []string{"preferred_username", "email", "name"} {
		if accessClaims.String(name) == "" && idClaims.String(name) != "" {
			accessClaims[name] = idClaims[name]
		}
	}
	return &Result{Issuer: issuer, AccessToken: accessToken, Claims: accessClaims}, nil
}

func noRedirects(base *http.Client) *http.Client {
	c := *base
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &c
}

func discover(ctx context.Context, client *http.Client, issuer string, allowInsecure bool) (*discovery, error) {
	failed := unavailable("Identity provider discovery is unavailable.")
	invalid := unavailable("Identity provider returned invalid discovery metadata.")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return nil, failed
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, failed
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, failed
	}
	var meta discovery
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&meta); err != nil {
		return nil, invalid
	}
	if meta.Issuer != issuer || meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" || meta.JWKSURI == "" {
		return nil, invalid
	}
	issuerURL, _ := url.Parse(issuer)
	for _, endpoint := range []string{meta.AuthorizationEndpoint, meta.TokenEndpoint, meta.JWKSURI} {
		u, err := normalizedURL(endpoint, "Identity provider endpoint", allowInsecure)
		if err != nil {
			return nil, invalid
		}
		if u.Scheme != issuerURL.Scheme || u.Host != issuerURL.Host {
			return nil, unavailable("Identity provider returned an endpoint outside its issuer origin.")
		}
	}
	if !slices.Contains(meta.ResponseModesSupported, "form_post") ||
		!slices.Contains(meta.CodeChallengeMethods, "S256") ||
		(meta.ResponseTypesSupported != nil && !slices.Contains(meta.ResponseTypesSupported, "code")) {
		return nil, unavailable("Identity provider does not support the required native-client sign-in flow.")
	}
	return &meta, nil
}

type callbackResult struct {
	code string
	err  error
}

type callbackReceiver struct {
	server      *http.Server
	redirectURI string
	results     chan callbackResult
}

func listenForCallback(port int, state, issuer string, requireIssuer bool) (*callbackReceiver, error) {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, unavailable("The local sign-in callback port is unavailable. Close the conflicting process and try again.")
	}
	r := &callbackReceiver{
		redirectURI: "http://127.0.0.1:" + strconv.Itoa(listener.Addr().(*net.TCPAddr).Port) + callbackPath,
		results:     make(chan callbackResult, 1),
	}
	deliver := func(result callbackResult) {
		select {
		case r.results <- result:
		default: // only the first callback counts
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, req *http.Request) {
		mediaType, _, _ := mime.ParseMediaType(req.Header.Get("Content-Type"))
		if req.Method != http.MethodPost || mediaType != "application/x-www-form-urlencoded" {
			http.NotFound(w, req)
			return
		}
		reject := func() {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "Sign-in could not be completed. Return to the terminal.")
			deliver(callbackResult{err: authError("Sign-in callback validation failed.")})
		}
		body, err := io.ReadAll(io.LimitReader(req.Body, callbackMaxBytes+1))
		if err != nil || len(body) > callbackMaxBytes {
			reject()
			return
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			reject()
			return
		}
		code, gotState := values.Get("code"), values.Get("state")
		if values.Has("error") || code == "" || gotState == "" ||
			subtle.ConstantTimeCompare([]byte(gotState), []byte(state)) != 1 ||
			(requireIssuer && values.Get("iss") != issuer) {
			reject()
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "Sign-in completed. Return to the terminal.")
		deliver(callbackResult{code: code})
	})
	r.server = &http.Server{Handler: mux, ReadHeaderTimeout: networkTimeout}
	go func() { _ = r.server.Serve(listener) }()
	return r, nil
}

func (r *callbackReceiver) close() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = r.server.Shutdown(ctx)
}

func exchange(ctx context.Context, client *http.Client, endpoint, clientID, redirectURI, code, verifier string) (string, string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"code":          {code},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", unavailable("Identity provider token exchange is unavailable.")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", unavailable("Identity provider token exchange is unavailable.")
	}
	defer resp.Body.Close()
	var body struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", "", authError("Identity provider returned an invalid token response.")
	}
	if resp.StatusCode != http.StatusOK || body.AccessToken == "" || body.IDToken == "" {
		return "", "", authError("Identity provider rejected sign-in.")
	}
	return body.AccessToken, body.IDToken, nil
}

// NormalizeURL checks that value is an absolute HTTPS URL (or loopback HTTP
// when allowed) without credentials or a fragment.
func NormalizeURL(value, label string, allowInsecureLoopback bool) (*url.URL, error) {
	return normalizedURL(value, label, allowInsecureLoopback)
}

func normalizedURL(value, label string, allowInsecureLoopback bool) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" {
		return nil, configError("Invalid " + strings.ToLower(label) + ".")
	}
	host := u.Hostname()
	loopback := host == "127.0.0.1" || host == "localhost" || host == "::1"
	if u.User != nil || u.Fragment != "" ||
		(u.Scheme != "https" && !(allowInsecureLoopback && loopback && u.Scheme == "http")) {
		return nil, configError(label + " must use HTTPS.")
	}
	return u, nil
}

func randomString() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(errors.New("crypto/rand unavailable"))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
