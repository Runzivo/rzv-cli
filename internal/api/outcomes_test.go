package api

import (
	"context"
	"net/http"
	"testing"
)

func respond(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func TestDocumentedNotFoundOutcomes(t *testing.T) {
	ctx := context.Background()
	usage := newTestClient(t, respond(404, `{"error":{"code":"usage_not_recorded"}}`))
	if got, err := usage.Usage(ctx, "t"); err != nil || got.Kind != "not_recorded" {
		t.Fatalf("usage: %+v %v", got, err)
	}
	linked := newTestClient(t, respond(404, `{"error":{"code":"installation_not_linked"}}`))
	if got, err := linked.Installation(ctx, "t"); err != nil || got != nil {
		t.Fatalf("installation: %+v %v", got, err)
	}
	profiles := newTestClient(t, respond(404, `{"error":{"code":"repository_not_authorized"}}`))
	if got, err := profiles.RunnerProfiles(ctx, "t", 1); err != nil || got != nil {
		t.Fatalf("profiles: %+v %v", got, err)
	}
	// A plain 404 stays an error.
	plain := newTestClient(t, respond(404, `{"error":{"code":"not_found"}}`))
	if _, err := plain.Usage(ctx, "t"); err == nil {
		t.Fatal("generic 404 treated as an outcome")
	}
}

func TestCompleteLinkOutcomes(t *testing.T) {
	cases := map[string]struct {
		status int
		code   string
	}{
		LinkAuthorityDenied:     {403, "installation_authority_denied"},
		LinkStateInvalid:        {409, "link_state_invalid"},
		LinkAuthorityUnverified: {503, "installation_authority_unverified"},
	}
	for want, tc := range cases {
		c := newTestClient(t, respond(tc.status, `{"error":{"code":"`+tc.code+`"}}`))
		inst, got, err := c.CompleteInstallationLink(context.Background(), "t", CompleteLinkInput{State: "s", InstallationID: 1, UserAccessToken: "x"})
		if err != nil || inst != nil || got != want {
			t.Errorf("%s: %v %q %v", want, inst, got, err)
		}
	}
}

func TestMalformedBodiesAreRejected(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t, respond(200, `{}`))
	if _, err := c.Jobs(ctx, "t"); err == nil {
		t.Error("jobs")
	}
	if _, err := c.Job(ctx, "t", "j"); err == nil {
		t.Error("job")
	}
	if _, err := c.Billing(ctx, "t"); err == nil {
		t.Error("billing")
	}
	if _, err := c.Purchases(ctx, "t"); err == nil {
		t.Error("purchases")
	}
	if _, err := c.Installations(ctx, "t"); err == nil {
		t.Error("installations")
	}
	if _, err := c.APIKeys(ctx, "t"); err == nil {
		t.Error("api keys")
	}
	if _, err := c.CreateAPIKey(ctx, "t", CreateAPIKeyInput{}); err == nil {
		t.Error("create api key")
	}
	if _, err := c.StartInstallationLink(ctx, "t"); err == nil {
		t.Error("start link")
	}
}
