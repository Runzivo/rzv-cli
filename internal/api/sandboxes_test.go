package api

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

const apiSandboxJSON = `{"id":"sbx_0123456789abcdef01234567","name":"dev","size":"2vcpu-8gb","disk_gib":20,"status":"cleanup_pending","lifetime_hours":12,"error":null,"created_at":"2026-10-29T09:00:00Z","started_at":null,"expires_at":null,"remaining_seconds":43200,"runtime_seconds":0,"credits_used":0}`

func TestSandboxCreateSendsLifetimeOnlyWhenSet(t *testing.T) {
	var bodies []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(data))
		w.WriteHeader(201)
		_, _ = w.Write([]byte(apiSandboxJSON))
	})
	ctx := context.Background()
	sandbox, err := c.SandboxCreate(ctx, "t-1", CreateSandboxInput{Name: "dev", Size: "2vcpu-8gb", LifetimeHours: 12})
	if err != nil {
		t.Fatal(err)
	}
	if sandbox.LifetimeHours != 12 || sandbox.Status != "cleanup_pending" || sandbox.ExpiresAt != nil || sandbox.RemainingSeconds != 43200 {
		t.Fatalf("sandbox = %+v", sandbox)
	}
	if _, err := c.SandboxCreate(ctx, "t-1", CreateSandboxInput{Name: "dev", Size: "2vcpu-8gb"}); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || !strings.Contains(bodies[0], `"lifetime_hours":12`) || strings.Contains(bodies[1], "lifetime") {
		t.Fatalf("bodies = %v", bodies)
	}
}

func TestSandboxWithoutLifetimeHoursIsUnknown(t *testing.T) {
	c := newTestClient(t, respond(200, strings.Replace(apiSandboxJSON, `"lifetime_hours":12,`, "", 1)))
	sandbox, err := c.SandboxGet(context.Background(), "t-1", "sbx_0123456789abcdef01234567")
	if err != nil || sandbox.LifetimeHours != 0 {
		t.Fatalf("sandbox = %+v, err = %v", sandbox, err)
	}
}

func TestSandboxCreateSurfacesInvalidLifetime(t *testing.T) {
	c := newTestClient(t, respond(400, `{"error":{"code":"invalid_lifetime"}}`))
	_, err := c.SandboxCreate(context.Background(), "t-1", CreateSandboxInput{Name: "dev", Size: "2vcpu-8gb", LifetimeHours: 12})
	apiErr, ok := err.(*Error)
	if !ok || apiErr.Status != 400 || apiErr.Code != "invalid_lifetime" {
		t.Fatalf("err = %#v", err)
	}
}
