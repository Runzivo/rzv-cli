package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHHost binds a sandbox's stable host certificate principal to Core's host CA.
type SSHHost struct {
	HostCAPublicKey string `json:"host_ca_public_key"`
	HostKeyAlias    string `json:"host_key_alias"`
}

type SSHTicket struct {
	SSHHost
	URL                  string    `json:"url"`
	Ticket               string    `json:"ticket"`
	ExpiresAt            time.Time `json:"expires_at"`
	Certificate          string    `json:"certificate"`
	CertificateExpiresAt time.Time `json:"certificate_expires_at"`
	Principal            string    `json:"principal"`
}

func (h SSHHost) Validate(sandboxID string) error {
	key, _, options, rest, err := ssh.ParseAuthorizedKey([]byte(h.HostCAPublicKey))
	if err != nil || len(options) != 0 || len(rest) != 0 || strings.ContainsAny(h.HostCAPublicKey, "\r\n") || key.Type() != ssh.KeyAlgoED25519 || h.HostKeyAlias != sandboxID+".sandbox.runzivo" {
		return required(false)
	}
	return nil
}

func (c *Client) SandboxSSHHost(ctx context.Context, workspace, sandboxID string) (*SSHHost, error) {
	var host SSHHost
	if err := c.do(ctx, http.MethodGet, sandboxPath(workspace, "/"+url.PathEscape(sandboxID)+"/ssh-host"), nil, nil, &host); err != nil {
		return nil, err
	}
	if err := host.Validate(sandboxID); err != nil {
		return nil, err
	}
	return &host, nil
}

func (c *Client) SandboxSSHTicket(ctx context.Context, workspace, sandboxID, publicKey string) (*SSHTicket, error) {
	var ticket SSHTicket
	if err := c.do(ctx, http.MethodPost, sandboxPath(workspace, "/"+url.PathEscape(sandboxID)+"/ssh-ticket"), nil, map[string]string{"public_key": publicKey}, &ticket); err != nil {
		return nil, err
	}
	if err := ticket.SSHHost.Validate(sandboxID); err != nil {
		return nil, err
	}
	now := time.Now()
	if ticket.Principal != "user" || ticket.Ticket == "" || ticket.Certificate == "" || !ticket.ExpiresAt.After(now) || ticket.ExpiresAt.After(now.Add(45*time.Second)) || !ticket.CertificateExpiresAt.After(now) || ticket.CertificateExpiresAt.After(now.Add(16*time.Minute)) {
		return nil, required(false)
	}
	if _, err := c.sshConnectURL(&ticket, workspace, sandboxID); err != nil {
		return nil, err
	}
	return &ticket, nil
}

func (c *Client) sshConnectURL(ticket *SSHTicket, workspace, sandboxID string) (string, error) {
	target, err := url.Parse(ticket.URL)
	base, _ := url.Parse(c.baseURL)
	scheme := "wss"
	if base.Scheme == "http" {
		scheme = "ws"
	}
	path := sandboxPath(workspace, "/"+url.PathEscape(sandboxID)+"/ssh")
	if err != nil || target.Scheme != scheme || target.Host != base.Host || target.User != nil || target.RawQuery != "" || target.Fragment != "" || target.EscapedPath() != path {
		return "", required(false)
	}
	target.RawQuery = url.Values{"ticket": {ticket.Ticket}}.Encode()
	return target.String(), nil
}

// SandboxSSH redeems one ticket. Neither the bearer credential nor ticket is
// included in returned errors or passed to another process.
func (c *Client) SandboxSSH(ctx context.Context, workspace, sandboxID string, ticket *SSHTicket) (*WebSocket, error) {
	target, err := c.sshConnectURL(ticket, workspace, sandboxID)
	if err != nil {
		return nil, err
	}
	handshakeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return dialWebSocket(handshakeCtx, target, nil)
}
