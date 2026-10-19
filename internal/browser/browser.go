// Package browser opens a URL in the person's default browser.
package browser

import (
	"errors"
	"os/exec"
	"runtime"
)

// Open starts the system browser and does not wait for it.
func Open(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return errors.New("unable to open the system browser")
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
