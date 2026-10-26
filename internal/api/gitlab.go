package api

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// GitLabConnection is one GitLab connection of a workspace. It carries
// metadata only; credentials never leave the server.
type GitLabConnection struct {
	ConnectionID      string    `json:"connection_id"`
	GitLabUserID      int64     `json:"gitlab_user_id"`
	GitLabUsername    string    `json:"gitlab_username"`
	Status            string    `json:"status"`
	ReconnectRequired bool      `json:"reconnect_required"`
	InstanceID        string    `json:"instance_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

// GitLabConnections calls listCustomerGitLabConnections. It is empty when the
// workspace has none.
func (c *Client) GitLabConnections(ctx context.Context, tenantID string) ([]GitLabConnection, error) {
	var body struct {
		Connections *[]GitLabConnection `json:"connections"`
	}
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/gitlab/connections"), nil, nil, &body); err != nil {
		return nil, err
	}
	if body.Connections == nil {
		return nil, &Error{Status: http.StatusOK, Code: "malformed_response"}
	}
	return *body.Connections, nil
}

// DisconnectGitLabConnection calls deleteCustomerGitLabConnection. The server
// revokes the grant and erases local credentials; a 204 means it is done.
func (c *Client) DisconnectGitLabConnection(ctx context.Context, tenantID, connectionID string) error {
	return c.do(ctx, http.MethodDelete, tenantPath(tenantID, "/gitlab/connections/"+url.PathEscape(connectionID)), nil, nil, nil)
}
