//go:build windows

package cli

import (
	"errors"
	"os"
	"os/exec"
)

func runOpenSSH(path string, args []string) error {
	cmd := exec.Command(path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return newError("ssh_exit", exit.ExitCode(), "OpenSSH exited with status %d.", exit.ExitCode())
	}
	return err
}
