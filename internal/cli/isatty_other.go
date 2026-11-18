//go:build !linux && !darwin

package cli

import "os"

// isTerminal is false here: rzv never prompts for an emailed code on this
// system, as before (the Windows terminal is not interactive for rzv).
func isTerminal(*os.File) bool { return false }
