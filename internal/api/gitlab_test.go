package api

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestGitLabConnectionsDecodeMetadataAndDisconnect(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/customer/tenants/t-1/gitlab/connections":
			_, _ = w.Write([]byte(`{"connections":[{"connection_id":"glc_0123456789abcdef01234567","gitlab_username":"ditah","status":"active","reconnect_required":true,"created_at":"2026-10-26T09:00:00Z"}]}`))
		case "DELETE /v1/customer/tenants/t-1/gitlab/connections/glc_0123456789abcdef01234567":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"not_found"}}`))
		}
	})
	connections, err := client.GitLabConnections(context.Background(), "t-1")
	if err != nil || len(connections) != 1 || !connections[0].ReconnectRequired || connections[0].GitLabUsername != "ditah" {
		t.Fatalf("connections = %+v, err %v", connections, err)
	}
	if err := client.DisconnectGitLabConnection(context.Background(), "t-1", "glc_0123456789abcdef01234567"); err != nil {
		t.Fatalf("disconnect error = %v", err)
	}
}

func TestGitLabConnectionsRejectMissingListAndSurfaceRefusals(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"integration_disabled"}}`))
	})
	_, err := client.GitLabConnections(context.Background(), "t-1")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "integration_disabled" {
		t.Fatalf("error = %v, want integration_disabled", err)
	}
	empty := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	if _, err := empty.GitLabConnections(context.Background(), "t-1"); err == nil {
		t.Fatal("a response without connections must be malformed")
	}
}
