// Package githubauth gets a GitHub user access token with the OAuth device
// flow. `rzv github authorize` sends it to Runzivo once, to prove the person
// controls a GitHub App installation; rzv never stores it.
package githubauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is GitHub's web origin.
const DefaultBaseURL = "https://github.com"

// Error codes. Denied and expired are the person's outcome; the rest are
// GitHub failures.
const (
	CodeDenied  = "access_denied"
	CodeExpired = "github_authorization_expired"
	CodeFailed  = "github_authorization_failed"
)

// Error is a device-flow failure. Messages never include tokens or codes.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// Options configures one device-flow authorization.
type Options struct {
	ClientID   string
	Scope      string
	BaseURL    string
	HTTPClient *http.Client
	// Prompt shows the verification URL and user code (to stderr).
	Prompt      func(verificationURI, userCode string)
	OpenBrowser func(string) error
	Sleep       func(context.Context, time.Duration) error
	Now         func() time.Time
}

type deviceCode struct {
	DeviceCode      string  `json:"device_code"`
	UserCode        string  `json:"user_code"`
	VerificationURI string  `json:"verification_uri"`
	ExpiresIn       float64 `json:"expires_in"`
	Interval        float64 `json:"interval"`
}

// Authorize runs the device flow and returns the user access token.
func Authorize(ctx context.Context, opts Options) (string, error) {
	base := opts.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	sleep := opts.Sleep
	if sleep == nil {
		sleep = func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		}
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	form := url.Values{"client_id": {opts.ClientID}}
	if opts.Scope != "" {
		form.Set("scope", opts.Scope)
	}
	var code deviceCode
	status, err := post(ctx, client, base+"/login/device/code", form, &code)
	if err != nil || status != http.StatusOK {
		return "", &Error{CodeFailed, "GitHub device code request failed."}
	}
	if code.DeviceCode == "" || code.UserCode == "" || code.ExpiresIn <= 0 || code.Interval <= 0 || !sameOrigin(code.VerificationURI, base) {
		return "", &Error{CodeFailed, "GitHub's device code response was invalid."}
	}
	if opts.Prompt != nil {
		opts.Prompt(code.VerificationURI, code.UserCode)
	}
	if opts.OpenBrowser != nil {
		_ = opts.OpenBrowser(code.VerificationURI) // the prompt covers a failure
	}

	interval := time.Duration(code.Interval * float64(time.Second))
	if interval < time.Second {
		interval = time.Second
	}
	deadline := now().Add(time.Duration(code.ExpiresIn * float64(time.Second)))
	expired := &Error{CodeExpired, "The GitHub device code expired before authorization completed."}
	for {
		if !now().Before(deadline) {
			return "", expired
		}
		if err := sleep(ctx, interval); err != nil {
			return "", &Error{CodeFailed, "GitHub authorization was cancelled."}
		}
		if !now().Before(deadline) {
			return "", expired
		}
		var poll struct {
			AccessToken string `json:"access_token"`
			Error       string `json:"error"`
		}
		status, err := post(ctx, client, base+"/login/oauth/access_token", url.Values{
			"client_id":   {opts.ClientID},
			"device_code": {code.DeviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}, &poll)
		if err != nil {
			return "", &Error{CodeFailed, "GitHub device token poll failed."}
		}
		if status == http.StatusOK && poll.AccessToken != "" {
			return poll.AccessToken, nil
		}
		switch poll.Error {
		case "authorization_pending":
		case "slow_down":
			interval += 5 * time.Second
		case "access_denied":
			return "", &Error{CodeDenied, "GitHub authorization was denied."}
		case "expired_token":
			return "", expired
		default:
			return "", &Error{CodeFailed, fmt.Sprintf("GitHub device token poll failed (%d).", status)}
		}
	}
}

func post(ctx context.Context, client *http.Client, endpoint string, form url.Values, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	// A body that is not JSON leaves out empty; callers check the fields.
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
	return resp.StatusCode, nil
}

func sameOrigin(raw, base string) bool {
	u, err := url.Parse(raw)
	b, berr := url.Parse(base)
	return err == nil && berr == nil && u.Scheme == b.Scheme && u.Host == b.Host && u.Host != ""
}
