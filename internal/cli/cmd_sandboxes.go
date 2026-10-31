package cli

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"github.com/Runzivo/rzv-cli/internal/api"
)

// sandboxSizes are the sizes Core defines. Which of them can start right now
// depends on capacity; the list response's sizes field names those.
var sandboxSizes = []string{"2vcpu-8gb", "4vcpu-16gb", "8vcpu-32gb", "16vcpu-64gb"}

const sandboxesUsage = `Usage:
  rzv sandboxes list [--workspace <id>] [--json]
  rzv sandboxes get <sandbox-id> [--workspace <id>] [--json]
  rzv sandboxes create --name <name> [--size 2vcpu-8gb|4vcpu-16gb|8vcpu-32gb|16vcpu-64gb] [--disk-gib <5-64>] [--idempotency-key <key>] [--workspace <id>] [--json]
  rzv sandboxes start|stop|delete <sandbox-id> [--workspace <id>] [--json]
  rzv sandboxes exec <sandbox-id> [--cwd <dir>] [--timeout <1-3600>] [--env KEY=VALUE,...] [--workspace <id>] [--json] -- <command...>
  rzv sandboxes ssh <sandbox-id> [--workspace <id>]
  rzv sandboxes terminal-ticket <sandbox-id> [--workspace <id>] [--json]

A sandbox is a Linux virtual machine with your files in /workspace. It lives eight
hours from its first start, stopped or not, then it and its files are deleted.
Stopping keeps the files and stops the compute charge. A workspace runs a limited
number at once. API keys need sandboxes:read to read and sandboxes:write (or all:all) to change
sandboxes.

exec runs one command and exits with its exit status (124 if it timed out). One
argument after -- is run as a shell script; two or more are run as that exact
argument list, each quoted for the shell. ssh opens an interactive terminal through
Runzivo (macOS and Linux); leave it with the shell's own exit. terminal-ticket
prints a terminal URL for a client that cannot send an Authorization header,
such as a browser; it works once, within about 30 seconds.`

const (
	maxExecTimeout = 3600
	maxEnvEntries  = 64
	maxEnvValue    = 8192
	maxCwd         = 1024
	maxCmd         = 65536
)

var (
	sandboxIDPattern = regexp.MustCompile(`^sbx_[0-9a-f]{24}$`)
	envNamePattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func init() {
	register("sandboxes", "Create sandboxes and run commands in them", runSandboxes)
	commandUsage["sandboxes"] = sandboxesUsage
}

func runSandboxes(a *app, cmd *command) error {
	if len(cmd.args) == 0 {
		return usageError("%s", sandboxesUsage)
	}
	switch cmd.args[0] {
	case "list":
		return sandboxesList(a, cmd)
	case "get":
		return sandboxAction(a, cmd, "get", func(c *api.Client, ws, id string) (*api.Sandbox, error) { return c.SandboxGet(a.ctx, ws, id) }, "")
	case "create":
		return sandboxesCreate(a, cmd)
	case "start":
		return sandboxAction(a, cmd, "start", func(c *api.Client, ws, id string) (*api.Sandbox, error) { return c.SandboxStart(a.ctx, ws, id) }, "Starting")
	case "stop":
		return sandboxAction(a, cmd, "stop", func(c *api.Client, ws, id string) (*api.Sandbox, error) { return c.SandboxStop(a.ctx, ws, id) }, "Stopping")
	case "delete":
		return sandboxAction(a, cmd, "delete", func(c *api.Client, ws, id string) (*api.Sandbox, error) { return c.SandboxDelete(a.ctx, ws, id) }, "Deleting")
	case "exec":
		return sandboxesExec(a, cmd)
	case "ssh":
		return sandboxesSSH(a, cmd)
	case "terminal-ticket":
		return sandboxesTerminalTicket(a, cmd)
	default:
		return usageError("%s", sandboxesUsage)
	}
}

func sandboxID(value string) (string, error) {
	if !sandboxIDPattern.MatchString(value) {
		return "", usageError("A sandbox id looks like sbx_ followed by 24 letters and digits. See `rzv sandboxes list`.")
	}
	return value, nil
}

func sandboxesList(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv sandboxes list [--workspace <id>] [--json]"); err != nil {
		return err
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	listing, err := client.SandboxList(a.ctx, workspace)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": workspace, "sandboxes": listing.Sandboxes, "max_running": listing.MaxRunning, "sizes": listing.Sizes}, renderSandboxList(workspace, listing))
	return nil
}

func renderSandboxList(workspace string, listing *api.SandboxListing) string {
	sandboxes, maxRunning := listing.Sandboxes, listing.MaxRunning
	offered := ""
	if len(listing.Sizes) > 0 {
		offered = "Sizes on offer here: " + display(strings.Join(listing.Sizes, ", ")) + "."
	}
	if len(sandboxes) == 0 {
		text := "No sandboxes in workspace " + display(workspace) + ". Create one with `rzv sandboxes create --name <name>`."
		if offered != "" {
			text += "\n" + offered
		}
		return text
	}
	var out strings.Builder
	table := tabwriter.NewWriter(&out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tNAME\tSIZE\tSTATUS\tTIME LEFT\tCREDITS USED")
	for _, s := range sandboxes {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n", display(s.ID), display(s.Name), display(s.Size), display(s.Status), timeLeft(s), credits(s.CreditsUsed))
	}
	_ = table.Flush()
	if maxRunning > 0 {
		fmt.Fprintf(&out, "\nThis workspace can run %d at once.", maxRunning)
	}
	if offered != "" {
		fmt.Fprintf(&out, "\n%s", offered)
	}
	return strings.TrimRight(out.String(), "\n")
}

func timeLeft(s api.Sandbox) string {
	if s.Status == "deleted" || s.Status == "expired" {
		return "-"
	}
	if s.StartedAt == nil {
		return "8h (starts when started)"
	}
	left := time.Duration(s.RemainingSeconds) * time.Second
	if left <= 0 {
		return "0m"
	}
	hours, minutes := int(left/time.Hour), int(left%time.Hour/time.Minute)
	if hours == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dh%02dm", hours, minutes)
}

func credits(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) + " min" }

func renderSandbox(s *api.Sandbox, verb string) string {
	lines := []string{}
	head := fmt.Sprintf("%s [%s]: %s", display(s.Name), display(s.ID), display(s.Status))
	if verb != "" {
		head = verb + " sandbox " + head
	}
	lines = append(lines, head)
	lines = append(lines, fmt.Sprintf("Size: %s, disk %d GiB", display(s.Size), s.DiskGiB))
	lines = append(lines, "Time left: "+timeLeft(*s))
	if s.ExpiresAt != nil {
		lines = append(lines, "Expires: "+display(*s.ExpiresAt))
	}
	lines = append(lines, fmt.Sprintf("Running time: %s, credits used: %s", (time.Duration(s.RuntimeSeconds)*time.Second).String(), credits(s.CreditsUsed)))
	if s.Error != nil {
		lines = append(lines, "Problem: "+display(s.Error.Message)+" ("+display(s.Error.Code)+")")
	}
	return strings.Join(lines, "\n")
}

func sandboxAction(a *app, cmd *command, name string, call func(*api.Client, string, string) (*api.Sandbox, error), verb string) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	usage := "rzv sandboxes " + name + " <sandbox-id> [--workspace <id>] [--json]"
	if err := cmd.expectArgs(2, usage); err != nil {
		return err
	}
	id, err := sandboxID(cmd.args[1])
	if err != nil {
		return err
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	sandbox, err := call(client, workspace, id)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": workspace, "sandbox": sandbox}, renderSandbox(sandbox, verb))
	return nil
}

func sandboxesCreate(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "name", "size", "disk-gib", "idempotency-key"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv sandboxes create --name <name> [--size 2vcpu-8gb|4vcpu-16gb|8vcpu-32gb|16vcpu-64gb] [--disk-gib <5-64>]"); err != nil {
		return err
	}
	name, err := cmd.required("name")
	if err != nil {
		return err
	}
	if len([]rune(name)) > 80 {
		return usageError("--name must be at most 80 characters.")
	}
	input := api.CreateSandboxInput{Name: name, Size: "2vcpu-8gb"}
	if size, err := cmd.option("size"); err != nil {
		return err
	} else if size != "" {
		if !slices.Contains(sandboxSizes, size) {
			return usageError("--size must be 2vcpu-8gb, 4vcpu-16gb, 8vcpu-32gb or 16vcpu-64gb. `rzv sandboxes list` shows the sizes on offer now.")
		}
		input.Size = size
	}
	if _, ok := cmd.options["disk-gib"]; ok {
		text, err := cmd.required("disk-gib")
		if err != nil {
			return err
		}
		disk, err := strconv.Atoi(text)
		if err != nil || disk < 5 || disk > 64 {
			return usageError("--disk-gib must be a whole number from 5 to 64.")
		}
		input.DiskGiB = disk
	}
	if key, err := cmd.option("idempotency-key"); err != nil {
		return err
	} else if key != "" {
		if len(key) < 8 || len(key) > 200 {
			return usageError("--idempotency-key must be 8 to 200 characters.")
		}
		input.ClientOperationID = key
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	sandbox, err := client.SandboxCreate(a.ctx, workspace, input)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": workspace, "sandbox": sandbox}, renderSandbox(sandbox, "Created"))
	return nil
}

func parseEnv(value string) (map[string]string, error) {
	if value == "" {
		return nil, nil
	}
	env := map[string]string{}
	for _, pair := range strings.Split(value, ",") {
		key, val, ok := strings.Cut(pair, "=")
		if !ok || key == "" || !envNamePattern.MatchString(key) {
			return nil, usageError("--env takes KEY=VALUE pairs separated by commas, such as --env A=1,B=2.")
		}
		if utf8.RuneCountInString(val) > maxEnvValue {
			return nil, usageError("--env values must be at most %d characters (%s is longer).", maxEnvValue, key)
		}
		env[key] = val
	}
	if len(env) > maxEnvEntries {
		return nil, usageError("--env takes at most %d variables.", maxEnvEntries)
	}
	return env, nil
}

func sandboxesExec(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "cwd", "timeout", "env"); err != nil {
		return err
	}
	if len(cmd.args) < 3 {
		return usageError("Use `rzv sandboxes exec <sandbox-id> [--cwd <dir>] [--timeout <seconds>] -- <command...>`.")
	}
	id, err := sandboxID(cmd.args[1])
	if err != nil {
		return err
	}
	input := api.ExecInput{Cmd: execCommand(cmd.args[2:])}
	if input.Cmd == "" {
		return usageError("Give a command after --, such as `rzv sandboxes exec %s -- ls`.", id)
	}
	if utf8.RuneCountInString(input.Cmd) > maxCmd {
		return usageError("The command is too long (at most %d characters).", maxCmd)
	}
	if input.Cwd, err = cmd.option("cwd"); err != nil {
		return err
	}
	if utf8.RuneCountInString(input.Cwd) > maxCwd {
		return usageError("--cwd must be at most %d characters.", maxCwd)
	}
	if _, ok := cmd.options["timeout"]; ok {
		text, err := cmd.required("timeout")
		if err != nil {
			return err
		}
		seconds, err := strconv.Atoi(text)
		if err != nil || seconds < 1 || seconds > maxExecTimeout {
			return usageError("--timeout must be a whole number of seconds from 1 to %d.", maxExecTimeout)
		}
		input.TimeoutSeconds = seconds
	}
	envText, err := cmd.option("env")
	if err != nil {
		return err
	}
	if input.Env, err = parseEnv(envText); err != nil {
		return err
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	handle := func(event api.ExecEvent) error {
		if cmd.json() {
			line := map[string]any{"type": event.Type}
			if event.Type == "exit" {
				line["exit_code"], line["timed_out"], line["truncated"] = event.ExitCode, event.TimedOut, event.Truncated
				if event.Reason != "" {
					line["reason"] = event.Reason
				}
			} else {
				line["data"] = base64.StdEncoding.EncodeToString(event.Data)
			}
			writeJSON(a.Stdout, line)
			return nil
		}
		var target io.Writer = a.Stdout
		if event.Type == "stderr" {
			target = a.Stderr
		}
		if event.Type != "exit" {
			_, err := target.Write(event.Data)
			return err
		}
		return nil
	}
	exit, err := client.SandboxExec(a.ctx, workspace, id, input, handle)
	if err != nil {
		return apiError(err, "")
	}
	switch {
	case exit.Reason != "":
		return execEndedBySandbox(exit)
	case exit.TimedOut:
		return newError("command_timed_out", 124, "The command timed out.")
	case exit.Truncated:
		return newError("output_truncated", 137, "The command was stopped because its output passed 8 MiB. Write large output to a file in the sandbox instead.")
	case exit.ExitCode != 0:
		code := exit.ExitCode
		if code < 1 || code > 255 {
			code = exitFailure
		}
		return newError("command_failed", code, "The command exited with status %d.", exit.ExitCode)
	}
	return nil
}

// execCommand builds the string the sandbox runs with bash -lc. One argument is
// a shell script and goes as it is; two or more are quoted so the remote shell
// sees exactly that argument list.
func execCommand(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}

// execEndedBySandbox explains a command Core ended because its sandbox left
// running. The exit status is the command's, or a failure if it has none.
func execEndedBySandbox(exit api.ExecEvent) error {
	code := exit.ExitCode
	if code < 1 || code > 255 {
		code = exitFailure
	}
	switch exit.Reason {
	case "sandbox_stopped":
		return newError(exit.Reason, code, "The command ended because the sandbox was stopped.")
	case "sandbox_expired":
		return newError(exit.Reason, code, "The command ended because the sandbox reached its eight hour lifetime.")
	case "sandbox_deleted":
		return newError(exit.Reason, code, "The command ended because the sandbox was deleted.")
	}
	return newError("command_ended", code, "The command was ended by Runzivo (%s).", display(exit.Reason))
}

var errNoTerminal = usageError("rzv sandboxes ssh needs an interactive terminal on macOS or Linux. In scripts use `rzv sandboxes exec`.")

func sandboxesSSH(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv sandboxes ssh <sandbox-id> [--workspace <id>]"); err != nil {
		return err
	}
	id, err := sandboxID(cmd.args[1])
	if err != nil {
		return err
	}
	if cmd.json() {
		return usageError("--json is not valid for ssh.")
	}
	terminal := a.Terminal
	if terminal == nil || a.Stdin == nil || !terminal.Interactive() {
		return errNoTerminal
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	sandbox, err := client.SandboxGet(a.ctx, workspace, id)
	if err != nil {
		return apiError(err, "")
	}
	if sandbox.Status != "running" {
		return newError("sandbox_not_running", exitFailure, "The sandbox is %s. Start it with `rzv sandboxes start %s`, and try again when it is running.", display(sandbox.Status), display(id))
	}
	socket, err := client.SandboxTerminal(a.ctx, workspace, id)
	if err != nil {
		return apiError(err, "")
	}
	defer socket.Close()
	cols, rows, err := terminal.Size()
	if err != nil {
		cols, rows = 80, 24
	}
	if err := socket.WriteText(api.ResizeMessage(cols, rows)); err != nil {
		return unavailableError("The terminal connection failed.")
	}
	rawRestore, err := terminal.MakeRaw()
	if err != nil {
		return newError("terminal_unavailable", exitFailure, "%s.", err.Error())
	}
	// The terminal goes back on a normal exit and when the process is asked to
	// end (SIGTERM, SIGHUP), whichever comes first; restoring twice is harmless.
	var restoreOnce sync.Once
	restore := func() { restoreOnce.Do(rawRestore) }
	defer restore()
	var hungUp atomic.Bool
	defer terminal.OnHangup(func() {
		hungUp.Store(true)
		restore()
		_ = socket.Close()
	})()
	defer terminal.OnResize(func() {
		if cols, rows, err := terminal.Size(); err == nil {
			_ = socket.WriteText(api.ResizeMessage(cols, rows))
		}
	})()
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := a.Stdin.Read(buffer)
			if n > 0 && socket.WriteBinary(buffer[:n]) != nil {
				return
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		kind, data, err := socket.ReadMessage()
		var closed *api.CloseError
		switch {
		case errors.As(err, &closed):
			return sandboxTerminalEnded(closed)
		case err != nil:
			if hungUp.Load() {
				return newError("terminal_closed", exitFailure, "The terminal was closed.")
			}
			if a.ctx.Err() != nil {
				return nil
			}
			return unavailableError("The terminal connection was lost. Run the command again for a new session.")
		case kind == api.MessageBinary:
			if _, err := a.Stdout.Write(data); err != nil {
				return nil
			}
		}
	}
}

// sandboxTerminalEnded explains a close from Core. A shell that exited by
// itself closes normally, with shell_exited or no reason.
func sandboxTerminalEnded(closed *api.CloseError) error {
	switch {
	case closed.Code == 1000:
		switch closed.Reason {
		case "sandbox_stopped":
			return newError("sandbox_stopped", exitFailure, "The sandbox was stopped.")
		case "sandbox_expired":
			return newError("sandbox_expired", exitFailure, "The sandbox reached its eight hour lifetime and was deleted.")
		case "sandbox_deleted":
			return newError("sandbox_deleted", exitFailure, "The sandbox was deleted.")
		case "idle_timeout":
			return newError("idle_timeout", exitFailure, "The terminal was closed after 30 minutes with no input or output.")
		}
		return nil
	case closed.Code == 1008 && closed.Reason == "input_rate":
		return newError("input_rate", exitFailure, "The terminal was closed because input arrived too fast (more than 256 KiB a second). Copy large files with `rzv sandboxes exec` instead.")
	case closed.Code == 1011 && closed.Reason == "output_overflow":
		return newError("output_overflow", exitFailure, "The terminal was closed because its output was not read fast enough. Run the command again for a new session.")
	}
	return unavailableError("The terminal connection ended unexpectedly (code %d). Run the command again for a new session.", closed.Code)
}

func sandboxesTerminalTicket(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv sandboxes terminal-ticket <sandbox-id> [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, err := sandboxID(cmd.args[1])
	if err != nil {
		return err
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	ticket, err := client.SandboxTerminalTicket(a.ctx, workspace, id)
	var apiErr *api.Error
	if errors.As(err, &apiErr) && apiErr.Status == 503 && apiErr.Code == "sandbox_exec_unavailable" {
		return newError(apiErr.Code, exitUnavailable, "Terminal tickets are not available on this Runzivo environment yet.")
	}
	if err != nil {
		return apiError(err, "")
	}
	connect, err := ticket.ConnectURL()
	if err != nil {
		return apiError(err, "")
	}
	// The ticket goes to stdout only; the note on stderr leaves stdout to the
	// URL or the JSON, so either can be piped.
	a.output(cmd, map[string]any{"workspace_id": workspace, "sandbox_id": id, "url": ticket.URL, "ticket": ticket.Ticket, "expires_at": ticket.ExpiresAt, "connect_url": connect}, display(connect))
	fmt.Fprintf(a.Stderr, "The ticket works once and expires in about 30 seconds (at %s).\n", display(ticket.ExpiresAt))
	return nil
}
