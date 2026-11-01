// Package cli implements the rzv commands.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Runzivo/rzv-cli/internal/config"
	"github.com/Runzivo/rzv-cli/internal/version"
)

// Deps are the process boundaries, injected so tests never touch the real
// environment, home directory, network or browser.
type Deps struct {
	Stdin       io.Reader
	Terminal    Terminal
	Stdout      io.Writer
	Stderr      io.Writer
	Getenv      func(string) string
	Store       *config.Store
	HTTPClient  *http.Client
	OpenBrowser func(string) error
	Now         func() time.Time
	// CallbackPort and LoginTimeout override the sign-in defaults in tests.
	CallbackPort int
	LoginTimeout time.Duration
}

type app struct {
	Deps
	ctx context.Context
}

type handler struct {
	summary string
	run     func(*app, *command) error
}

// commands is filled by the command files' init functions.
var commands = map[string]handler{}

// aliases maps older or shorter names onto a command.
var commandAliases = map[string]string{
	"accounts": "workspaces",
}

func register(name, summary string, run func(*app, *command) error) {
	commands[name] = handler{summary: summary, run: run}
}

// Run executes one rzv invocation and returns the process exit code.
func Run(ctx context.Context, argv []string, deps Deps) int {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Getenv == nil {
		deps.Getenv = func(string) string { return "" }
	}
	a := &app{Deps: deps, ctx: ctx}
	cmd, err := parse(argv)
	if err != nil {
		return a.fail(nil, err)
	}
	if cmd.name == "" || cmd.name == "help" {
		if cmd.name == "help" && len(cmd.args) > 0 {
			cmd.name, cmd.args = cmd.args[0], nil
			cmd.flags["help"] = true
		} else {
			if cmd.name == "" && !cmd.flags["help"] && len(cmd.options) > 0 {
				return a.fail(cmd, usageError("A command is required. Run `rzv help`."))
			}
			if cmd.name == "" && cmd.flags["version"] {
				return a.runHandler(cmd, commands["version"])
			}
			fmt.Fprint(a.Stdout, topUsage())
			return 0
		}
	}
	name := cmd.name
	if target, ok := commandAliases[name]; ok {
		name = target
	}
	h, ok := commands[name]
	if !ok {
		if name == "staff" {
			return a.fail(cmd, usageError("Staff commands are not part of rzv. Use the Runzivo staff console."))
		}
		return a.fail(cmd, usageError("Unknown command %q. Run `rzv help`.", cmd.name))
	}
	if cmd.flags["help"] {
		usage := commandUsage[name]
		if name == "workspaces" {
			usage = a.workspaceUsage()
		}
		fmt.Fprintf(a.Stdout, "%s\n", usage)
		return 0
	}
	return a.runHandler(cmd, h)
}

func (a *app) runHandler(cmd *command, h handler) int {
	if err := a.selectProfile(cmd); err != nil {
		return a.fail(cmd, err)
	}
	if err := h.run(a, cmd); err != nil {
		return a.fail(cmd, err)
	}
	return 0
}

// fail prints an error to stderr: JSON with --json, a plain line otherwise.
func (a *app) fail(cmd *command, err error) int {
	e := asError(err)
	if e.Quiet {
		return e.Exit
	}
	if cmd != nil && cmd.json() {
		writeJSON(a.Stderr, map[string]string{"error": e.Code, "message": e.Message})
	} else {
		fmt.Fprintf(a.Stderr, "rzv: %s\n", displayLog(e.Message))
	}
	return e.Exit
}

// output writes value as one JSON object with --json, otherwise text.
func (a *app) output(cmd *command, value any, text string) {
	if cmd.json() {
		writeJSON(a.Stdout, value)
		return
	}
	fmt.Fprintln(a.Stdout, text)
}

func writeJSON(w io.Writer, value any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(value)
}

// display escapes control characters so server-supplied text cannot move
// the cursor or rewrite the terminal.
func display(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			fmt.Fprintf(&b, "\\u%04x", r)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// displayLog is display line by line: newlines are kept, every other control
// character is escaped.
func displayLog(value string) string {
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		lines[index] = display(line)
	}
	return strings.Join(lines, "\n")
}

func displayOrDash(value *string) string {
	if value == nil {
		return "-"
	}
	return display(*value)
}

func userAgent() string {
	return fmt.Sprintf("rzv/%s (%s/%s)", version.Version, runtime.GOOS, runtime.GOARCH)
}

func topUsage() string {
	var b strings.Builder
	b.WriteString("rzv is the Runzivo command-line tool.\n\nUsage: rzv <command> [arguments] [--json]\n\nCommands:\n")
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&b, "  %-16s %s\n", name, commands[name].summary)
	}
	b.WriteString("\nRun `rzv help <command>` for details. Add --json for machine-readable output.\n")
	b.WriteString("Staff commands are not included; staff work stays in the Runzivo staff console.\n")
	return b.String()
}

// commandUsage holds each command's detailed help, filled by init functions.
var commandUsage = map[string]string{}
