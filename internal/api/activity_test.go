package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

const activityJSON = `{"events":[{"id":"1","occurred_at":"2026-01-01T00:00:00Z","category":"login","action":"USER.LOGIN","description":"Signed in","result":"success","error_code":null,"actor":{"type":"customer","label":"Customer","email":null,"api_key_id":null},"auth_method":"oidc_token","target":null,"ip":null,"user_agent":null,"request_id":null,"session_ref":null}],"next_cursor":"next","scope":"workspace","retention_days":180}`
const auditJSON = `{"period_days":30,"generated_at":"2026-01-01T00:00:00Z","history_recorded":false,"components":[{"id":"runners","name":"Runners","state":"unknown","detail":"No signal","last_signal_at":null,"uptime_percent":null}],"assurances":{"jobs_run":1,"fresh_microvms":1,"destroyed":1,"clean_teardowns":1,"forced_stops":0,"awaiting_teardown":0},"evidence":[]}`

func TestActivityWireContract(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/customer/tenants/t/activity":
			if r.URL.Query().Get("actor_type") != "api_key" || r.URL.Query().Get("limit") != "4" {
				t.Error(r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(activityJSON))
		case "/v1/customer/tenants/t/activity/export":
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			_, _ = w.Write([]byte("occurred_at,category,action,description,result,error_code,actor_type,actor,actor_email,api_key_id,auth_method,target_type,target_id,ip,user_agent,request_id,session_ref\n"))
		case "/v1/customer/tenants/t/audit-status":
			if r.URL.Query().Get("period_days") != "30" {
				t.Error(r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(auditJSON))
		default:
			t.Error(r.URL.Path)
		}
	})
	ctx := context.Background()
	if p, e := c.Activity(ctx, "t", ActivityFilter{ActorType: "api_key", Limit: 4}); e != nil || p.NextCursor == nil {
		t.Fatalf("%+v %v", p, e)
	}
	if csv, e := c.ExportActivity(ctx, "t", ActivityFilter{Limit: 9, Cursor: "x"}); e != nil || !strings.HasPrefix(csv, "occurred_at,") {
		t.Fatalf("%q %v", csv, e)
	}
	if s, e := c.AuditStatus(ctx, "t", 30); e != nil || s.Components[0].UptimePercent != nil {
		t.Fatalf("%+v %v", s, e)
	}
}
func TestActivityCSVRejectsInvalidSuccess(t *testing.T) {
	for _, tc := range []struct{ content, body string }{{"application/json", "{}"}, {"text/csvx", "{}"}, {"text/csv", "occurred_at,category\n"}, {"text/csv", "occurred_at,category,action,description,result,error_code,actor_type,actor,actor_email,api_key_id,auth_method,target_type,target_id,ip,user_agent,request_id,session_ref\n\"unterminated"}, {"text/csv", "occurred_at,category,action,description,result,error_code,actor_type,actor,actor_email,api_key_id,auth_method,target_type,target_id,ip,user_agent,request_id,session_ref\na,b\n"}} {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", tc.content)
			_, _ = w.Write([]byte(tc.body))
		})
		if _, e := c.ExportActivity(context.Background(), "t", ActivityFilter{}); e == nil {
			t.Fatal("accepted invalid CSV")
		}
	}
}

func TestActivityCSVAllowsLargeValidExportAndRejectsOverflow(t *testing.T) {
	header := "occurred_at,category,action,description,result,error_code,actor_type,actor,actor_email,api_key_id,auth_method,target_type,target_id,ip,user_agent,request_id,session_ref\n"
	row := "2026-01-01T00:00:00Z,general,USER.LOGIN," + strings.Repeat("d", 500) + ",success,,customer,label,,,oidc_token,,,," + strings.Repeat("u", 512) + ",,\n"
	large := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(header))
		for range 10000 {
			_, _ = w.Write([]byte(row))
		}
	})
	if csv, err := large.ExportActivity(context.Background(), "t", ActivityFilter{}); err != nil || len(csv) <= maxBodyBytes {
		t.Fatalf("len=%d err=%v", len(csv), err)
	}
	overflow := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		chunk := strings.Repeat("x", 1<<20)
		for range 65 {
			_, _ = w.Write([]byte(chunk))
		}
	})
	if _, err := overflow.ExportActivity(context.Background(), "t", ActivityFilter{}); err == nil {
		t.Fatal("accepted oversized CSV")
	}
}

func TestActivityAcceptsEmptyDescription(t *testing.T) {
	body := strings.Replace(activityJSON, `"description":"Signed in"`, `"description":""`, 1)
	client := newTestClient(t, respond(http.StatusOK, body))
	if _, err := client.Activity(context.Background(), "t", ActivityFilter{}); err != nil {
		t.Fatal(err)
	}
}

func TestAuditRunningCount(t *testing.T) {
	for _, tc := range []struct {
		value   string
		want    int64
		invalid bool
	}{
		{"", 0, false}, {`,"running":4`, 4, false}, {`,"running":0`, 0, false},
		{`,"running":null`, 0, true}, {`,"running":-1`, 0, true}, {`,"running":"4"`, 0, true},
	} {
		body := strings.Replace(auditJSON, `"awaiting_teardown":0`, `"awaiting_teardown":0`+tc.value, 1)
		c := newTestClient(t, respond(http.StatusOK, body))
		status, err := c.AuditStatus(context.Background(), "t", 30)
		if tc.invalid {
			if err == nil {
				t.Fatal("accepted invalid running count")
			}
			continue
		}
		if err != nil || status.Assurances.Running != tc.want {
			t.Fatalf("%v, %v", status, err)
		}
	}
}
