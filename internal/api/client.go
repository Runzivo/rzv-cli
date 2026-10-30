// Package api is a typed client for the Runzivo customer API
// (api/customer.openapi.yaml in the Runzivo service). It covers only the
// operations rzv uses.
package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	requestTimeout  = 10 * time.Second
	maxBodyBytes    = 8 << 20
	maxCSVBodyBytes = 64 << 20
)

// Error is a failed API call. Status is 0 when no response arrived. Code is
// the server's error code when it sent a well-formed one.
type Error struct {
	Status int
	Code   string
}

func (e *Error) Error() string {
	if e.Status == 0 {
		return "the customer API is unavailable"
	}
	return fmt.Sprintf("customer API request failed (%d: %s)", e.Status, e.Code)
}

var errorCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// Client calls the customer API with one bearer credential: a person's
// access token or a workspace API key.
type Client struct {
	baseURL    string
	credential string
	userAgent  string
	http       *http.Client
}

// New returns a client for baseURL, which must be an HTTPS origin (or
// loopback HTTP for local testing).
func New(baseURL, credential, userAgent string, httpClient *http.Client) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("API base URL must be an origin such as https://api.runzivo.dev")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback(u.Hostname())) {
		return nil, fmt.Errorf("API base URL must use HTTPS")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	c := *httpClient
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{baseURL: u.Scheme + "://" + u.Host, credential: credential, userAgent: userAgent, http: &c}, nil
}

// loopback reports whether host is this machine, the only place rzv sends a
// credential without TLS.
func loopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// tenantPath builds /v1/customer/tenants/{id}/{rest} with the id escaped.
func tenantPath(tenantID, rest string) string {
	return "/v1/customer/tenants/" + url.PathEscape(tenantID) + rest
}

// do sends one request. A 2xx response body decodes into out (when out is
// not nil); anything else becomes *Error.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return &Error{Status: 0, Code: "request_failed"}
		}
		reader = bytes.NewReader(data)
	}
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return &Error{Status: 0, Code: "request_failed"}
	}
	req.Header.Set("Authorization", "Bearer "+c.credential)
	req.Header.Set("Accept", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return &Error{Status: 0, Code: "customer_api_unavailable"}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return &Error{Status: 0, Code: "customer_api_unavailable"}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return decodeError(resp.StatusCode, data)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return &Error{Status: resp.StatusCode, Code: "malformed_response"}
	}
	return nil
}

// doCSV uses the same client policy as do while retaining the CSV bytes for
// stdout. It rejects oversized or mislabeled successful responses.
func (c *Client) doCSV(ctx context.Context, path string, query url.Values) ([]byte, error) {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, &Error{Status: 0, Code: "request_failed"}
	}
	req.Header.Set("Authorization", "Bearer "+c.credential)
	req.Header.Set("Accept", "text/csv")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &Error{Status: 0, Code: "customer_api_unavailable"}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCSVBodyBytes+1))
	if err != nil {
		return nil, &Error{Status: 0, Code: "customer_api_unavailable"}
	}
	if len(data) > maxCSVBodyBytes {
		return nil, &Error{Status: resp.StatusCode, Code: "response_too_large"}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, decodeError(resp.StatusCode, data)
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || contentType != "text/csv" {
		return nil, &Error{Status: resp.StatusCode, Code: "malformed_response"}
	}
	reader := csv.NewReader(bytes.NewReader(data))
	header, err := reader.Read()
	if err != nil || !slices.Equal(header, []string{"occurred_at", "category", "action", "description", "result", "error_code", "actor_type", "actor", "actor_email", "api_key_id", "auth_method", "target_type", "target_id", "ip", "user_agent", "request_id", "session_ref"}) {
		return nil, &Error{Status: resp.StatusCode, Code: "malformed_response"}
	}
	reader.FieldsPerRecord = len(header)
	for {
		if _, err := reader.Read(); err == io.EOF {
			break
		} else if err != nil {
			return nil, &Error{Status: resp.StatusCode, Code: "malformed_response"}
		}
	}
	return data, nil
}

func decodeError(status int, data []byte) *Error {
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &body) == nil && errorCodePattern.MatchString(body.Error.Code) {
		return &Error{Status: status, Code: body.Error.Code}
	}
	return &Error{Status: status, Code: "invalid_error_response"}
}

// required reports a malformed response when a required field is missing.
func required(ok bool) error {
	if !ok {
		return &Error{Status: 200, Code: "malformed_response"}
	}
	return nil
}

func nonEmpty(values ...string) bool {
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			return false
		}
	}
	return true
}
