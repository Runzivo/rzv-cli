//go:build windows

package cli

import "errors"

// Required to compile the shared command file. The command rejects Windows
// before reaching this stub until native Windows SSH is supported.
func runOpenSSH(path string, args []string) error {
	return errors.New("sandbox OpenSSH is not supported on Windows")
}
