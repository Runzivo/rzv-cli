package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

const volID = "vol_0123456789abcdef01234567"

const diskJSON = `{"id":"vol_0123456789abcdef01234567","name":"data","size_gib":10,"status":"available","attached_sandbox_id":null,"mount_path":"/mnt/disk","created_at":"2026-11-01T10:00:00Z","deleted_at":null,"usage":{"gb_hours":21.5,"credits_used":0.36}}`

func attachedDisk() string {
	return strings.Replace(diskJSON, `"attached_sandbox_id":null`, `"attached_sandbox_id":"`+sbxID+`"`, 1)
}

func TestDisksListShowsQuotaAndAttachment(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/disks", 200, `{"disks":[`+attachedDisk()+`],"quota":{"quota_gib":64,"used_gib":10,"max_disks":5,"disks":1},"min_size_gib":1,"max_size_gib":64}`)
	code, out, _ := h.run("disks", "list")
	if code != 0 || !strings.Contains(out, volID) || !strings.Contains(out, "10 GiB") || !strings.Contains(out, sbxID) || !strings.Contains(out, "21.5 GB-h") || !strings.Contains(out, "Quota: 10 of 64 GiB used, 1 of 5 disks.") {
		t.Fatalf("list: %d %q", code, out)
	}
	code, out, _ = h.run("disks", "list", "--json")
	var listed struct {
		Disks []map[string]any `json:"disks"`
		Quota map[string]any   `json:"quota"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &listed) != nil || len(listed.Disks) != 1 || listed.Quota["quota_gib"] != float64(64) {
		t.Fatalf("list --json: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/disks", 200, `{"disks":[],"quota":{"quota_gib":0,"used_gib":0,"max_disks":5,"disks":0},"min_size_gib":1,"max_size_gib":64}`)
	if code, out, _ := h.run("disks", "list"); code != 0 || !strings.Contains(out, "No persistent disks") || !strings.Contains(out, "no persistent disk quota") {
		t.Fatalf("empty: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/disks", 503, `{"error":{"code":"persistent_disks_unavailable"}}`)
	if code, _, errOut := h.run("disks", "list"); code != exitUnavailable || !strings.Contains(errOut, "not available on this Runzivo environment") {
		t.Fatalf("unavailable: %d %q", code, errOut)
	}
}

func TestDisksCreateAttachDetachDelete(t *testing.T) {
	h := selectedHarness(t)
	var bodies []map[string]any
	record := func(status int, reply string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &body)
			bodies = append(bodies, body)
			w.WriteHeader(status)
			_, _ = w.Write([]byte(reply))
		}
	}
	h.routes["POST /v1/customer/tenants/t-1/disks"] = record(201, diskJSON)
	code, out, _ := h.run("disks", "create", "--name", "data", "--size-gib", "10", "--idempotency-key", "op-disk-0001")
	if code != 0 || !strings.Contains(out, "Created disk data ["+volID+"]: available") || !strings.Contains(out, "--disk "+volID) || bodies[0]["size_gib"] != float64(10) || bodies[0]["client_operation_id"] != "op-disk-0001" {
		t.Fatalf("create: %d %q %v", code, out, bodies)
	}
	h.routes["POST /v1/customer/tenants/t-1/disks/"+volID+"/attach"] = record(200, attachedDisk())
	code, out, _ = h.run("disks", "attach", volID, "--sandbox", sbxID)
	if code != 0 || !strings.Contains(out, "Attached disk") || !strings.Contains(out, "/mnt/disk") || bodies[1]["sandbox_id"] != sbxID {
		t.Fatalf("attach: %d %q %v", code, out, bodies)
	}
	h.json("POST /v1/customer/tenants/t-1/disks/"+volID+"/detach", 200, diskJSON)
	if code, out, _ := h.run("disks", "detach", volID); code != 0 || !strings.Contains(out, "Detached disk") || !strings.Contains(out, "Not attached") {
		t.Fatalf("detach: %d %q", code, out)
	}
	h.json("DELETE /v1/customer/tenants/t-1/disks/"+volID, 200, strings.Replace(diskJSON, `"status":"available"`, `"status":"deleting"`, 1))
	if code, out, _ := h.run("disks", "delete", volID); code != 0 || !strings.Contains(out, "Deleting disk data") {
		t.Fatalf("delete: %d %q", code, out)
	}
	h.json("DELETE /v1/customer/tenants/t-1/disks/"+volID, 409, `{"error":{"code":"disk_attached"}}`)
	if code, _, errOut := h.run("disks", "delete", volID); code != exitFailure || !strings.Contains(errOut, "attached to a sandbox") {
		t.Fatalf("delete attached: %d %q", code, errOut)
	}
	h.json("POST /v1/customer/tenants/t-1/disks", 403, `{"error":{"code":"persistent_disks_not_entitled"}}`)
	if code, _, errOut := h.run("disks", "create", "--name", "x", "--size-gib", "1"); code != exitAccessDenied || !strings.Contains(errOut, "no persistent disk quota") {
		t.Fatalf("not entitled: %d %q", code, errOut)
	}
	for _, args := range [][]string{
		{"disks"}, {"disks", "bogus"},
		{"disks", "create", "--name", "x"},
		{"disks", "create", "--name", "x", "--size-gib", "0"},
		{"disks", "create", "--name", "x", "--size-gib", "65"},
		{"disks", "create", "--name", " ", "--size-gib", "5"},
		{"disks", "get", "vol_short"},
		{"disks", "attach", volID},
		{"disks", "attach", volID, "--sandbox", "sbx_bad"},
		{"disks", "delete"},
	} {
		if code, _, _ := h.run(args...); code != exitUsage {
			t.Errorf("%v exit %d, want usage", args, code)
		}
	}
}

func TestSandboxesCreateWithADisk(t *testing.T) {
	h := selectedHarness(t)
	var body map[string]any
	h.routes["POST /v1/customer/tenants/t-1/sandboxes"] = func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(strings.Replace(sandboxWith("starting"), `"credits_used":20.5`, `"credits_used":0,"disk_id":"`+volID+`"`, 1)))
	}
	code, out, _ := h.run("sandboxes", "create", "--name", "dev", "--disk", volID)
	if code != 0 || body["disk_id"] != volID || !strings.Contains(out, "Persistent disk: "+volID) {
		t.Fatalf("create with disk: %d %q %v", code, out, body)
	}
	if code, _, _ := h.run("sandboxes", "create", "--name", "dev", "--disk", "vol_bad"); code != exitUsage {
		t.Fatalf("bad disk id exit %d", code)
	}
	h.json("POST /v1/customer/tenants/t-1/sandboxes", 404, `{"error":{"code":"disk_not_found"}}`)
	if code, _, errOut := h.run("sandboxes", "create", "--name", "dev", "--disk", volID); code == 0 || !strings.Contains(errOut, "persistent disk was not found") {
		t.Fatalf("missing disk: %d %q", code, errOut)
	}
}
