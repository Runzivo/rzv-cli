package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestAIOperations(t *testing.T) {
	seen := map[string]bool{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.Method+" "+r.URL.RequestURI()] = true
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path[len(r.URL.Path)-5:] == "limit":
			_, _ = w.Write([]byte(`{"period_start":"a","period_end":"b","used_tokens":0,"used_cost_micros":null,"token_limit":null,"cost_limit_cents":null,"limited":false,"limit_kind":null}`))
		case r.URL.Path[len(r.URL.Path)-5:] == "calls":
			_, _ = w.Write([]byte(`{"calls":[],"next_cursor":null}`))
		case r.URL.Path[len(r.URL.Path)-7:] == "summary":
			_, _ = w.Write([]byte(`{"from":"a","to":"b","group_by":null,"totals":{"requests":0,"input_tokens":0,"output_tokens":0,"cached_tokens":0,"cost_estimate_micros":null,"unpriced_requests":0,"errors":0,"limited":0,"p50_latency_ms":null,"p95_latency_ms":null},"series":[]}`))
		case r.URL.Path[len(r.URL.Path)-8:] == "settings":
			_, _ = w.Write([]byte(`{"mode":"off","agents":{},"autonomy":"ask","monthly_token_limit":null,"monthly_cost_limit_cents":null,"currency":"USD","allowed_roles":[],"byo_available":true,"runzivo_available":false,"can_manage":true,"updated_at":null,"allowed_networks":[]}`))
		case r.URL.Path[len(r.URL.Path)-6:] == "models":
			_, _ = w.Write([]byte(`{"models":[],"fetched_at":"2026-10-21T00:00:00Z"}`))
		case r.URL.Path[len(r.URL.Path)-5:] == "/test":
			_, _ = w.Write([]byte(`{"status":"ok","latency_ms":1,"error_code":null}`))
		case r.URL.Path[len(r.URL.Path)-10:] == "/providers":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"providers":[]}`))
			} else {
				_, _ = w.Write([]byte(`{"provider_id":"p","kind":"example","label":"x","base_url":null,"key_hint":null,"has_key":false,"status":"ok","last_tested_at":null,"last_error":null,"created_by":null,"created_at":"a","updated_at":"a"}`))
			}
		default:
			_, _ = w.Write([]byte(`{"provider_id":"p","kind":"example","label":"x","base_url":null,"key_hint":null,"has_key":false,"status":"ok","last_tested_at":null,"last_error":null,"created_by":null,"created_at":"a","updated_at":"a"}`))
		}
	}))
	defer s.Close()
	c, err := New(s.URL, "token", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = c.AIProviders(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.CreateAIProvider(ctx, "x", AIProviderInput{Kind: "example"}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.UpdateAIProvider(ctx, "x", "p", AIProviderInput{}); err != nil {
		t.Fatal(err)
	}
	if err = c.DeleteAIProvider(ctx, "x", "p"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.TestAIProvider(ctx, "x", "p"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = c.AIModels(ctx, "x", "p", true); err != nil {
		t.Fatal(err)
	}
	if _, err = c.AISettings(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.UpdateAISettings(ctx, "x", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.AIUsageSummary(ctx, "x", url.Values{"group_by": {"day"}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = c.AICalls(ctx, "x", url.Values{"limit": {"2"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.AILimit(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"GET /v1/customer/tenants/x/ai/providers", "POST /v1/customer/tenants/x/ai/providers", "PUT /v1/customer/tenants/x/ai/providers/p", "DELETE /v1/customer/tenants/x/ai/providers/p", "POST /v1/customer/tenants/x/ai/providers/p/test", "GET /v1/customer/tenants/x/ai/providers/p/models?refresh=true", "GET /v1/customer/tenants/x/ai/settings", "PUT /v1/customer/tenants/x/ai/settings", "GET /v1/customer/tenants/x/ai/usage/summary?group_by=day", "GET /v1/customer/tenants/x/ai/usage/calls?limit=2", "GET /v1/customer/tenants/x/ai/usage/limit"} {
		if !seen[want] {
			t.Errorf("missing %s", want)
		}
	}
}
