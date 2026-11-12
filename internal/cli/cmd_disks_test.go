package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
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
		Disks      []map[string]any `json:"disks"`
		Quota      map[string]any   `json:"quota"`
		MinSizeGiB *int             `json:"min_size_gib"`
		MaxSizeGiB *int             `json:"max_size_gib"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &listed) != nil || len(listed.Disks) != 1 || listed.Quota["quota_gib"] != float64(64) ||
		listed.MinSizeGiB == nil || *listed.MinSizeGiB != 1 || listed.MaxSizeGiB == nil || *listed.MaxSizeGiB != 64 {
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
	if code, out, _ := h.run("disks", "delete", volID); code != 0 || !strings.Contains(out, "Deleting disk data") || !strings.Contains(out, "shows as deleting until its server confirms") {
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
		{"disks", "create", "--name", "x", "--size-gib", "16385"},
		{"disks", "create", "--name", "x", "--size-gib", "1.5"},
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

func TestDisksCreateUsesContractSizeRange(t *testing.T) {
	h := selectedHarness(t)
	var sizes []int
	h.routes["POST /v1/customer/tenants/t-1/disks"] = func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SizeGiB int `json:"size_gib"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, body.SizeGiB)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(strings.Replace(diskJSON, `"size_gib":10`, `"size_gib":`+strconv.Itoa(body.SizeGiB), 1)))
	}
	for _, size := range []int{1, 64, 65, 512, 16384} {
		before := len(sizes)
		code, out, errOut := h.run("disks", "create", "--name", "data", "--size-gib", strconv.Itoa(size))
		if code != 0 || len(sizes) != before+1 || sizes[before] != size || !strings.Contains(out, "Size: "+strconv.Itoa(size)+" GiB") {
			t.Fatalf("size %d: %d %q %q, sent %v", size, code, out, errOut, sizes)
		}
	}
	// Core, rather than a fixed CLI quota, decides what fits now.
	h.json("POST /v1/customer/tenants/t-1/disks", 409, `{"error":{"code":"disk_quota_exceeded"}}`)
	if code, _, errOut := h.run("disks", "create", "--name", "data", "--size-gib", "512"); code != exitFailure || !strings.Contains(errOut, "workspace's disk quota") {
		t.Fatalf("Core quota refusal: %d %q", code, errOut)
	}
}

func TestDisksListQuotaModes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		quota string
		max   int
		want  string
	}{
		{"older server", `{"quota_gib":0,"used_gib":0,"max_disks":5,"disks":0}`, 64, "Ask Runzivo support for one."},
		{"grant", `{"quota_gib":0,"used_gib":0,"max_disks":5,"disks":0,"mode":"grant","region_capacity_gib":0,"region_free_gib":0}`, 64, "Ask Runzivo support for one."},
		{"capacity", `{"quota_gib":288,"used_gib":32,"max_disks":5,"disks":1,"mode":"capacity","region_capacity_gib":512,"region_free_gib":256}`, 256, "Capacity: 32 of 288 GiB used, 1 of 5 disks.\nRegion: 256 of 512 GiB free."},
		{"full region", `{"quota_gib":0,"used_gib":0,"max_disks":5,"disks":0,"mode":"capacity","region_capacity_gib":512,"region_free_gib":0}`, 0, "No persistent disk capacity is available in this region."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := selectedHarness(t)
			h.json("GET /v1/customer/tenants/t-1/disks", 200, `{"disks":[],"quota":`+tc.quota+`,"min_size_gib":1,"max_size_gib":`+strconv.Itoa(tc.max)+`}`)
			if code, out, _ := h.run("disks", "list"); code != 0 || !strings.Contains(out, tc.want) || (strings.Contains(tc.quota, `"capacity"`) && strings.Contains(out, "Ask Runzivo support")) {
				t.Fatalf("list: %d %q", code, out)
			}
			var want map[string]any
			if err := json.Unmarshal([]byte(tc.quota), &want); err != nil {
				t.Fatal(err)
			}
			var listed struct {
				Quota      map[string]any `json:"quota"`
				MaxSizeGiB int            `json:"max_size_gib"`
			}
			code, out, _ := h.run("disks", "list", "--json")
			if code != 0 || json.Unmarshal([]byte(out), &listed) != nil || listed.MaxSizeGiB != tc.max || len(listed.Quota) != len(want) {
				t.Fatalf("list --json: %d %q", code, out)
			}
			for key, value := range want {
				if listed.Quota[key] != value {
					t.Errorf("quota.%s = %v, want %v", key, listed.Quota[key], value)
				}
			}
		})
	}
}

func TestDisksScheduledDeletion(t *testing.T) {
	h := selectedHarness(t)
	scheduled := strings.Replace(diskJSON, `"usage":`, `"scheduled_deletion":{"delete_after":"2026-12-01T09:30:00Z","reason":"unpaid"},"usage":`, 1)
	closed := strings.Replace(scheduled, `"reason":"unpaid"`, `"reason":"workspace_closed"`, 1)
	h.json("GET /v1/customer/tenants/t-1/disks", 200, `{"disks":[`+scheduled+`],"quota":{"quota_gib":64,"used_gib":10,"max_disks":5,"disks":1},"min_size_gib":1,"max_size_gib":64}`)
	if code, out, _ := h.run("disks", "list"); code != 0 || !strings.Contains(out, volID+": Scheduled for deletion on 2026-12-01 09:30 UTC (unpaid)") {
		t.Fatalf("list: %d %q", code, out)
	}
	code, out, _ := h.run("disks", "list", "--json")
	var listed struct {
		Disks []struct {
			ScheduledDeletion map[string]string `json:"scheduled_deletion"`
		} `json:"disks"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &listed) != nil || len(listed.Disks) != 1 ||
		listed.Disks[0].ScheduledDeletion["delete_after"] != "2026-12-01T09:30:00Z" || listed.Disks[0].ScheduledDeletion["reason"] != "unpaid" {
		t.Fatalf("list --json: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/disks/"+volID, 200, closed)
	if code, out, _ := h.run("disks", "get", volID); code != 0 || !strings.Contains(out, "Scheduled for deletion on 2026-12-01 09:30 UTC (workspace closed)") {
		t.Fatalf("get: %d %q", code, out)
	}
	code, out, _ = h.run("disks", "get", volID, "--json")
	var got struct {
		Disk map[string]any `json:"disk"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil || got.Disk["scheduled_deletion"] == nil {
		t.Fatalf("get --json: %d %q", code, out)
	}
	// Null, and absent (an older Core), both read as no scheduled deletion.
	for _, body := range []string{strings.Replace(diskJSON, `"usage":`, `"scheduled_deletion":null,"usage":`, 1), diskJSON} {
		h.json("GET /v1/customer/tenants/t-1/disks/"+volID, 200, body)
		if code, out, _ := h.run("disks", "get", volID); code != 0 || strings.Contains(out, "Scheduled for deletion") {
			t.Fatalf("not scheduled: %d %q", code, out)
		}
		code, out, _ := h.run("disks", "get", volID, "--json")
		got.Disk = nil
		if code != 0 || json.Unmarshal([]byte(out), &got) != nil {
			t.Fatalf("not scheduled --json: %d %q", code, out)
		}
		if value, ok := got.Disk["scheduled_deletion"]; !ok || value != nil {
			t.Fatalf("not scheduled --json: %q", out)
		}
	}
	// A deadline that is not RFC 3339 is a malformed response.
	h.json("GET /v1/customer/tenants/t-1/disks/"+volID, 200, strings.Replace(scheduled, "2026-12-01T09:30:00Z", "soon", 1))
	if code, _, _ := h.run("disks", "get", volID); code == 0 {
		t.Fatal("a malformed delete_after was accepted")
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

func TestSandboxesDiskPreservesLifetimeSizeAndPlanHandling(t *testing.T) {
	h := selectedHarness(t)
	var body map[string]any
	h.routes["POST /v1/customer/tenants/t-1/sandboxes"] = func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.WriteHeader(201)
		_, _ = w.Write([]byte(strings.Replace(sandboxLifetimeJSON("starting", 24, "null", 86400), `"credits_used":20.5`, `"credits_used":20.5,"disk_id":"`+volID+`"`, 1)))
	}
	code, out, errOut := h.run("sandboxes", "create", "--name", "dev", "--disk", volID, "--size", "16vcpu-64gb", "--disk-gib", "40", "--lifetime", "24h")
	if code != 0 || body["disk_id"] != volID || body["size"] != "16vcpu-64gb" || body["disk_gib"] != float64(40) || body["lifetime_hours"] != float64(24) || !strings.Contains(out, "Lifetime: 24h") || !strings.Contains(out, "Persistent disk: "+volID) {
		t.Fatalf("create options: %d %q %q %v", code, out, errOut, body)
	}
	// The defaults remain omitted for older services.
	body = nil
	if code, _, errOut := h.run("sandboxes", "create", "--name", "dev", "--lifetime", "8h"); code != 0 || body["disk_id"] != nil || body["lifetime_hours"] != nil {
		t.Fatalf("default options: %d %q %v", code, errOut, body)
	}
	h.json("POST /v1/customer/tenants/t-1/sandboxes", 403, `{"error":{"code":"plan_required","required_plan":"team"}}`)
	if code, _, errOut := h.run("sandboxes", "create", "--name", "dev", "--disk", volID, "--size", "16vcpu-64gb"); code != exitAccessDenied || !strings.Contains(errOut, "16vcpu-64gb needs the Team plan.") {
		t.Fatalf("plan refusal with disk: %d %q", code, errOut)
	}
	h.json("POST /v1/customer/tenants/t-1/sandboxes", 400, `{"error":{"code":"invalid_request"}}`)
	h.json("GET /v1/customer/tenants/t-1/sandboxes", 200, `{"sandboxes":[],"max_running":2}`)
	if code, _, errOut := h.run("sandboxes", "create", "--name", "dev", "--disk", volID, "--lifetime", "24h"); code != exitUsage || !strings.Contains(errOut, "doesn't support choosing a lifetime yet") {
		t.Fatalf("older server with disk: %d %q", code, errOut)
	}
}

func TestDisksNewRefusals(t *testing.T) {
	h := selectedHarness(t)
	for code, want := range map[string]string{
		"disk_host_unsupported":  "This sandbox's files are on a server that cannot use persistent disks. Attach the disk to a new sandbox instead.",
		"disk_image_unsupported": "The sandbox image on this server cannot mount persistent disks yet.",
		"disk_concurrent_change": "The disk changed while the command ran. Run it again.",
	} {
		h.json("POST /v1/customer/tenants/t-1/disks/"+volID+"/attach", 409, `{"error":{"code":"`+code+`"}}`)
		if exit, _, errOut := h.run("disks", "attach", volID, "--sandbox", sbxID); exit != exitFailure || !strings.Contains(errOut, want) {
			t.Errorf("%s: %d %q", code, exit, errOut)
		}
	}
}

func TestDisksWrongStateReadsTheDisk(t *testing.T) {
	h := selectedHarness(t)
	scheduled := func(when string) string {
		return strings.Replace(diskJSON, `"usage":`, `"scheduled_deletion":{"delete_after":"`+when+`","reason":"unpaid"},"usage":`, 1)
	}
	past := "That disk is past its scheduled deletion time and is being deleted."
	deleting := "That disk is being deleted."
	combined := "The disk is being deleted, or is past its scheduled deletion time."
	for _, tc := range []struct {
		name   string
		status int
		disk   string
		want   string
	}{
		{"past deadline", 200, scheduled("2020-01-01T00:00:00Z"), past},
		{"future deadline", 200, scheduled("2099-01-01T00:00:00Z"), deleting},
		{"not scheduled", 200, strings.Replace(diskJSON, `"status":"available"`, `"status":"deleting"`, 1), deleting},
		{"read fails", 404, `{"error":{"code":"disk_not_found"}}`, combined},
	} {
		h.json("GET /v1/customer/tenants/t-1/disks/"+volID, tc.status, tc.disk)
		h.json("POST /v1/customer/tenants/t-1/disks/"+volID+"/attach", 409, `{"error":{"code":"disk_wrong_state"}}`)
		if exit, _, errOut := h.run("disks", "attach", volID, "--sandbox", sbxID); exit != exitFailure || !strings.Contains(errOut, tc.want) {
			t.Errorf("attach, %s: %d %q", tc.name, exit, errOut)
		}
		h.json("POST /v1/customer/tenants/t-1/sandboxes", 409, `{"error":{"code":"disk_wrong_state"}}`)
		if exit, _, errOut := h.run("sandboxes", "create", "--name", "dev", "--disk", volID); exit != exitFailure || !strings.Contains(errOut, tc.want) {
			t.Errorf("sandboxes create, %s: %d %q", tc.name, exit, errOut)
		}
	}
}

func TestSandboxesDiskState(t *testing.T) {
	h := selectedHarness(t)
	withDisk := strings.Replace(sandboxJSON, `"credits_used":20.5`, `"credits_used":20.5,"disk_id":"`+volID+`"`, 1)
	state := func(body string) string {
		return strings.Replace(withDisk, `"disk_id":`, `"disk_state":`+body+`,"disk_id":`, 1)
	}
	for _, tc := range []struct{ body, want string }{
		{state(`{"state":"unavailable","detail":"mount failed"}`), "Persistent disk: " + volID + ", not mounted (mount failed)"},
		{state(`{"state":"pending","detail":"waiting for the guest"}`), "Persistent disk: " + volID + ", checking (waiting for the guest)"},
		{state(`{"state":"pending","detail":null}`), "Persistent disk: " + volID + ", checking\n"},
		{state(`{"state":"mounted","detail":null}`), "Persistent disk: " + volID + ", mounted at /mnt/disk\n"},
		{state(`null`), "Persistent disk: " + volID + ", mounted at /mnt/disk while running"},
		{withDisk, "Persistent disk: " + volID + ", mounted at /mnt/disk while running"},
	} {
		h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, tc.body)
		if code, out, _ := h.run("sandboxes", "get", sbxID); code != 0 || !strings.Contains(out, tc.want) {
			t.Errorf("get %s: %d %q", tc.body, code, out)
		}
	}
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, state(`{"state":"unavailable","detail":"mount failed"}`))
	code, out, _ := h.run("sandboxes", "get", sbxID, "--json")
	var got struct {
		Sandbox struct {
			DiskState map[string]any `json:"disk_state"`
		} `json:"sandbox"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil || got.Sandbox.DiskState["state"] != "unavailable" || got.Sandbox.DiskState["detail"] != "mount failed" {
		t.Fatalf("get --json: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/sandboxes", 200, `{"sandboxes":[`+state(`{"state":"pending","detail":null}`)+`],"max_running":2,"sizes":["2vcpu-8gb"]}`)
	code, out, _ = h.run("sandboxes", "list", "--json")
	var listed struct {
		Sandboxes []struct {
			DiskState map[string]any `json:"disk_state"`
		} `json:"sandboxes"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &listed) != nil || len(listed.Sandboxes) != 1 || listed.Sandboxes[0].DiskState["state"] != "pending" {
		t.Fatalf("list --json: %d %q", code, out)
	}
}
