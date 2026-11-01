//go:build !windows

package cli

import (
	"os"
	"syscall"
)

// Replacing rzv with OpenSSH preserves its terminal handling, signals and
// exact exit status, including remote command failures.
func runOpenSSH(path string, args []string) error {
	return syscall.Exec(path, append([]string{path}, args...), os.Environ())
}
