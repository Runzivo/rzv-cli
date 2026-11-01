//go:build !windows

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

// SystemTerminal is the terminal of this process. On macOS and Linux it uses
// the stty tool every Unix system has, so PTY handling needs no additional dependency.
func SystemTerminal() Terminal { return unixTerminal{} }

type unixTerminal struct{}

func characterDevice(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (unixTerminal) Interactive() bool {
	return characterDevice(os.Stdin) && characterDevice(os.Stdout)
}

func stty(args ...string) (string, error) {
	command := exec.Command("stty", args...)
	command.Stdin = os.Stdin
	out, err := command.Output()
	return strings.TrimSpace(string(out)), err
}

func (unixTerminal) MakeRaw() (func(), error) {
	saved, err := stty("-g")
	if err != nil || saved == "" {
		return nil, fmt.Errorf("the terminal cannot be switched to raw mode")
	}
	if _, err := stty("raw", "-echo"); err != nil {
		return nil, fmt.Errorf("the terminal cannot be switched to raw mode")
	}
	return func() { _, _ = stty(saved) }, nil
}

func (unixTerminal) Size() (int, int, error) {
	out, err := stty("size")
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected terminal size %q", out)
	}
	rows, rowsErr := strconv.Atoi(fields[0])
	cols, colsErr := strconv.Atoi(fields[1])
	if rowsErr != nil || colsErr != nil || rows < 1 || cols < 1 {
		return 0, 0, fmt.Errorf("unexpected terminal size %q", out)
	}
	return cols, rows, nil
}

func (unixTerminal) OnResize(fn func()) func() {
	changes := make(chan os.Signal, 1)
	signal.Notify(changes, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-changes:
				fn()
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(changes)
		close(done)
	}
}

func (unixTerminal) OnHangup(fn func()) func() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGHUP)
	done := make(chan struct{})
	go func() {
		select {
		case <-signals:
			fn()
		case <-done:
		}
	}()
	return func() {
		signal.Stop(signals)
		close(done)
	}
}
