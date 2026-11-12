//go:build windows

package cli

import "os"

var orcaStopSignals = []os.Signal{os.Interrupt}
