//go:build !windows

package cli

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// The real terminal calls its handler once for SIGTERM and for SIGHUP, and
// not after stop.
func TestUnixTerminalHangupHandlerRunsOnSIGTERMAndSIGHUP(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGHUP} {
		called := make(chan struct{}, 2)
		stop := unixTerminal{}.OnHangup(func() { called <- struct{}{} })
		if err := syscall.Kill(os.Getpid(), sig); err != nil {
			t.Fatal(err)
		}
		select {
		case <-called:
		case <-time.After(3 * time.Second):
			t.Fatalf("%v did not reach the handler", sig)
		}
		stop()
	}
	stopped := make(chan struct{}, 1)
	stop := unixTerminal{}.OnHangup(func() { stopped <- struct{}{} })
	// Keep a second subscriber so the signal does not kill the test process.
	keep := unixTerminal{}.OnHangup(func() {})
	stop()
	_ = syscall.Kill(os.Getpid(), syscall.SIGHUP)
	select {
	case <-stopped:
		t.Fatal("a stopped handler still ran")
	case <-time.After(200 * time.Millisecond):
	}
	keep()
}
