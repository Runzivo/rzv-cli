package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SandboxError says why a sandbox failed. It never names a host.
type SandboxError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Sandbox is a sandbox as the customer API returns it.
type Sandbox struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Size             string        `json:"size"`
	DiskGiB          int           `json:"disk_gib"`
	Status           string        `json:"status"`
	Error            *SandboxError `json:"error"`
	CreatedAt        string        `json:"created_at"`
	StartedAt        *string       `json:"started_at"`
	ExpiresAt        *string       `json:"expires_at"`
	RemainingSeconds int64         `json:"remaining_seconds"`
	RuntimeSeconds   int64         `json:"runtime_seconds"`
	// CreditsUsed is equivalent minutes charged so far.
	CreditsUsed float64 `json:"credits_used"`
}

func (s *Sandbox) valid() bool {
	return nonEmpty(s.ID, s.Name, s.Size, s.Status, s.CreatedAt)
}

func sandboxPath(tenantID, rest string) string { return tenantPath(tenantID, "/sandboxes"+rest) }

// SandboxListing is listCustomerSandboxes' result: the sandboxes, the number
// the workspace may run at once, and the sizes this environment offers (a
// server older than the sizes field sends none).
type SandboxListing struct {
	Sandboxes  []Sandbox `json:"sandboxes"`
	MaxRunning int       `json:"max_running"`
	Sizes      []string  `json:"sizes"`
}

// SandboxList calls listCustomerSandboxes.
func (c *Client) SandboxList(ctx context.Context, tenantID string) (*SandboxListing, error) {
	var body SandboxListing
	if err := c.do(ctx, http.MethodGet, sandboxPath(tenantID, ""), nil, nil, &body); err != nil {
		return nil, err
	}
	for i := range body.Sandboxes {
		if err := required(body.Sandboxes[i].valid()); err != nil {
			return nil, err
		}
	}
	if body.Sizes == nil {
		body.Sizes = []string{}
	}
	return &body, nil
}

// SandboxGet calls getCustomerSandbox.
func (c *Client) SandboxGet(ctx context.Context, tenantID, sandboxID string) (*Sandbox, error) {
	return c.sandboxCall(ctx, http.MethodGet, sandboxPath(tenantID, "/"+url.PathEscape(sandboxID)), nil)
}

// CreateSandboxInput is a create request. DiskGiB 0 and an empty
// ClientOperationID are left out so the server's defaults apply.
type CreateSandboxInput struct {
	Name              string `json:"name"`
	Size              string `json:"size"`
	DiskGiB           int    `json:"disk_gib,omitempty"`
	ClientOperationID string `json:"client_operation_id,omitempty"`
}

// SandboxCreate calls createCustomerSandbox.
func (c *Client) SandboxCreate(ctx context.Context, tenantID string, input CreateSandboxInput) (*Sandbox, error) {
	return c.sandboxCall(ctx, http.MethodPost, sandboxPath(tenantID, ""), input)
}

// SandboxStart, SandboxStop and SandboxDelete call the action routes.
func (c *Client) SandboxStart(ctx context.Context, tenantID, sandboxID string) (*Sandbox, error) {
	return c.sandboxCall(ctx, http.MethodPost, sandboxPath(tenantID, "/"+url.PathEscape(sandboxID)+"/start"), nil)
}

func (c *Client) SandboxStop(ctx context.Context, tenantID, sandboxID string) (*Sandbox, error) {
	return c.sandboxCall(ctx, http.MethodPost, sandboxPath(tenantID, "/"+url.PathEscape(sandboxID)+"/stop"), nil)
}

func (c *Client) SandboxDelete(ctx context.Context, tenantID, sandboxID string) (*Sandbox, error) {
	return c.sandboxCall(ctx, http.MethodDelete, sandboxPath(tenantID, "/"+url.PathEscape(sandboxID)), nil)
}

func (c *Client) sandboxCall(ctx context.Context, method, path string, body any) (*Sandbox, error) {
	var sandbox Sandbox
	if err := c.do(ctx, method, path, nil, body, &sandbox); err != nil {
		return nil, err
	}
	if err := required(sandbox.valid()); err != nil {
		return nil, err
	}
	return &sandbox, nil
}

// ExecInput is a command to run in a running sandbox.
type ExecInput struct {
	Cmd            string            `json:"cmd"`
	Cwd            string            `json:"cwd,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
}

// ExecEvent is one event of the streamed output: stdout or stderr bytes, then
// one exit. Truncated means the output passed 8 MiB and the command was
// stopped; Reason is set when Core ended the command because the sandbox
// stopped, expired or was deleted.
type ExecEvent struct {
	Type      string `json:"type"`
	Data      []byte `json:"-"`
	ExitCode  int    `json:"exit_code"`
	TimedOut  bool   `json:"timed_out"`
	Truncated bool   `json:"truncated"`
	Reason    string `json:"reason"`
}

// maxEventBytes bounds one NDJSON line of the exec stream.
const maxEventBytes = 4 << 20

// SandboxExec calls execCustomerSandbox and hands each event to handle as it
// arrives. It returns the exit event; a stream that ends without one is a
// malformed response. The request has no overall timeout: a command can run
// for an hour, and ctx ends it.
func (c *Client) SandboxExec(ctx context.Context, tenantID, sandboxID string, input ExecInput, handle func(ExecEvent) error) (ExecEvent, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return ExecEvent{}, &Error{Status: 0, Code: "request_failed"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+sandboxPath(tenantID, "/"+url.PathEscape(sandboxID)+"/exec"), bytes.NewReader(data))
	if err != nil {
		return ExecEvent{}, &Error{Status: 0, Code: "request_failed"}
	}
	req.Header.Set("Authorization", "Bearer "+c.credential)
	req.Header.Set("Accept", "application/x-ndjson")
	req.Header.Set("Content-Type", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	stream := *c.http
	stream.Timeout = 0
	resp, err := stream.Do(req)
	if err != nil {
		return ExecEvent{}, &Error{Status: 0, Code: "customer_api_unavailable"}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
		return ExecEvent{}, decodeError(resp.StatusCode, body, resp.Header.Get("Retry-After"))
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), maxEventBytes)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var raw struct {
			Type      string `json:"type"`
			Data      string `json:"data"`
			ExitCode  int    `json:"exit_code"`
			TimedOut  bool   `json:"timed_out"`
			Truncated bool   `json:"truncated"`
			Reason    string `json:"reason"`
		}
		if json.Unmarshal(line, &raw) != nil {
			return ExecEvent{}, &Error{Status: resp.StatusCode, Code: "malformed_response"}
		}
		event := ExecEvent{Type: raw.Type, ExitCode: raw.ExitCode, TimedOut: raw.TimedOut, Truncated: raw.Truncated, Reason: raw.Reason}
		switch raw.Type {
		case "stdout", "stderr":
			decoded, err := base64.StdEncoding.DecodeString(raw.Data)
			if err != nil {
				return ExecEvent{}, &Error{Status: resp.StatusCode, Code: "malformed_response"}
			}
			event.Data = decoded
		case "exit":
		default:
			return ExecEvent{}, &Error{Status: resp.StatusCode, Code: "malformed_response"}
		}
		if err := handle(event); err != nil {
			return ExecEvent{}, err
		}
		if event.Type == "exit" {
			return event, nil
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return ExecEvent{}, &Error{Status: 0, Code: "customer_api_unavailable"}
	}
	if ctx.Err() != nil {
		return ExecEvent{}, ctx.Err()
	}
	return ExecEvent{}, &Error{Status: resp.StatusCode, Code: "malformed_response"}
}

// TerminalTicket is createCustomerSandboxTerminalTicket's result: a
// single-use ticket for the terminal WebSocket at URL, valid until ExpiresAt
// (about 30 seconds). It is for clients that cannot send Authorization on a
// WebSocket, such as a browser.
type TerminalTicket struct {
	URL       string `json:"url"`
	Ticket    string `json:"ticket"`
	ExpiresAt string `json:"expires_at"`
}

// SandboxTerminalTicket calls createCustomerSandboxTerminalTicket.
func (c *Client) SandboxTerminalTicket(ctx context.Context, tenantID, sandboxID string) (*TerminalTicket, error) {
	var ticket TerminalTicket
	if err := c.do(ctx, http.MethodPost, sandboxPath(tenantID, "/"+url.PathEscape(sandboxID)+"/terminal-ticket"), nil, nil, &ticket); err != nil {
		return nil, err
	}
	if err := required(nonEmpty(ticket.URL, ticket.Ticket, ticket.ExpiresAt)); err != nil {
		return nil, err
	}
	if _, err := time.Parse(time.RFC3339, ticket.ExpiresAt); err != nil {
		return nil, required(false)
	}
	return &ticket, nil
}

// ConnectURL is URL with the ticket added as the ticket query parameter. The
// ticket is a credential, so the URL must be wss, or ws on this machine only.
func (t *TerminalTicket) ConnectURL() (string, error) {
	target, err := url.Parse(t.URL)
	if err != nil || target.Host == "" || (target.Scheme != "wss" && !(target.Scheme == "ws" && loopback(target.Hostname()))) {
		return "", required(false)
	}
	query := target.Query()
	query.Set("ticket", t.Ticket)
	target.RawQuery = query.Encode()
	return target.String(), nil
}

// SandboxTerminal opens the sandbox's terminal WebSocket with this client's
// credential in the Authorization header, as the CLI and API clients do.
func (c *Client) SandboxTerminal(ctx context.Context, tenantID, sandboxID string) (*WebSocket, error) {
	target := c.baseURL + sandboxPath(tenantID, "/"+url.PathEscape(sandboxID)+"/terminal")
	switch {
	case strings.HasPrefix(target, "https://"):
		target = "wss://" + strings.TrimPrefix(target, "https://")
	case strings.HasPrefix(target, "http://"):
		target = "ws://" + strings.TrimPrefix(target, "http://")
	}
	header := http.Header{"Authorization": {"Bearer " + c.credential}}
	if c.userAgent != "" {
		header.Set("User-Agent", c.userAgent)
	}
	return dialWebSocket(ctx, target, header)
}

// ResizeMessage is the one text frame a terminal client sends.
func ResizeMessage(cols, rows int) []byte {
	data, _ := json.Marshal(map[string]any{"type": "resize", "cols": cols, "rows": rows})
	return data
}
