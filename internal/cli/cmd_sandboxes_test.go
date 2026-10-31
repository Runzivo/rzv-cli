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
	h := selectedHarness(t)
	h.json("GET /v1/customer/tenants/t-1/sandboxes", 200, `{"sandboxes":[`+sandboxJSON+`,`+strings.Replace(sandboxWith("stopped"), `"id":"sbx_0123456789abcdef01234567","name":"dev"`, `"id":"sbx_aaaaaaaaaaaaaaaaaaaaaaaa","name":"build"`, 1)+`],"max_running":2,"sizes":["2vcpu-8gb"]}`)
	code, out, _ := h.run("sandboxes", "list")
	if code != 0 || !strings.Contains(out, "ID") || !strings.Contains(out, sbxID) || !strings.Contains(out, "7h30m") || !strings.Contains(out, "20.5 min") || !strings.Contains(out, "This workspace can run 2 at once.") || !strings.Contains(out, "Sizes on offer here: 2vcpu-8gb.") || !strings.Contains(out, "build") {
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
	if code, out, _ := h.run("sandboxes", "get", sbxID); code != 0 || !strings.Contains(out, "dev ["+sbxID+"]: running") || !strings.Contains(out, "Time left: 7h30m") {
		t.Fatalf("get: %d %q", code, out)
	}
	for _, args := range [][]string{{"sandboxes", "get"}, {"sandboxes", "get", "dev"}, {"sandboxes", "get", "sbx_short"}, {"sandboxes", "bogus"}, {"sandboxes"}} {
		if code, _, _ := h.run(args...); code != exitUsage {
			t.Errorf("%v exit %d, want usage", args, code)
		}
	}
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
	for _, args := range [][]string{
		{"sandboxes", "create"},
		{"sandboxes", "create", "--name", "x", "--size", "1vcpu"},
		{"sandboxes", "create", "--name", "x", "--disk-gib", "4"},
		{"sandboxes", "create", "--name", "x", "--disk-gib", "65"},
		{"sandboxes", "create", "--name", "x", "--idempotency-key", "short"},
		{"sandboxes", "create", "--name", strings.Repeat("n", 81)},
	} {
		if code, _, _ := h.run(args...); code != exitUsage {
			t.Errorf("%v exit %d, want usage", args, code)
		}
	}
	if len(bodies) != 2 {
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
		{"start", 409, "sandbox_expired", exitFailure, "eight hour lifetime"},
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

func TestSandboxesSSHPassesKeysBothWaysAndRestoresTheTerminal(t *testing.T) {
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
	code, out, errOut := h.runWithTerminal(terminal, stdin, "sandboxes", "ssh", sbxID)
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
}

func TestSandboxesSSHRefusesWhatCannotWork(t *testing.T) {
	h := selectedHarness(t)
	interactive := &fakeTerminal{interactive: true, cols: 80, rows: 24}
	stdin := pipeReader{data: make(chan []byte)}
	if code, _, errOut := h.runWithTerminal(&fakeTerminal{}, stdin, "sandboxes", "ssh", sbxID); code != exitUsage || !strings.Contains(errOut, "interactive terminal") {
		t.Fatalf("no terminal: %d %q", code, errOut)
	}
	if code, _, _ := h.runWithTerminal(interactive, stdin, "sandboxes", "ssh", sbxID, "--json"); code != exitUsage {
		t.Fatalf("--json: %d", code)
	}
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxWith("stopped"))
	if code, _, errOut := h.runWithTerminal(interactive, stdin, "sandboxes", "ssh", sbxID); code != exitFailure || !strings.Contains(errOut, "is stopped") || !strings.Contains(errOut, "rzv sandboxes start") {
		t.Fatalf("stopped sandbox: %d %q", code, errOut)
	}
	if interactive.raw {
		t.Fatal("the terminal was switched to raw mode for a sandbox that is not running")
	}
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID, 200, sandboxJSON)
	h.json("GET /v1/customer/tenants/t-1/sandboxes/"+sbxID+"/terminal", 403, `{"error":{"code":"api_key_scope_required"}}`)
	if code, _, errOut := h.runWithTerminal(interactive, stdin, "sandboxes", "ssh", sbxID); code != exitAccessDenied || !strings.Contains(errOut, "scope does not allow") {
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
	if code, _, errOut := h.runWithTerminal(interactive, stdin, "sandboxes", "ssh", sbxID); code != 0 {
		t.Fatalf("a shell that exited: %d %q", code, errOut)
	}
	// A dropped connection says to run the command again.
	terminalRoute(t, h, func(conn net.Conn, reader *bufio.Reader, _ http.Header) {
		_, _, _ = readClientFrame(reader)
	})
	if code, _, errOut := h.runWithTerminal(interactive, stdin, "sandboxes", "ssh", sbxID); code != exitUnavailable || !strings.Contains(errOut, "again") {
		t.Fatalf("a dropped connection: %d %q", code, errOut)
	}
}

// SIGTERM or SIGHUP while the terminal is raw puts it back and ends the session.
func TestSandboxesSSHRestoresTheTerminalOnHangup(t *testing.T) {
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
		code, _, errOut = h.runWithTerminal(terminal, pipeReader{data: make(chan []byte)}, "sandboxes", "ssh", sbxID)
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
	if code, _, errOut := h.run("sandboxes", "exec", sbxID, "--", "sleep", "60"); code != exitFailure || !strings.Contains(errOut, "eight hour lifetime") {
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

func TestSandboxesSSHExplainsWhyCoreClosedTheTerminal(t *testing.T) {
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
		code, _, errOut := h.runWithTerminal(terminal, pipeReader{data: make(chan []byte)}, "sandboxes", "ssh", sbxID)
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
