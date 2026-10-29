package cli

// Terminal is the person's terminal, injected so tests never touch the real
// one. rzv sandboxes ssh needs raw input, the window size and resize events.
type Terminal interface {
	// Interactive reports whether standard input and output are a terminal.
	Interactive() bool
	// MakeRaw passes every key to the sandbox untouched, including Ctrl-C, and
	// returns the function that puts the terminal back.
	MakeRaw() (restore func(), err error)
	// Size is the window's width and height in characters.
	Size() (cols, rows int, err error)
	// OnResize calls fn whenever the window changes size, until stop is called.
	OnResize(fn func()) (stop func())
}
