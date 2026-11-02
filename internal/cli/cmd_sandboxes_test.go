package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const sandboxJSON = `{"id":"sbx_0123456789abcdef01234567","name":"dev","size":"2vcpu-8gb","disk_gib":20,"status":"running","error":null,"created_at":"2026-10-29T09:00:00Z","started_at":"2026-10-29T09:00:05Z","expires_at":"2026-10-29T17:00:05Z","remaining_seconds":27000,"runtime_seconds":1200,"credits_used":20.5}`

const sbxID = "sbx_0123456789abcdef01234567"

func sandboxWith(status string) string {
	return strings.Replace(sandboxJSON, `"status":"running"`, `"status":"`+status+`"`, 1)
}

func TestSandboxesListAndGet(t *testing.T) {
	fixedZone(t, "EST", -5*3600)
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/sandboxes", 200, `{"sandboxes":[`+sandboxJSON+`,`+strings.Replace(sandboxWith("stopped"), `"id":"sbx_0123456789abcdef01234567","name":"dev"`, `"id":"sbx_aaaaaaaaaaaaaaaaaaaaaaaa","name":"build"`, 1)+`],"max_running":2,"sizes":["2vcpu-8gb"]}`)
	code, out, _ := h.run("sandboxes", "list")
	if code != 0 || !strings.Contains(out, "ID") || !strings.Contains(out, sbxID) || !strings.Contains(out, "7h 30m") || !strings.Contains(out, "20.5 min") || !strings.Contains(out, "This workspace can run 2 at once.") || !strings.Contains(out, "Sizes on offer here: 2vcpu-8gb.") || !strings.Contains(out, "build") {
		t.Fatalf("list: %d %q", code, out)
	}
	code, out, _ = h.run("sandboxes", "list", "--json")
	var listed struct {
		Sandboxes  []map[string]any `json:"sandboxes"`
		MaxRunning int              `json:"max_running"`
		Sizes      []string         `json:"sizes"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &listed) != nil || len(listed.Sandboxes) != 2 || listed.MaxRunning != 2 || len(listed.Sizes) != 1 || listed.Sandboxes[0]["id"] != sbxID {
		t.Fatalf("list --json: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/sandboxes", 200, `{"sandboxes":[],"max_running":2,"sizes":["2vcpu-8gb","4vcpu-16gb"]}`)
	if code, out, _ := h.run("sandboxes", "list"); code != 0 || !strings.Contains(out, "No sandboxes in workspace t-1") || !strings.Contains(out, "Sizes on offer here: 2vcpu-8gb, 4vcpu-16gb.") {
		t.Fatalf("empty: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/sandboxes", 200, `{"sandboxes":[{"id":"x"}],"max_running":2}`)
	if code, _, _ := h.run("sandboxes", "list"); code == 0 {
		t.Fatal("a sandbox without its fields was accepted")
	}
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxJSON)
	if code, out, _ := h.run("sandboxes", "get", sbxID); code != 0 || !strings.Contains(out, "dev ["+sbxID+"]: running") || !strings.Contains(out, "Time left: 7h 30m") {
		t.Fatalf("get: %d %q", code, out)
	}
	for _, args := range [][]string{{"sandboxes", "get"}, {"sandboxes", "get", "dev"}, {"sandboxes", "get", "sbx_short"}, {"sandboxes", "bogus"}, {"sandboxes"}} {
		if code, _, _ := h.run(args...); code != exitUsage {
			t.Errorf("%v exit %d, want usage", args, code)
		}
	}
}

// fixedZone makes the local time zone a fixed one for the test, so the
// expiry text does not depend on the machine running it.
func fixedZone(t *testing.T, name string, offset int) {
	t.Helper()
	saved := localZone
	zone := time.FixedZone(name, offset)
	localZone = func() *time.Location { return zone }
	t.Cleanup(func() { localZone = saved })
}

func sandboxLifetimeJSON(status string, hours int, expiresAt string, remaining int) string {
	s := sandboxWith(status)
	s = strings.Replace(s, `"expires_at":"2026-10-29T17:00:05Z"`, `"expires_at":`+expiresAt, 1)
	s = strings.Replace(s, `"remaining_seconds":27000`, `"remaining_seconds":`+strconv.Itoa(remaining), 1)
	if hours > 0 {
		s = strings.Replace(s, `"status"`, `"lifetime_hours":`+strconv.Itoa(hours)+`,"status"`, 1)
	}
	return s
}

func TestSandboxesCreateStartStopDelete(t *testing.T) {
	h := selectedHarness(t)
	var bodies []map[string]any
	h.routes["POST /v1/customer/tenants/t-1/sandboxes"] = func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		bodies = append(bodies, body)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(sandboxWith("starting")))
	}
	code, out, _ := h.run("sandboxes", "create", "--name", "dev")
	if code != 0 || !strings.Contains(out, "Created sandbox dev ["+sbxID+"]: starting") || bodies[0]["name"] != "dev" || bodies[0]["size"] != "2vcpu-8gb" || bodies[0]["disk_gib"] != nil || bodies[0]["client_operation_id"] != nil {
		t.Fatalf("create: %d %q %v", code, out, bodies)
	}
	code, _, _ = h.run("sandboxes", "create", "--name", "big", "--size", "4vcpu-16gb", "--disk-gib", "40", "--idempotency-key", "op-0123456789")
	if code != 0 || bodies[1]["size"] != "4vcpu-16gb" || bodies[1]["disk_gib"] != float64(40) || bodies[1]["client_operation_id"] != "op-0123456789" {
		t.Fatalf("create with options: %d %v", code, bodies[1])
	}
	for i, size := range []string{"8vcpu-32gb", "16vcpu-64gb"} {
		if code, _, _ := h.run("sandboxes", "create", "--name", "large", "--size", size); code != 0 || bodies[2+i]["size"] != size {
			t.Fatalf("create %s: %d %v", size, code, bodies)
		}
	}
	for _, args := range [][]string{
		{"sandboxes", "create"},
		{"sandboxes", "create", "--name", "x", "--size", "1vcpu"},
		{"sandboxes", "create", "--name", "x", "--size", "32vcpu-128gb"},
		{"sandboxes", "create", "--name", "x", "--disk-gib", "4"},
		{"sandboxes", "create", "--name", "x", "--disk-gib", "65"},
		{"sandboxes", "create", "--name", "x", "--idempotency-key", "short"},
		{"sandboxes", "create", "--name", strings.Repeat("n", 81)},
	} {
		if code, _, _ := h.run(args...); code != exitUsage {
			t.Errorf("%v exit %d, want usage", args, code)
		}
	}
	if len(bodies) != 4 {
		t.Fatalf("a refused create reached the API: %d calls", len(bodies))
	}
	h.json("POST /v1/customer/tenants/t-1/sandboxes/"+sbxID+"/start", 200, sandboxWith("starting"))
	h.json("POST /v1/customer/tenants/t-1/sandboxes/"+sbxID+"/stop", 200, sandboxWith("stopping"))
	h.json("DELETE /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxWith("deleting"))
	for verb, want := range map[string]string{"start": "Starting sandbox dev", "stop": "Stopping sandbox dev", "delete": "Deleting sandbox dev"} {
		if code, out, _ := h.run("sandboxes", verb, sbxID); code != 0 || !strings.Contains(out, want) {
			t.Errorf("%s: %d %q", verb, code, out)
		}
	}
	if code, out, _ := h.run("sandboxes", "stop", sbxID, "--json"); code != 0 || !strings.Contains(out, `"status":"stopping"`) {
		t.Errorf("stop --json: %d %q", code, out)
	}
}

func TestSandboxesExplainRefusalsWithoutHostDetails(t *testing.T) {
	h := selectedHarness(t)
	cases := []struct {
		verb   string
		status int
		code   string
		exit   int
		text   string
	}{
		{"start", 404, "sandbox_not_found", exitNotFound, "sandbox was not found"},
		{"start", 409, "sandbox_limit_reached", exitFailure, "maximum number of sandboxes"},
		{"start", 409, "credit_exhausted", exitFailure, "out of credit"},
		{"stop", 409, "sandbox_wrong_state", exitFailure, "not in a state"},
		{"start", 409, "sandbox_expired", exitFailure, "reached its lifetime"},
		{"start", 409, "no_capacity", exitFailure, "No capacity"},
		{"start", 409, "size_unavailable", exitFailure, "not offered on this Runzivo environment"},
		{"start", 409, "workspace_blocked", exitAccessDenied, "blocked"},
		{"start", 503, "sandboxes_unavailable", exitUnavailable, "not available on this Runzivo environment"},
		{"start", 403, "api_key_scope_required", exitAccessDenied, "scope does not allow"},
		{"start", 403, "permission_required", exitAccessDenied, "does not allow"},
	}
	for _, c := range cases {
		h.json("POST /v1/customer/tenants/t-1/sandboxes/"+sbxID+"/"+c.verb, c.status, `{"error":{"code":"`+c.code+`"}}`)
		code, _, errOut := h.run("sandboxes", c.verb, sbxID)
		if code != c.exit || !strings.Contains(errOut, c.text) {
			t.Errorf("%s %s: exit %d %q, want %d containing %q", c.verb, c.code, code, errOut, c.exit, c.text)
		}
	}
	h.json("POST /v1/customer/tenants/t-1/sandboxes", 409, `{"error":{"code":"sandbox_name_taken"}}`)
	if code, _, errOut := h.run("sandboxes", "create", "--name", "dev"); code != exitFailure || !strings.Contains(errOut, "already exists") {
		t.Errorf("name taken: %d %q", code, errOut)
	}
}

func execServer(h *harness, events string, status int, seen *map[string]any) {
	h.routes["POST /v1/customer/tenants/t-1/sandboxes/"+sbxID+"/exec"] = func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, seen)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(events))
	}
}

func chunk(kind, text string) string {
	return `{"type":"` + kind + `","data":"` + base64.StdEncoding.EncodeToString([]byte(text)) + `"}` + "\n"
}

func TestSandboxesExecStreamsOutputAndExitsWithTheCommandStatus(t *testing.T) {
	h := selectedHarness(t)
	var seen map[string]any
	execServer(h, chunk("stdout", "hello ")+chunk("stderr", "warning\n")+chunk("stdout", "world\n")+`{"type":"exit","exit_code":0,"timed_out":false}`+"\n", 200, &seen)
	code, out, errOut := h.run("sandboxes", "exec", sbxID, "--cwd", "/workspace/app", "--timeout", "60", "--env", "A=1,B=two", "--", "echo", "hello", "world")
	if code != 0 || out != "hello world\n" || errOut != "warning\n" {
		t.Fatalf("exec: %d stdout %q stderr %q", code, out, errOut)
	}
	env, _ := seen["env"].(map[string]any)
	if seen["cmd"] != "'echo' 'hello' 'world'" || seen["cwd"] != "/workspace/app" || seen["timeout_seconds"] != float64(60) || env["A"] != "1" || env["B"] != "two" {
		t.Fatalf("exec request = %v", seen)
	}

	execServer(h, chunk("stdout", "partial")+`{"type":"exit","exit_code":3,"timed_out":false}`+"\n", 200, nil)
	if code, out, errOut := h.run("sandboxes", "exec", sbxID, "--", "false"); code != 3 || out != "partial" || !strings.Contains(errOut, "exited with status 3") {
		t.Fatalf("failing command: %d %q %q", code, out, errOut)
	}
	execServer(h, `{"type":"exit","exit_code":124,"timed_out":true}`+"\n", 200, nil)
	if code, _, errOut := h.run("sandboxes", "exec", sbxID, "--", "sleep", "9999"); code != 124 || !strings.Contains(errOut, "timed out") {
		t.Fatalf("timeout: %d %q", code, errOut)
	}
	// --json passes the events through, base64 data and all.
	execServer(h, chunk("stdout", "hi")+`{"type":"exit","exit_code":0,"timed_out":false}`+"\n", 200, nil)
	code, out, _ = h.run("sandboxes", "exec", sbxID, "--json", "--", "echo", "hi")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if code != 0 || len(lines) != 2 || !strings.Contains(lines[0], `"data":"aGk="`) || !strings.Contains(lines[1], `"type":"exit"`) {
		t.Fatalf("exec --json: %d %q", code, out)
	}
	// A stream that stops before the exit event is not a success.
	execServer(h, chunk("stdout", "cut off"), 200, nil)
	if code, _, _ := h.run("sandboxes", "exec", sbxID, "--", "echo"); code == 0 {
		t.Fatal("a stream without an exit event was accepted")
	}
	execServer(h, "not json\n", 200, nil)
	if code, _, _ := h.run("sandboxes", "exec", sbxID, "--", "echo"); code == 0 {
		t.Fatal("a malformed stream was accepted")
	}
	execServer(h, `{"error":{"code":"sandbox_wrong_state"}}`, 409, nil)
	if code, _, errOut := h.run("sandboxes", "exec", sbxID, "--", "ls"); code != exitFailure || !strings.Contains(errOut, "not in a state") {
		t.Fatalf("exec on a stopped sandbox: %d %q", code, errOut)
	}
	execServer(h, `{"error":{"code":"sandbox_exec_unavailable"}}`, 503, nil)
	if code, _, errOut := h.run("sandboxes", "exec", sbxID, "--", "ls"); code != exitUnavailable || !strings.Contains(errOut, "not available") {
		t.Fatalf("exec while the relay is off: %d %q", code, errOut)
	}
	for _, args := range [][]string{
		{"sandboxes", "exec", sbxID},
		{"sandboxes", "exec", sbxID, "--timeout", "0", "--", "ls"},
		{"sandboxes", "exec", sbxID, "--timeout", "3601", "--", "ls"},
		{"sandboxes", "exec", sbxID, "--env", "BAD", "--", "ls"},
		{"sandboxes", "exec", sbxID, "--env", "1A=2", "--", "ls"},
		{"sandboxes", "exec", "bad", "--", "ls"},
		{"sandboxes", "exec", sbxID, "--", ""},
		{"sandboxes", "exec", sbxID, "--cwd", strings.Repeat("é", 1025), "--", "ls"},
		{"sandboxes", "exec", sbxID, "--cwd", "/" + strings.Repeat("d", 1024), "--", "ls"},
		{"sandboxes", "exec", sbxID, "--env", "A=" + strings.Repeat("v", 8193), "--", "ls"},
	} {
		if code, _, _ := h.run(args...); code != exitUsage {
			t.Errorf("%v exit %d, want usage", args, code)
		}
	}
}

// fakeTerminal is a terminal that records what rzv did to it.
type fakeTerminal struct {
	mu          sync.Mutex
	interactive bool
	raw         bool
	restored    bool
	resize      func()
	hangup      func()
	restoreRuns int
	cols, rows  int
}

func (f *fakeTerminal) Interactive() bool { return f.interactive }
func (f *fakeTerminal) MakeRaw() (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.raw = true
	return func() { f.mu.Lock(); f.restored = true; f.restoreRuns++; f.mu.Unlock() }, nil
}
func (f *fakeTerminal) OnHangup(fn func()) func() {
	f.mu.Lock()
	f.hangup = fn
	f.mu.Unlock()
	return func() {}
}
func (f *fakeTerminal) Size() (int, int, error) { return f.cols, f.rows, nil }
func (f *fakeTerminal) OnResize(fn func()) func() {
	f.mu.Lock()
	f.resize = fn
	f.mu.Unlock()
	return func() {}
}

type pipeReader struct{ data chan []byte }

func (p pipeReader) Read(buffer []byte) (int, error) {
	chunk, ok := <-p.data
	if !ok {
		return 0, io.EOF
	}
	return copy(buffer, chunk), nil
}

func (h *harness) runWithTerminal(terminal Terminal, stdin io.Reader, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, Deps{
		Stdout: &stdout, Stderr: &stderr, Store: h.store, Stdin: stdin, Terminal: terminal,
		Getenv:      func(k string) string { return h.env[k] },
		OpenBrowser: h.openBrowser, CallbackPort: -1, LoginTimeout: 5 * time.Second,
	})
	return code, stdout.String(), stderr.String()
}

// terminalRoute upgrades the request and hands the connection to serve.
func terminalRoute(t *testing.T, h *harness, serve func(conn net.Conn, reader *bufio.Reader, header http.Header)) {
	t.Helper()
	h.routes["GET /v1/customer/tenants/t-1/sandboxes/"+sbxID+"/terminal"] = func(w http.ResponseWriter, r *http.Request) {
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11")) //nolint:gosec
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n")
		_ = rw.Flush()
		serve(conn, rw.Reader, r.Header)
	}
}

func serverFrame(opcode byte, payload []byte) []byte {
	return append([]byte{0x80 | opcode, byte(len(payload))}, payload...)
}

func readClientFrame(reader *bufio.Reader) (byte, []byte, error) {
	var head [2]byte
	if _, err := io.ReadFull(reader, head[:]); err != nil {
		return 0, nil, err
	}
	n := int(head[1] & 0x7F)
	if n == 126 {
		var extra [2]byte
		if _, err := io.ReadFull(reader, extra[:]); err != nil {
			return 0, nil, err
		}
		n = int(binary.BigEndian.Uint16(extra[:]))
	}
	mask := make([]byte, 4)
	if _, err := io.ReadFull(reader, mask); err != nil {
		return 0, nil, err
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return 0, nil, err
	}
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
	return head[0] & 0x0F, payload, nil
}

func TestSandboxesTerminalPassesKeysBothWaysAndRestoresTheTerminal(t *testing.T) {
	for _, verb := range []string{"terminal"} {
		t.Run(verb, func(t *testing.T) {
			h := selectedHarness(t)
			h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxJSON)
			var auth string
			type got struct {
				opcode  byte
				payload string
			}
			frames := make(chan got, 8)
			terminalRoute(t, h, func(conn net.Conn, reader *bufio.Reader, header http.Header) {
				auth = header.Get("Authorization")
				// The first frame is the window size, then keystrokes arrive as bytes.
				for i := 0; i < 2; i++ {
					opcode, payload, err := readClientFrame(reader)
					if err != nil {
						return
					}
					frames <- got{opcode, string(payload)}
				}
				_, _ = conn.Write(serverFrame(2, []byte("$ ls\r\nfile\r\n")))
				_, _ = conn.Write(serverFrame(8, append([]byte{0x03, 0xE8}, "sandbox_stopped"...)))
			})
			terminal := &fakeTerminal{interactive: true, cols: 132, rows: 43}
			stdin := pipeReader{data: make(chan []byte, 1)}
			stdin.data <- []byte("ls\r")
			code, out, errOut := h.runWithTerminal(terminal, stdin, "sandboxes", verb, sbxID)
			if code != exitFailure || !strings.Contains(errOut, "The sandbox was stopped.") || out != "$ ls\r\nfile\r\n" {
				t.Fatalf("ssh: %d stdout %q stderr %q", code, out, errOut)
			}
			if first, second := <-frames, <-frames; first.opcode != 1 || !strings.Contains(first.payload, `"cols":132`) || !strings.Contains(first.payload, `"rows":43`) || second.opcode != 2 || second.payload != "ls\r" {
				t.Fatalf("frames = %+v %+v", first, second)
			}
			if !strings.HasPrefix(auth, "Bearer ") || len(auth) < 12 {
				t.Fatalf("the terminal was opened with Authorization %q", auth)
			}
			terminal.mu.Lock()
			defer terminal.mu.Unlock()
			if !terminal.raw || !terminal.restored {
				t.Fatalf("raw mode %v, restored %v", terminal.raw, terminal.restored)
			}
		})
	}
}

func TestSandboxesTerminalRefusesWhatCannotWork(t *testing.T) {
	h := selectedHarness(t)
	interactive := &fakeTerminal{interactive: true, cols: 80, rows: 24}
	stdin := pipeReader{data: make(chan []byte)}
	if code, _, errOut := h.runWithTerminal(&fakeTerminal{}, stdin, "sandboxes", "terminal", sbxID); code != exitUsage || !strings.Contains(errOut, "interactive terminal") {
		t.Fatalf("no terminal: %d %q", code, errOut)
	}
	if code, _, _ := h.runWithTerminal(interactive, stdin, "sandboxes", "terminal", sbxID, "--json"); code != exitUsage {
		t.Fatalf("--json: %d", code)
	}
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxWith("stopped"))
	if code, _, errOut := h.runWithTerminal(interactive, stdin, "sandboxes", "terminal", sbxID); code != exitFailure || !strings.Contains(errOut, "is stopped") || !strings.Contains(errOut, "rzv sandboxes start") {
		t.Fatalf("stopped sandbox: %d %q", code, errOut)
	}
	if interactive.raw {
		t.Fatal("the terminal was switched to raw mode for a sandbox that is not running")
	}
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxJSON)
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID+"/terminal", 403, `{"error":{"code":"api_key_scope_required"}}`)
	if code, _, errOut := h.runWithTerminal(interactive, stdin, "sandboxes", "terminal", sbxID); code != exitAccessDenied || !strings.Contains(errOut, "scope does not allow") {
		t.Fatalf("refused terminal: %d %q", code, errOut)
	}
	if interactive.raw {
		t.Fatal("the terminal was switched to raw mode for a refused connection")
	}
	// A shell that exits closes normally with no reason: not an error.
	terminalRoute(t, h, func(conn net.Conn, reader *bufio.Reader, _ http.Header) {
		_, _, _ = readClientFrame(reader)
		_, _ = conn.Write(serverFrame(8, []byte{0x03, 0xE8}))
	})
	delete(h.routes, "GET /v1/customer/tenants/t-1/sandboxes/"+sbxID+"/terminal-dummy")
	if code, _, errOut := h.runWithTerminal(interactive, stdin, "sandboxes", "terminal", sbxID); code != 0 {
		t.Fatalf("a shell that exited: %d %q", code, errOut)
	}
	// A dropped connection says to run the command again.
	terminalRoute(t, h, func(conn net.Conn, reader *bufio.Reader, _ http.Header) {
		_, _, _ = readClientFrame(reader)
	})
	if code, _, errOut := h.runWithTerminal(interactive, stdin, "sandboxes", "terminal", sbxID); code != exitUnavailable || !strings.Contains(errOut, "again") {
		t.Fatalf("a dropped connection: %d %q", code, errOut)
	}
}

// SIGTERM or SIGHUP while the terminal is raw puts it back and ends the session.
func TestSandboxesTerminalRestoresTheTerminalOnHangup(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxJSON)
	open := make(chan struct{})
	terminalRoute(t, h, func(conn net.Conn, reader *bufio.Reader, _ http.Header) {
		_, _, _ = readClientFrame(reader) // the window size
		close(open)
		_, _ = io.Copy(io.Discard, conn) // then silence until the client closes
	})
	terminal := &fakeTerminal{interactive: true, cols: 80, rows: 24}
	go func() {
		<-open
		for {
			terminal.mu.Lock()
			hangup := terminal.hangup
			terminal.mu.Unlock()
			if hangup != nil {
				hangup()
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	done := make(chan struct{})
	var code int
	var errOut string
	go func() {
		code, _, errOut = h.runWithTerminal(terminal, pipeReader{data: make(chan []byte)}, "sandboxes", "terminal", sbxID)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ssh did not end when the process was asked to")
	}
	terminal.mu.Lock()
	defer terminal.mu.Unlock()
	if !terminal.raw || !terminal.restored || terminal.restoreRuns != 1 {
		t.Fatalf("raw %v restored %v restore ran %d times, want once", terminal.raw, terminal.restored, terminal.restoreRuns)
	}
	if code != exitFailure || !strings.Contains(errOut, "terminal was closed") {
		t.Fatalf("hangup: %d %q", code, errOut)
	}
}

func TestSandboxesExecExplainsOutputCapAndSandboxEnds(t *testing.T) {
	h := selectedHarness(t)
	execServer(h, chunk("stdout", "lots")+`{"type":"exit","exit_code":137,"timed_out":false,"truncated":true}`+"\n", 200, nil)
	if code, out, errOut := h.run("sandboxes", "exec", sbxID, "--", "yes"); code != 137 || out != "lots" || !strings.Contains(errOut, "8 MiB") {
		t.Fatalf("truncated: %d %q %q", code, out, errOut)
	}
	execServer(h, `{"type":"exit","exit_code":143,"timed_out":false,"truncated":false,"reason":"sandbox_stopped"}`+"\n", 200, nil)
	if code, _, errOut := h.run("sandboxes", "exec", sbxID, "--", "sleep", "60"); code != 143 || !strings.Contains(errOut, "sandbox was stopped") {
		t.Fatalf("stopped mid-command: %d %q", code, errOut)
	}
	execServer(h, `{"type":"exit","exit_code":0,"timed_out":false,"truncated":false,"reason":"sandbox_expired"}`+"\n", 200, nil)
	if code, _, errOut := h.run("sandboxes", "exec", sbxID, "--", "sleep", "60"); code != exitFailure || !strings.Contains(errOut, "reached its lifetime") {
		t.Fatalf("expired mid-command: %d %q", code, errOut)
	}
	code, out, _ := h.run("sandboxes", "exec", sbxID, "--json", "--", "sleep", "60")
	if code == 0 || !strings.Contains(out, `"reason":"sandbox_expired"`) || !strings.Contains(out, `"truncated":false`) {
		t.Fatalf("exec --json with a reason: %d %q", code, out)
	}
	execServer(h, `{"error":{"code":"session_limit_reached"}}`, 409, nil)
	if code, _, errOut := h.run("sandboxes", "exec", sbxID, "--", "ls"); code != exitFailure || !strings.Contains(errOut, "4 commands and 2 terminals") {
		t.Fatalf("session limit: %d %q", code, errOut)
	}
}

func TestSandboxesTerminalExplainsWhyCoreClosedTheTerminal(t *testing.T) {
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxJSON)
	cases := []struct {
		code   uint16
		reason string
		exit   int
		text   string
	}{
		{1000, "shell_exited", 0, ""},
		{1000, "idle_timeout", exitFailure, "30 minutes"},
		{1008, "input_rate", exitFailure, "too fast"},
		{1011, "output_overflow", exitFailure, "not read fast enough"},
		{1011, "", exitUnavailable, "code 1011"},
	}
	for _, c := range cases {
		terminalRoute(t, h, func(conn net.Conn, reader *bufio.Reader, _ http.Header) {
			_, _, _ = readClientFrame(reader)
			payload := binary.BigEndian.AppendUint16(nil, c.code)
			_, _ = conn.Write(serverFrame(8, append(payload, c.reason...)))
		})
		terminal := &fakeTerminal{interactive: true, cols: 80, rows: 24}
		code, _, errOut := h.runWithTerminal(terminal, pipeReader{data: make(chan []byte)}, "sandboxes", "terminal", sbxID)
		if code != c.exit || !strings.Contains(errOut, c.text) {
			t.Errorf("close %d %s: exit %d %q, want %d containing %q", c.code, c.reason, code, errOut, c.exit, c.text)
		}
	}
}

func TestSandboxesTerminalTicket(t *testing.T) {
	h := selectedHarness(t)
	route := "POST /v1/customer/tenants/t-1/sandboxes/" + sbxID + "/terminal-ticket"
	h.json(route, 201, `{"url":"wss://api.example.test/v1/customer/tenants/t-1/sandboxes/`+sbxID+`/terminal","ticket":"tkt-test-only","expires_at":"2026-10-30T09:00:30Z"}`)
	// stdout holds only the URL; the single-use note goes to stderr without the ticket.
	code, out, errOut := h.run("sandboxes", "terminal-ticket", sbxID)
	if code != 0 || out != "wss://api.example.test/v1/customer/tenants/t-1/sandboxes/"+sbxID+"/terminal?ticket=tkt-test-only\n" {
		t.Fatalf("terminal-ticket: %d %q", code, out)
	}
	if strings.Count(errOut, "\n") != 1 || !strings.Contains(errOut, "works once") || !strings.Contains(errOut, "about 30 seconds") || strings.Contains(errOut, "tkt-test-only") {
		t.Fatalf("terminal-ticket note: %q", errOut)
	}
	code, out, errOut = h.run("sandboxes", "terminal-ticket", sbxID, "--json")
	var got map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil || got["ticket"] != "tkt-test-only" || got["url"] != "wss://api.example.test/v1/customer/tenants/t-1/sandboxes/"+sbxID+"/terminal" || got["expires_at"] != "2026-10-30T09:00:30Z" || !strings.HasSuffix(got["connect_url"].(string), "?ticket=tkt-test-only") || got["sandbox_id"] != sbxID {
		t.Fatalf("terminal-ticket --json: %d %q", code, out)
	}
	if !strings.Contains(errOut, "works once") || strings.Contains(errOut, "tkt-test-only") {
		t.Fatalf("terminal-ticket --json note: %q", errOut)
	}
	// Refusals never echo a ticket.
	h.json(route, 201, `{"url":"ftp://x","ticket":"tkt-test-only","expires_at":"2026-10-30T09:00:30Z"}`)
	if code, out, errOut := h.run("sandboxes", "terminal-ticket", sbxID); code == 0 || strings.Contains(out+errOut, "tkt-test-only") {
		t.Fatalf("a malformed ticket response: %d %q %q", code, out, errOut)
	}
	// The ticket travels over wss, or over ws only to this machine.
	for _, target := range []string{"https://api.example.test/terminal", "ws://api.example.test/v1/terminal"} {
		h.json(route, 201, `{"url":"`+target+`","ticket":"tkt-test-only","expires_at":"2026-10-30T09:00:30Z"}`)
		if code, out, errOut := h.run("sandboxes", "terminal-ticket", sbxID); code == 0 || strings.Contains(out+errOut, "tkt-test-only") {
			t.Fatalf("a ticket for %s was accepted: %d %q %q", target, code, out, errOut)
		}
	}
	for _, target := range []string{"ws://127.0.0.1:8080/terminal", "ws://localhost/terminal", "ws://[::1]:8080/terminal"} {
		h.json(route, 201, `{"url":"`+target+`","ticket":"tkt-test-only","expires_at":"2026-10-30T09:00:30Z"}`)
		if code, out, _ := h.run("sandboxes", "terminal-ticket", sbxID); code != 0 || out != target+"?ticket=tkt-test-only\n" {
			t.Fatalf("a loopback ticket for %s: %d %q", target, code, out)
		}
	}
	h.json(route, 201, `{"url":"wss://api.example.test/terminal","ticket":"tkt-test-only","expires_at":"in 30 seconds"}`)
	if code, out, errOut := h.run("sandboxes", "terminal-ticket", sbxID); code == 0 || strings.Contains(out+errOut, "tkt-test-only") {
		t.Fatalf("an expires_at that is not RFC 3339 was accepted: %d %q %q", code, out, errOut)
	}
	h.json(route, 201, `{"url":"wss://api.example.test/terminal","expires_at":"2026-10-30T09:00:30Z"}`)
	if code, _, _ := h.run("sandboxes", "terminal-ticket", sbxID); code == 0 {
		t.Fatal("a response without a ticket was accepted")
	}
	h.json(route, 429, `{"error":{"code":"ticket_limit_reached"}}`)
	if code, _, errOut := h.run("sandboxes", "terminal-ticket", sbxID); code != exitFailure || !strings.Contains(errOut, "10 terminal tickets") {
		t.Fatalf("ticket limit: %d %q", code, errOut)
	}
	h.json(route, 503, `{"error":{"code":"sandbox_exec_unavailable"}}`)
	if code, _, errOut := h.run("sandboxes", "terminal-ticket", sbxID); code != exitUnavailable || !strings.Contains(errOut, "Terminal tickets are not available") || strings.Contains(errOut, "Running commands") {
		t.Fatalf("no public origin: %d %q", code, errOut)
	}
	for _, args := range [][]string{{"sandboxes", "terminal-ticket"}, {"sandboxes", "terminal-ticket", "dev"}, {"sandboxes", "terminal-ticket", sbxID, "extra"}} {
		if code, _, _ := h.run(args...); code != exitUsage {
			t.Errorf("%v exit %d, want usage", args, code)
		}
	}
}

// The limits count characters, as JSON Schema maxLength does, and a command
// of spaces is still a command.
func TestSandboxesExecLimitsCountCharacters(t *testing.T) {
	h := selectedHarness(t)
	var seen map[string]any
	execServer(h, `{"type":"exit","exit_code":0,"timed_out":false}`+"\n", 200, &seen)
	cwd, value := strings.Repeat("é", 1024), strings.Repeat("é", 8192)
	if code, _, errOut := h.run("sandboxes", "exec", sbxID, "--cwd", cwd, "--env", "A="+value, "--", "ls"); code != 0 || seen["cwd"] != cwd {
		t.Fatalf("multibyte values at the limit: %d %q", code, errOut)
	}
	seen = nil
	if code, _, errOut := h.run("sandboxes", "exec", sbxID, "--", " "); code != 0 || seen["cmd"] != " " {
		t.Fatalf("a whitespace command: %d %q %v", code, errOut, seen)
	}
}

func TestSandboxesExecQuotesEachArgumentAfterTheFirst(t *testing.T) {
	h := selectedHarness(t)
	var seen map[string]any
	execServer(h, `{"type":"exit","exit_code":7,"timed_out":false}`+"\n", 200, &seen)
	if code, _, _ := h.run("sandboxes", "exec", sbxID, "--", "sh", "-c", "exit 7"); code != 7 || seen["cmd"] != `'sh' '-c' 'exit 7'` {
		t.Fatalf("sh -c \"exit 7\": %d %v", code, seen)
	}
	// One argument is a shell script and goes unchanged.
	for _, script := range []string{"echo a; echo b", "ls | wc -l", `echo "$HOME"`} {
		execServer(h, `{"type":"exit","exit_code":0,"timed_out":false}`+"\n", 200, &seen)
		if code, _, _ := h.run("sandboxes", "exec", sbxID, "--", script); code != 0 || seen["cmd"] != script {
			t.Fatalf("script %q: %d %v", script, code, seen)
		}
	}
	// The limit counts the quoted command: 'a' plus a space is 4 characters a word.
	execServer(h, `{"type":"exit","exit_code":0,"timed_out":false}`+"\n", 200, nil)
	if code, _, _ := h.run(append([]string{"sandboxes", "exec", sbxID, "--"}, strings.Split(strings.Repeat("a", maxCmd/4+1), "")...)...); code != exitUsage {
		t.Fatalf("a quoted command over the limit: exit %d, want usage", code)
	}
}

func TestExecCommandGivesTheShellTheExactArguments(t *testing.T) {
	args := []string{"two words", "it's", `"double"`, "$HOME", "`id`", "a;b", "&&", "line\nbreak", "", "naïve ☃ 日本", `back\slash`, "'", "*"}
	if got := execCommand([]string{"it's", ""}); got != `'it'\''s' ''` {
		t.Fatalf("execCommand = %q", got)
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to run the command with")
	}
	// printf ends each argument the shell passes it with a NUL byte, so
	// newlines and empty arguments come back intact.
	cmd := execCommand(append([]string{"printf", `%s\0`}, args...))
	out, err := exec.Command(sh, "-c", cmd).Output()
	if err != nil {
		t.Fatalf("sh -c %q: %v", cmd, err)
	}
	got := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	if len(got) != len(args) {
		t.Fatalf("sh saw %q, want %q", got, args)
	}
	for i := range args {
		if got[i] != args[i] {
			t.Errorf("argument %d: sh saw %q, want %q", i, got[i], args[i])
		}
	}
}

func TestSandboxesCreateLifetime(t *testing.T) {
	h := selectedHarness(t)
	var bodies []string
	h.routes["POST /v1/customer/tenants/t-1/sandboxes"] = func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(data))
		w.WriteHeader(201)
		_, _ = w.Write([]byte(sandboxLifetimeJSON("starting", 12, "null", 43200)))
	}
	for _, c := range []struct {
		flag  string
		hours float64
	}{{"8h", 8}, {"12h", 12}, {"24h", 24}} {
		before := len(bodies)
		code, out, errOut := h.run("sandboxes", "create", "--name", "dev", "--lifetime", c.flag)
		if code != 0 || len(bodies) != before+1 {
			t.Fatalf("--lifetime %s: %d %q %q", c.flag, code, out, errOut)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(bodies[before]), &body); err != nil || body["lifetime_hours"] != c.hours {
			t.Fatalf("--lifetime %s sent %s", c.flag, bodies[before])
		}
	}
	before := len(bodies)
	if code, _, errOut := h.run("sandboxes", "create", "--name", "dev"); code != 0 || len(bodies) != before+1 || strings.Contains(bodies[before], "lifetime") {
		t.Fatalf("create without --lifetime: %d %q sent %v", code, errOut, bodies[before:])
	}
	code, out, _ := h.run("sandboxes", "create", "--name", "dev", "--lifetime", "12h", "--json")
	if code != 0 || !strings.Contains(out, `"lifetime_hours":12`) {
		t.Fatalf("create --json: %d %q", code, out)
	}
	before = len(bodies)
	for _, value := range []string{"6h", "48h", "25h", "abc", "8", "12", "24", "0h", "8H", "8h30m", "-8h", "1d"} {
		code, _, errOut := h.run("sandboxes", "create", "--name", "dev", "--lifetime", value)
		if code != exitUsage || !strings.Contains(errOut, "--lifetime must be 8h, 12h or 24h") {
			t.Errorf("--lifetime %q: exit %d %q, want usage", value, code, errOut)
		}
	}
	if code, _, _ := h.run("sandboxes", "create", "--name", "dev", "--lifetime"); code != exitUsage {
		t.Errorf("--lifetime without a value: exit %d, want usage", code)
	}
	if len(bodies) != before {
		t.Fatalf("a refused lifetime reached the API: %v", bodies[before:])
	}
	if code, _, _ := h.run("sandboxes", "start", sbxID, "--lifetime", "8h"); code != exitUsage {
		t.Errorf("start --lifetime exit %d, want usage", code)
	}
}

func TestSandboxesCreateExplainsInvalidLifetimeFromTheServer(t *testing.T) {
	h := selectedHarness(t)
	h.json("POST /v1/customer/tenants/t-1/sandboxes", 400, `{"error":{"code":"invalid_lifetime"}}`)
	code, _, errOut := h.run("sandboxes", "create", "--name", "dev", "--lifetime", "12h")
	if code != exitUsage || !strings.Contains(errOut, "lifetime must be 8h, 12h or 24h") || strings.Contains(errOut, "400") {
		t.Fatalf("invalid_lifetime: %d %q", code, errOut)
	}
}

func TestSandboxesShowLifetimeExpiryAndTimeLeft(t *testing.T) {
	fixedZone(t, "EST", -5*3600)
	h := selectedHarness(t)
	running := sandboxLifetimeJSON("running", 12, `"2026-11-02T23:04:00Z"`, 7*3600+42*60+30)
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, running)
	code, out, _ := h.run("sandboxes", "get", sbxID)
	for _, want := range []string{"dev [" + sbxID + "]: running", "Lifetime: 12h from first run", "Time left: 7h 42m", "Expires: 2026-11-02 18:04 EST (23:04 UTC)"} {
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("get running: %d missing %q in %q", code, want, out)
		}
	}
	// Local and UTC dates differ: the UTC part carries its date.
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxLifetimeJSON("running", 24, `"2026-11-03T02:30:00Z"`, 3600))
	if code, out, _ := h.run("sandboxes", "get", sbxID); code != 0 || !strings.Contains(out, "Expires: 2026-11-02 21:30 EST (2026-11-03 02:30 UTC)") || !strings.Contains(out, "Time left: 1h 00m") {
		t.Fatalf("get across midnight UTC: %d %q", code, out)
	}
	fixedZone(t, "UTC", 0)
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, running)
	if code, out, _ := h.run("sandboxes", "get", sbxID); code != 0 || !strings.Contains(out, "Expires: 2026-11-02 23:04 UTC\n") {
		t.Fatalf("get in UTC: %d %q", code, out)
	}
	fixedZone(t, "EST", -5*3600)

	// A stopped sandbox keeps its expiry.
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxLifetimeJSON("stopped", 8, `"2026-11-02T23:04:00Z"`, 59*60))
	if code, out, _ := h.run("sandboxes", "get", sbxID); code != 0 || !strings.Contains(out, "Time left: 59m") || !strings.Contains(out, "Expires: 2026-11-02 18:04 EST (23:04 UTC)") {
		t.Fatalf("get stopped: %d %q", code, out)
	}

	// Before the first start there is no expiry and the full lifetime is left.
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxLifetimeJSON("stopped", 24, "null", 24*3600))
	if code, out, _ := h.run("sandboxes", "get", sbxID); code != 0 || !strings.Contains(out, "Time left: 24h 00m") || !strings.Contains(out, "Expires: starts when running") {
		t.Fatalf("get before first run: %d %q", code, out)
	}

	// An older server sends no lifetime_hours: it is unknown, never assumed.
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxLifetimeJSON("running", 0, `"2026-11-02T23:04:00Z"`, 600))
	code, out, _ = h.run("sandboxes", "get", sbxID)
	if code != 0 || strings.Contains(out, "Lifetime:") || !strings.Contains(out, "Time left: 10m") {
		t.Fatalf("get without lifetime_hours: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxLifetimeJSON("stopped", 0, "null", 0))
	if code, out, _ := h.run("sandboxes", "get", sbxID); code != 0 || !strings.Contains(out, "Time left: -") {
		t.Fatalf("get with no lifetime and no remaining time: %d %q", code, out)
	}

	// The list carries the same facts, one row each.
	h.json("GET /v1/customer/tenants/t-1/sandboxes", 200, `{"sandboxes":[`+running+`,`+sandboxLifetimeJSON("stopped", 24, "null", 24*3600)+`,`+sandboxLifetimeJSON("running", 0, `"2026-11-02T23:04:00Z"`, 600)+`],"max_running":2}`)
	code, out, _ = h.run("sandboxes", "list")
	rows := strings.Split(strings.TrimSpace(out), "\n")
	if code != 0 || len(rows) < 4 {
		t.Fatalf("list: %d %q", code, out)
	}
	for _, want := range []string{"LIFETIME", "TIME LEFT", "EXPIRES"} {
		if !strings.Contains(rows[0], want) {
			t.Errorf("list header %q lacks %s", rows[0], want)
		}
	}
	for i, want := range [][]string{
		{"running", "12h", "7h 42m", "2026-11-02 18:04 EST (23:04 UTC)"},
		{"stopped", "24h", "24h 00m", "starts when running"},
		{"running", "-", "10m", "2026-11-02 18:04 EST (23:04 UTC)"},
	} {
		if got := strings.Join(strings.Fields(rows[1+i]), " "); !strings.Contains(got, "2vcpu-8gb "+strings.Join(want, " ")+" 20.5 min") {
			t.Errorf("list row %d = %q, want %v after the size", i, rows[1+i], want)
		}
	}

	// --json passes the API fields through.
	code, out, _ = h.run("sandboxes", "list", "--json")
	var listed struct {
		Sandboxes []map[string]any `json:"sandboxes"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &listed) != nil || len(listed.Sandboxes) != 3 || listed.Sandboxes[0]["lifetime_hours"] != float64(12) || listed.Sandboxes[0]["expires_at"] != "2026-11-02T23:04:00Z" || listed.Sandboxes[1]["expires_at"] != nil {
		t.Fatalf("list --json: %d %q", code, out)
	}
	if _, present := listed.Sandboxes[2]["lifetime_hours"]; present {
		t.Errorf("list --json invented lifetime_hours for an older server: %v", listed.Sandboxes[2])
	}
}

func TestSandboxesShowCleanupPendingAsNotDeleted(t *testing.T) {
	fixedZone(t, "EST", -5*3600)
	h := selectedHarness(t)
	pending := sandboxLifetimeJSON("cleanup_pending", 8, `"2026-11-02T23:04:00Z"`, 0)
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, pending)
	code, out, _ := h.run("sandboxes", "get", sbxID)
	if code != 0 || !strings.Contains(out, "dev ["+sbxID+"]: cleanup pending") || !strings.Contains(out, "Time left: -") || !strings.Contains(out, "files are not deleted yet") || strings.Contains(out, "cleanup_pending") {
		t.Fatalf("get cleanup_pending: %d %q", code, out)
	}
	h.json("GET /v1/customer/tenants/t-1/sandboxes", 200, `{"sandboxes":[`+pending+`],"max_running":2}`)
	code, out, _ = h.run("sandboxes", "list")
	if code != 0 || !strings.Contains(out, "cleanup pending") || strings.Contains(out, "cleanup_pending") || strings.Contains(out, "deleted") {
		t.Fatalf("list cleanup_pending: %d %q", code, out)
	}
	// The raw status stays in JSON.
	if code, out, _ := h.run("sandboxes", "list", "--json"); code != 0 || !strings.Contains(out, `"status":"cleanup_pending"`) {
		t.Fatalf("list --json cleanup_pending: %d %q", code, out)
	}
	// A delete that leaves files behind reports the same status.
	h.json("DELETE /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, pending)
	if code, out, _ := h.run("sandboxes", "delete", sbxID); code != 0 || !strings.Contains(out, "Deleting sandbox dev ["+sbxID+"]: cleanup pending") {
		t.Fatalf("delete cleanup_pending: %d %q", code, out)
	}
}
