//go:build !windows

package cli

import (
	"os"
	"syscall"
)

var orcaStopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
