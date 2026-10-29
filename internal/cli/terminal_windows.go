//go:build windows

package cli

import "fmt"

// SystemTerminal on Windows cannot switch the console to raw mode without
// third-party code, so rzv sandboxes ssh is not offered there yet; exec works.
func SystemTerminal() Terminal { return windowsTerminal{} }

type windowsTerminal struct{}

func (windowsTerminal) Interactive() bool { return false }
func (windowsTerminal) MakeRaw() (func(), error) {
	return nil, fmt.Errorf("an interactive terminal is not available on Windows")
}
func (windowsTerminal) Size() (int, int, error) {
	return 0, 0, fmt.Errorf("an interactive terminal is not available on Windows")
}
func (windowsTerminal) OnResize(func()) func() { return func() {} }
func (windowsTerminal) OnHangup(func()) func() { return func() {} }
