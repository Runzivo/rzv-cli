package cli

import (
	"os"

	"golang.org/x/sys/unix"
)

// isTerminal reports whether file is a terminal, not merely a character
// device such as /dev/null.
func isTerminal(file *os.File) bool {
	_, err := unix.IoctlGetTermios(int(file.Fd()), unix.TIOCGETA)
	return err == nil
}
