package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const apiClosureJSON = `{"id":"c1","status":"pending","reason":"reason","requested_by":{"email":null},"requested_at":"2026-01-01T00:00:00Z","decided_at":null,"decided_by_type":null,"decision_note":null,"version":1}`

func TestClosureWireContract(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.EscapedPath() {
		case "GET /v1/customer/tenants/t%2F1/closure-requests":
			_, _ = w.Write([]byte(`{"requests":[` + apiClosureJSON + `]}`))
		case "POST /v1/customer/tenants/t%2F1/closure-requests":
			var v ClosureRequestInput
			_ = json.NewDecoder(r.Body).Decode(&v)
			if v.Reason != "reason" || v.IdempotencyKey != "12345678" {
				t.Error(v)
			}
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"request":` + apiClosureJSON + `}`))
		case "POST /v1/customer/tenants/t%2F1/closure-requests/c%2F1/cancel":
			var v map[string]int64
			_ = json.NewDecoder(r.Body).Decode(&v)
			if v["expected_version"] != 1 {
				t.Error(v)
			}
			_, _ = w.Write([]byte(`{"request":` + apiClosureJSON + `}`))
		default:
			t.Errorf("%s %s", r.Method, r.URL.EscapedPath())
		}
	})
	ctx := context.Background()
	if v, e := c.ClosureRequests(ctx, "t/1"); e != nil || v[0].RequestedBy.Email != nil {
		t.Fatal(v, e)
	}
	if _, e := c.RequestClosure(ctx, "t/1", ClosureRequestInput{"reason", "12345678"}); e != nil {
		t.Fatal(e)
	}
	if _, e := c.CancelClosure(ctx, "t/1", "c/1", 1); e != nil {
		t.Fatal(e)
	}
}
func TestClosureRejectsMalformed(t *testing.T) {
	for _, call := range []func(*Client) error{func(c *Client) error { _, e := c.ClosureRequests(context.Background(), "t"); return e }, func(c *Client) error {
		_, e := c.RequestClosure(context.Background(), "t", ClosureRequestInput{})
		return e
	}, func(c *Client) error { _, e := c.CancelClosure(context.Background(), "t", "c", 1); return e }} {
		c := newTestClient(t, respond(200, `{}`))
		if call(c) == nil {
			t.Fatal("accepted malformed")
		}
	}
}

func TestClosureRequiresNullableFields(t *testing.T) {
	body := strings.Replace(apiClosureJSON, `,"decision_note":null`, "", 1)
	c := newTestClient(t, respond(http.StatusOK, `{"requests":[`+body+`]}`))
	if _, err := c.ClosureRequests(context.Background(), "t"); err == nil {
		t.Fatal("accepted missing nullable field")
	}
}
