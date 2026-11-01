package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Runzivo/rzv-cli/internal/api"
	"github.com/Runzivo/rzv-cli/internal/sshstate"
)

func startingError(retryAfter time.Duration) error {
	return &api.Error{Status: http.StatusServiceUnavailable, Code: "sandbox_ssh_starting", RetryAfter: retryAfter}
}

// recordSleeps returns a sleeper that records each wait without sleeping.
func recordSleeps(sleeps *[]time.Duration) func(context.Context, time.Duration) error {
	return func(ctx context.Context, d time.Duration) error {
		*sleeps = append(*sleeps, d)
		return ctx.Err()
	}
}

func fastSSHRetries(t *testing.T) {
	t.Helper()
	saved := sshRetryMin
	sshRetryMin = time.Millisecond
	t.Cleanup(func() { sshRetryMin = saved })
}

func TestWaitForSSHTicketRetriesUntilReady(t *testing.T) {
	fastSSHRetries(t)
	attempts, notices := 0, 0
	var sleeps []time.Duration
	want := &api.SSHTicket{Ticket: "ready"}
	got, err := waitForSSHTicket(context.Background(), time.Now().Add(time.Minute), func(context.Context) (*api.SSHTicket, error) {
		attempts++
		if attempts < 3 {
			return nil, startingError(0)
		}
		return want, nil
	}, func() { notices++ }, recordSleeps(&sleeps))
	if err != nil || got != want || attempts != 3 || notices != 2 || len(sleeps) != 2 {
		t.Fatalf("ticket=%v err=%v attempts=%d notices=%d sleeps=%v", got, err, attempts, notices, sleeps)
	}
}

// Retry-After is honoured, never below the minimum and never past the deadline.
func TestWaitForSSHTicketHonoursRetryAfter(t *testing.T) {
	saved := sshRetryMin
	sshRetryMin = time.Second
	t.Cleanup(func() { sshRetryMin = saved })
	for _, tc := range []struct {
		name       string
		retryAfter time.Duration
		left       time.Duration
		want       time.Duration
	}{
		{"server delay", 3 * time.Second, time.Minute, 3 * time.Second},
		{"no header uses the minimum", 0, time.Minute, time.Second},
		{"short delay raised to the minimum", 100 * time.Millisecond, time.Minute, time.Second},
		{"long delay cut to the time left", time.Minute, 2 * time.Second, 2 * time.Second},
	} {
		var sleeps []time.Duration
		attempts := 0
		_, _ = waitForSSHTicket(context.Background(), time.Now().Add(tc.left), func(context.Context) (*api.SSHTicket, error) {
			attempts++
			if attempts == 1 {
				return nil, startingError(tc.retryAfter)
			}
			return &api.SSHTicket{}, nil
		}, func() {}, recordSleeps(&sleeps))
		if len(sleeps) != 1 || sleeps[0] > tc.want || sleeps[0] < tc.want-200*time.Millisecond {
			t.Fatalf("%s: sleeps = %v, want about %s", tc.name, sleeps, tc.want)
		}
	}
}

func TestWaitForSSHTicketDoesNotRetryOtherErrors(t *testing.T) {
	attempts := 0
	wantErr := &api.Error{Status: http.StatusForbidden, Code: "permission_required"}
	_, err := waitForSSHTicket(context.Background(), time.Now().Add(time.Minute), func(context.Context) (*api.SSHTicket, error) {
		attempts++
		return nil, wantErr
	}, func() { t.Fatal("reported waiting for a permanent error") }, sleepContext)
	if err != wantErr || attempts != 1 {
		t.Fatalf("permanent error retried: err=%v attempts=%d", err, attempts)
	}
}

// A request still in flight at the deadline completes on the parent context;
// a starting answer after the deadline is the timeout, not "API unavailable".
func TestWaitForSSHTicketDeadlineDuringRequestIsATimeout(t *testing.T) {
	deadline := time.Now().Add(20 * time.Millisecond)
	attempts := 0
	_, err := waitForSSHTicket(context.Background(), deadline, func(ctx context.Context) (*api.SSHTicket, error) {
		attempts++
		time.Sleep(time.Until(deadline) + 10*time.Millisecond)
		if ctx.Err() != nil {
			return nil, &api.Error{Code: "customer_api_unavailable"}
		}
		return nil, startingError(0)
	}, func() {}, sleepContext)
	if !errors.Is(err, errSSHStartTimeout) || attempts != 1 {
		t.Fatalf("err=%v attempts=%d, want the start timeout after one request", err, attempts)
	}
}

// Cancelling while waiting between requests stops at once with
// context.Canceled and sends no further request.
func TestWaitForSSHTicketCancelDuringSleep(t *testing.T) {
	saved := sshRetryMin
	sshRetryMin = time.Hour
	t.Cleanup(func() { sshRetryMin = saved })
	ctx, cancel := context.WithCancel(context.Background())
	var mu sync.Mutex
	attempts := 0
	done := make(chan error, 1)
	go func() {
		_, err := waitForSSHTicket(ctx, time.Now().Add(2*time.Hour), func(context.Context) (*api.SSHTicket, error) {
			mu.Lock()
			attempts++
			mu.Unlock()
			return nil, startingError(0)
		}, func() { cancel() }, sleepContext)
		done <- err
	}()
	select {
	case err := <-done:
		mu.Lock()
		defer mu.Unlock()
		if !errors.Is(err, context.Canceled) || attempts != 1 {
			t.Fatalf("err=%v attempts=%d, want context.Canceled after one request", err, attempts)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wait did not stop when cancelled")
	}
}

func TestParseRetryAfterViaClient(t *testing.T) {
	f := newSSHFixture(t, func(net.Conn, *bufio.Reader) {})
	f.starting = 1
	deps := fixtureDeps(f, t.TempDir(), nil, &bytes.Buffer{}, &bytes.Buffer{})
	deps.Now = time.Now
	a := &app{Deps: deps, ctx: context.Background()}
	cmd, _ := parse([]string{"sandboxes", "ssh-config", sbxID})
	state, client, err := a.sshState(cmd, sbxID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SandboxSSHTicket(context.Background(), state.Binding.Workspace, state.Binding.Sandbox, state.PublicKey())
	apiErr, ok := isSSHStarting(err)
	if !ok || apiErr.RetryAfter != time.Second {
		t.Fatalf("err=%v, want sandbox_ssh_starting with Retry-After 1s", err)
	}
}

// The full ticket path waits through startup with one status line on stderr.
func TestPrepareSSHWaitsForStartWithOneNotice(t *testing.T) {
	fastSSHRetries(t)
	f := newSSHFixture(t, func(net.Conn, *bufio.Reader) {})
	f.starting = 2
	var stderr bytes.Buffer
	deps := fixtureDeps(f, t.TempDir(), nil, &bytes.Buffer{}, &stderr)
	deps.Now = time.Now
	a := &app{Deps: deps, ctx: context.Background()}
	cmd, _ := parse([]string{"sandboxes", "ssh-config", sbxID})
	state, client, err := a.sshState(cmd, sbxID)
	if err != nil {
		t.Fatal(err)
	}
	// Retry-After: 1 from the fixture is honoured; keep the test short by
	// allowing for two one-second waits.
	start := time.Now()
	ticket, err := a.prepareSSH(state, client)
	if err != nil || ticket == nil {
		t.Fatal(err)
	}
	if n := strings.Count(stderr.String(), "Waiting for sandbox SSH to start"); n != 1 {
		t.Fatalf("notices = %d: %q", n, stderr.String())
	}
	if f.startingCalls != 2 || f.issued != 1 || time.Since(start) < 2*time.Second {
		t.Fatalf("starting=%d issued=%d elapsed=%s", f.startingCalls, f.issued, time.Since(start))
	}
}

// When the Match exec refresh times out, it records the deadline and prints no
// error; the ProxyCommand that OpenSSH runs next fails at once with one
// message and sends no ticket request.
func TestSSHProxyAfterRefreshTimeoutFailsAtOnce(t *testing.T) {
	savedWait, savedMin := sshStartWait, sshRetryMin
	sshStartWait, sshRetryMin = 50*time.Millisecond, time.Millisecond
	t.Cleanup(func() { sshStartWait, sshRetryMin = savedWait, savedMin })
	f := newSSHFixture(t, func(net.Conn, *bufio.Reader) {})
	f.starting = -1
	root := t.TempDir()
	var refreshErr, proxyErr bytes.Buffer
	if code := Run(context.Background(), []string{"sandboxes", "ssh-config", sbxID, "--refresh"}, fixtureDeps(f, root, nil, &bytes.Buffer{}, &refreshErr)); code != 255 {
		t.Fatalf("refresh exit = %d", code)
	}
	if strings.Contains(refreshErr.String(), "did not start") || !strings.Contains(refreshErr.String(), "Waiting for sandbox SSH to start") {
		t.Fatalf("refresh stderr = %q, want only the waiting notice", refreshErr.String())
	}
	f.mu.Lock()
	requests := f.startingCalls
	f.mu.Unlock()
	if requests < 2 {
		t.Fatalf("refresh requests = %d", requests)
	}
	if code := Run(context.Background(), []string{"sandboxes", "ssh", sbxID, "--stdio"}, fixtureDeps(f, root, nil, &bytes.Buffer{}, &proxyErr)); code != 255 {
		t.Fatalf("proxy exit = %d", code)
	}
	f.mu.Lock()
	after := f.startingCalls
	f.mu.Unlock()
	if after != requests || strings.Count(proxyErr.String(), "did not start within 60 seconds") != 1 || strings.Contains(proxyErr.String(), "Waiting") {
		t.Fatalf("proxy requests %d -> %d, stderr = %q", requests, after, proxyErr.String())
	}
	// The marker is used once: the next connection waits afresh.
	if code := Run(context.Background(), []string{"sandboxes", "ssh", sbxID, "--stdio"}, fixtureDeps(f, root, nil, &bytes.Buffer{}, &bytes.Buffer{})); code != 255 {
		t.Fatalf("second proxy exit = %d", code)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startingCalls <= after {
		t.Fatal("a later connection did not request a ticket")
	}
}

// A stale marker from an earlier connection does not block a new one.
func TestSSHProxyIgnoresAStaleWaitMarker(t *testing.T) {
	fastSSHRetries(t)
	f := newSSHFixture(t, func(net.Conn, *bufio.Reader) {})
	deps := fixtureDeps(f, t.TempDir(), nil, &bytes.Buffer{}, &bytes.Buffer{})
	deps.Now = time.Now
	a := &app{Deps: deps, ctx: context.Background()}
	cmd, _ := parse([]string{"sandboxes", "ssh-config", sbxID})
	state, client, err := a.sshState(cmd, sbxID)
	if err != nil {
		t.Fatal(err)
	}
	stale, _ := json.Marshal(sshWaitMarker{Deadline: time.Now().Add(-time.Hour)})
	if err := sshstate.WritePrivate(sshWaitPath(state), stale); err != nil {
		t.Fatal(err)
	}
	if _, err := a.prepareSSHFor(state, client, sshPrepareProxy); err != nil || f.issued != 1 {
		t.Fatalf("err=%v issued=%d", err, f.issued)
	}
}
