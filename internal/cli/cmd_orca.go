package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Runzivo/rzv-cli/internal/api"
	"github.com/Runzivo/rzv-cli/internal/config"
	"github.com/Runzivo/rzv-cli/internal/sshstate"
)

// The orca commands are the lifecycle the Runzivo plugin for Orca runs. Each
// Orca workspace gets one sandbox, reached through the same OpenSSH config as
// `rzv sandboxes ssh-config --write`. create, suspend, resume and destroy
// follow Orca's VM recipe contract: progress on stderr, and create and resume
// print one recipe result on stdout.
const orcaUsage = `Usage:
  rzv orca create [--size 2vcpu-8gb|4vcpu-16gb|8vcpu-32gb|16vcpu-64gb] [--disk-gib <5-64>] [--lifetime 8h|12h|24h] [--workspace <id>]
  rzv orca suspend|resume|destroy
  rzv orca defaults [--size <size>] [--disk-gib <5-64>] [--lifetime 8h|12h|24h] [--workspace <id>] [--reset] [--json]
  rzv orca status [--workspace <id>] [--json]

Orca runs create, suspend, resume and destroy from the Runzivo plugin's
recipes; you do not run them yourself. create makes one sandbox for the Orca
workspace, copies the commit Orca selected into /workspace/<repo>, and writes
its SSH config under ~/.ssh/rzv (as ssh-config --write does). suspend stops the
sandbox (files kept, no compute charge), resume starts it again, and destroy
deletes it with its files. The lifetime still runs while stopped and can never
pass 24 hours.

defaults sets the disk, lifetime, size and workspace that create uses when its
recipe does not choose them, for this profile. status lists the sandboxes Orca
made, with their expiry and credits used.`

const (
	orcaNamePrefix    = "orca-"
	orcaProvider      = "runzivo"
	orcaSandboxUser   = "user"
	orcaWorkspaceRoot = "/workspace"
)

// Waits are variables so tests run quickly.
var (
	orcaRunningWait = 10 * time.Minute
	orcaStoppedWait = 3 * time.Minute
	orcaPollEvery   = 2 * time.Second
	orcaSSHAttempts = 10
	orcaSSHRetry    = 3 * time.Second
	// orcaCleanupWait stays inside Orca's 5-second grace before SIGKILL.
	orcaCleanupWait = 4 * time.Second
)

var (
	orcaInstancePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,150}$`)
	orcaRepoNameUnsafe  = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	orcaSlugUnsafe      = regexp.MustCompile(`[^a-z0-9]+`)
	orcaBranchPattern   = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,200}$`)
	orcaCommitPattern   = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	orcaRemotePattern   = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

// orcaRun runs a local program (git, ssh) with the given working directory and
// streams. Tests replace it.
var orcaRun = func(ctx context.Context, dir string, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error {
	c := exec.CommandContext(ctx, name, args...)
	c.Dir, c.Stdin, c.Stdout, c.Stderr = dir, stdin, stdout, stderr
	return c.Run()
}

func init() {
	register("orca", "Runzivo sandboxes for Orca workspaces", runOrca)
	commandUsage["orca"] = orcaUsage
}

// orcaResult is Orca's provisioned-root recipe result (schema version 2).
type orcaResult struct {
	SchemaVersion int            `json:"schemaVersion"`
	CheckoutMode  string         `json:"checkoutMode"`
	Connection    orcaConnection `json:"connection"`
	UserData      orcaUserData   `json:"userData"`
}

type orcaConnection struct {
	Type        string     `json:"type"`
	ProjectRoot string     `json:"projectRoot"`
	Target      orcaTarget `json:"target"`
}

type orcaTarget struct {
	Label      string `json:"label"`
	ConfigHost string `json:"configHost"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
}

// orcaUserData is what later lifecycle calls need. Orca stores it locally
// and sends it back on stdin; it holds no credentials.
type orcaUserData struct {
	Provider      string  `json:"provider"`
	SandboxID     string  `json:"sandboxId"`
	WorkspaceID   string  `json:"workspaceId"`
	Profile       string  `json:"profile"`
	Size          string  `json:"size,omitempty"`
	DiskGiB       int     `json:"diskGiB,omitempty"`
	LifetimeHours int     `json:"lifetimeHours,omitempty"`
	ExpiresAt     *string `json:"expiresAt,omitempty"`
}

type orcaLifecyclePayload struct {
	Mode         string `json:"mode"`
	RecipeResult struct {
		Connection struct {
			ProjectRoot string `json:"projectRoot"`
		} `json:"connection"`
		UserData orcaUserData `json:"userData"`
	} `json:"recipeResult"`
}

// orcaDefaults are a profile's create defaults, saved by `rzv orca defaults`.
type orcaDefaults struct {
	Size          string `json:"size,omitempty"`
	DiskGiB       int    `json:"disk_gib,omitempty"`
	LifetimeHours int    `json:"lifetime_hours,omitempty"`
	Workspace     string `json:"workspace,omitempty"`
}

func runOrca(a *app, cmd *command) error {
	if len(cmd.args) == 0 {
		return usageError("%s", orcaUsage)
	}
	if cmd.args[0] == "defaults" {
		return orcaDefaultsCommand(a, cmd)
	}
	if cmd.args[0] == "status" {
		return orcaStatus(a, cmd)
	}
	if cmd.json() {
		return usageError("Orca reads these commands' JSON from stdout already; leave out --json.")
	}
	// Orca reads exactly one JSON object from stdout, so everything the
	// shared helpers print goes to stderr.
	out := a.Stdout
	a.Stdout = a.Stderr
	// Orca cancels a step with SIGTERM to the process group, then SIGKILL 5
	// seconds later. Turning SIGTERM into a cancelled context lets a create
	// delete the sandbox it made in that window.
	ctx, stop := signal.NotifyContext(a.ctx, orcaStopSignals...)
	defer stop()
	a.ctx = ctx
	var err error
	switch cmd.args[0] {
	case "create":
		err = orcaCreate(a, cmd, out)
	case "suspend":
		err = orcaSuspend(a, cmd)
	case "resume":
		err = orcaResume(a, cmd, out)
	case "destroy":
		err = orcaDestroy(a, cmd)
	default:
		return usageError("%s", orcaUsage)
	}
	if err != nil && ctx.Err() != nil {
		return newError("cancelled", exitFailure, "Cancelled.")
	}
	return err
}

func (a *app) orcaLog(format string, args ...any) {
	fmt.Fprintf(a.Stderr, "rzv: "+format+"\n", args...)
}

func orcaDefaultsPath(a *app) string {
	return filepath.Join(a.Store.Dir, "orca", a.profileName()+".json")
}

func readOrcaDefaults(a *app) (orcaDefaults, error) {
	var d orcaDefaults
	if a.Store == nil {
		return d, nil
	}
	if _, err := os.Lstat(orcaDefaultsPath(a)); errors.Is(err, os.ErrNotExist) {
		return d, nil
	}
	data, err := sshstate.ReadPrivate(orcaDefaultsPath(a))
	if err != nil {
		return d, err
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return d, newError("orca_defaults_invalid", exitFailure, "The saved Orca defaults are unreadable. Run `rzv orca defaults --reset`.")
	}
	return d, nil
}

func parseOrcaSize(value string) (string, error) {
	if !slices.Contains(sandboxSizes, value) {
		return "", usageError("--size must be one of %s.", strings.Join(sandboxSizes, ", "))
	}
	return value, nil
}

func parseOrcaDisk(value string) (int, error) {
	disk, err := strconv.Atoi(value)
	if err != nil || disk < 5 || disk > 64 {
		return 0, usageError("--disk-gib must be a whole number from 5 to 64.")
	}
	return disk, nil
}

func parseOrcaLifetime(value string) (int, error) {
	hours, ok := sandboxLifetimes[value]
	if !ok {
		return 0, usageError("--lifetime must be 8h, 12h or 24h.")
	}
	return hours, nil
}

func orcaDefaultsCommand(a *app, cmd *command) error {
	if err := cmd.allow("size", "disk-gib", "lifetime", "workspace", "reset"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv orca defaults [--size <size>] [--disk-gib <5-64>] [--lifetime 8h|12h|24h] [--workspace <id>] [--reset]"); err != nil {
		return err
	}
	if a.Store == nil {
		return newError("no_config", exitFailure, "No configuration directory is available.")
	}
	reset := cmd.flags["reset"]
	var err error
	d := orcaDefaults{}
	if !reset {
		if d, err = readOrcaDefaults(a); err != nil {
			return err
		}
	}
	changed := reset
	if v, err := cmd.option("size"); err != nil {
		return err
	} else if v != "" {
		if d.Size, err = parseOrcaSize(v); err != nil {
			return err
		}
		changed = true
	}
	if v, err := cmd.option("disk-gib"); err != nil {
		return err
	} else if v != "" {
		if d.DiskGiB, err = parseOrcaDisk(v); err != nil {
			return err
		}
		changed = true
	}
	if v, err := cmd.option("lifetime"); err != nil {
		return err
	} else if v != "" {
		if d.LifetimeHours, err = parseOrcaLifetime(v); err != nil {
			return err
		}
		changed = true
	}
	if v, err := cmd.option("workspace"); err != nil {
		return err
	} else if v != "" {
		d.Workspace = v
		changed = true
	}
	if changed {
		if err := sshstate.PrivateDir(filepath.Dir(orcaDefaultsPath(a))); err != nil {
			return err
		}
		data, _ := json.Marshal(d)
		if err := sshstate.WritePrivate(orcaDefaultsPath(a), data); err != nil {
			return err
		}
	}
	show := func(v string) string {
		if v == "" {
			return "-"
		}
		return v
	}
	lifetime, disk := "8h", "20 GiB"
	if d.LifetimeHours > 0 {
		lifetime = fmt.Sprintf("%dh", d.LifetimeHours)
	}
	if d.DiskGiB > 0 {
		disk = fmt.Sprintf("%d GiB", d.DiskGiB)
	}
	text := fmt.Sprintf("Orca defaults for profile %s:\n  Size: %s (the recipe's size wins)\n  Disk: %s\n  Lifetime: %s\n  Workspace: %s", a.profileName(), show(d.Size), disk, lifetime, show(d.Workspace))
	a.output(cmd, map[string]any{"profile": a.profileName(), "defaults": d}, text)
	return nil
}

func orcaStatus(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv orca status [--workspace <id>] [--json]"); err != nil {
		return err
	}
	if _, set := cmd.options["workspace"]; !set {
		if d, err := readOrcaDefaults(a); err == nil && d.Workspace != "" {
			cmd.options["workspace"] = d.Workspace
		}
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	listing, err := client.SandboxList(a.ctx, workspace)
	if err != nil {
		return apiError(err, "")
	}
	mine := *listing
	mine.Sandboxes = nil
	for _, s := range listing.Sandboxes {
		if strings.HasPrefix(s.Name, orcaNamePrefix) {
			mine.Sandboxes = append(mine.Sandboxes, s)
		}
	}
	if mine.Sandboxes == nil {
		mine.Sandboxes = []api.Sandbox{}
	}
	a.output(cmd, map[string]any{"workspace_id": workspace, "sandboxes": mine.Sandboxes}, renderSandboxList(workspace, &mine))
	return nil
}

// orcaCreateInput resolves create's request from the recipe's flags, then the
// profile's saved defaults, then Core's defaults.
func orcaCreateInput(a *app, cmd *command) (api.CreateSandboxInput, orcaDefaults, error) {
	d, err := readOrcaDefaults(a)
	if err != nil {
		return api.CreateSandboxInput{}, d, err
	}
	input := api.CreateSandboxInput{Size: "2vcpu-8gb", DiskGiB: d.DiskGiB, LifetimeHours: d.LifetimeHours}
	if d.Size != "" {
		input.Size = d.Size
	}
	if v, err := cmd.option("size"); err != nil {
		return input, d, err
	} else if v != "" {
		if input.Size, err = parseOrcaSize(v); err != nil {
			return input, d, err
		}
	}
	if v, err := cmd.option("disk-gib"); err != nil {
		return input, d, err
	} else if v != "" {
		if input.DiskGiB, err = parseOrcaDisk(v); err != nil {
			return input, d, err
		}
	}
	if v, err := cmd.option("lifetime"); err != nil {
		return input, d, err
	} else if v != "" {
		if input.LifetimeHours, err = parseOrcaLifetime(v); err != nil {
			return input, d, err
		}
	}
	// 8h is Core's default; leaving it out keeps older services working.
	if input.LifetimeHours == 8 {
		input.LifetimeHours = 0
	}
	if _, set := cmd.options["workspace"]; !set && d.Workspace != "" {
		cmd.options["workspace"] = d.Workspace
	}
	return input, d, nil
}

// orcaSandboxName is "orca-<workspace>-<instance>", unique per Orca
// workspace because Core refuses a second sandbox with the same name.
func orcaSandboxName(workspaceName, instance string) string {
	short := strings.TrimPrefix(instance, "orca-")
	short = orcaSlugUnsafe.ReplaceAllString(strings.ToLower(short), "")
	if len(short) > 8 {
		short = short[:8]
	}
	slug := strings.Trim(orcaSlugUnsafe.ReplaceAllString(strings.ToLower(workspaceName), "-"), "-")
	if len(slug) > 50 {
		slug = strings.Trim(slug[:50], "-")
	}
	if slug == "" {
		slug = "workspace"
	}
	return orcaNamePrefix + slug + "-" + short
}

func orcaCreate(a *app, cmd *command, out io.Writer) error {
	if err := cmd.allow("size", "disk-gib", "lifetime", "workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv orca create [--size <size>] [--disk-gib <5-64>] [--lifetime 8h|12h|24h]"); err != nil {
		return err
	}
	if v := a.Getenv("ORCA_RECIPE_RESULT_SCHEMA_VERSION"); v != "2" {
		return newError("orca_schema_unsupported", exitUsage, "The Runzivo recipe gives each workspace its own sandbox checkout (Orca recipe schema 2), but Orca asked for schema %q. Update Orca to 1.4.246 or later.", v)
	}
	instance := strings.TrimSpace(a.Getenv("ORCA_VM_INSTANCE_ID"))
	if !orcaInstancePattern.MatchString(instance) {
		return newError("orca_instance_missing", exitUsage, "ORCA_VM_INSTANCE_ID is missing or invalid. Orca sets it when it runs this recipe.")
	}
	input, _, err := orcaCreateInput(a, cmd)
	if err != nil {
		return err
	}
	input.Name = orcaSandboxName(a.Getenv("ORCA_WORKSPACE_NAME"), instance)
	// One sandbox per Orca workspace: a retried create returns the sandbox
	// the first attempt made instead of a second one.
	input.ClientOperationID = "orca-" + strings.TrimPrefix(instance, "orca-")
	source, err := a.orcaSource()
	if err != nil {
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
	if listing.SSHEnabled != nil && !*listing.SSHEnabled {
		return newError("sandbox_ssh_unavailable", exitUnavailable, "SSH is not enabled for sandboxes in this Runzivo environment, and Orca needs it.")
	}
	if len(listing.Sizes) > 0 && !slices.Contains(listing.Sizes, input.Size) {
		return newError("size_unavailable", exitFailure, "Size %s is not on offer here now. On offer: %s.", input.Size, strings.Join(listing.Sizes, ", "))
	}
	if hours := max(input.LifetimeHours, 8); !slices.Contains(listing.Lifetimes, hours) {
		return newError("lifetime_unavailable", exitFailure, "A %dh lifetime is not on offer here now. Run `rzv orca defaults --lifetime 8h` or pick another.", hours)
	}
	a.orcaLog("Creating Runzivo sandbox %s (%s, %s)...", input.Name, sizeText(input.Size), lifetimeHoursText(input.LifetimeHours))
	sandbox, err := client.SandboxCreate(a.ctx, workspace, input)
	if err != nil {
		if a.ctx.Err() != nil {
			// Core may have made it before the request was cut off.
			a.orcaCleanupByName(client, workspace, input.Name)
		}
		return a.sandboxCreateError(err, input.Size)
	}
	// From here on a failure removes the sandbox: Orca has no result to
	// destroy, so nothing else would.
	id, ok := sandbox.ID, false
	defer func() {
		if !ok {
			a.orcaCleanup(client, workspace, id)
		}
	}()
	if sandbox, err = a.orcaWaitRunning(client, workspace, id); err != nil {
		return err
	}
	if err := a.orcaWriteSSHConfig(cmd, sandbox.ID, workspace, client); err != nil {
		return err
	}
	root := path.Join(orcaWorkspaceRoot, source.repoName)
	if err := a.orcaPushSource(sandbox.ID, root, source); err != nil {
		return err
	}
	result := a.orcaResultFor(sandbox, workspace, root)
	ok = true
	a.orcaSummary(sandbox)
	writeJSON(out, result)
	return nil
}

func sizeText(size string) string {
	switch size {
	case "2vcpu-8gb":
		return "2 vCPU, 8 GB RAM, 1x equivalent minutes"
	case "4vcpu-16gb":
		return "4 vCPU, 16 GB RAM, 2x equivalent minutes"
	case "8vcpu-32gb":
		return "8 vCPU, 32 GB RAM, 4x equivalent minutes"
	case "16vcpu-64gb":
		return "16 vCPU, 64 GB RAM, 8x equivalent minutes"
	}
	return size
}

func lifetimeHoursText(hours int) string {
	if hours == 0 {
		hours = 8
	}
	return fmt.Sprintf("%dh lifetime", hours)
}

// sizeMultiplier is the equivalent-minute rate of a size.
func sizeMultiplier(size string) int {
	switch size {
	case "4vcpu-16gb":
		return 2
	case "8vcpu-32gb":
		return 4
	case "16vcpu-64gb":
		return 8
	}
	return 1
}

func (a *app) orcaSummary(s *api.Sandbox) {
	a.orcaLog("Sandbox %s is running: %s, %d GiB disk.", s.ID, sizeText(s.Size), s.DiskGiB)
	a.orcaLog("Expires %s. It then stops and its files are deleted; nothing extends it.", expiryText(*s))
	a.orcaLog("While running it uses about %d equivalent minutes per hour from the workspace's credits. Suspending the Orca workspace stops the charge.", 60*sizeMultiplier(s.Size))
}

func (a *app) orcaResultFor(s *api.Sandbox, workspace, root string) orcaResult {
	alias := s.ID + ".rzv"
	return orcaResult{
		SchemaVersion: 2,
		CheckoutMode:  "provisioned-root",
		Connection: orcaConnection{
			Type:        "ssh",
			ProjectRoot: root,
			Target:      orcaTarget{Label: "Runzivo " + s.Name, ConfigHost: alias, Host: alias, Port: 22, Username: orcaSandboxUser},
		},
		UserData: orcaUserData{
			Provider: orcaProvider, SandboxID: s.ID, WorkspaceID: workspace, Profile: a.profileName(),
			Size: s.Size, DiskGiB: s.DiskGiB, LifetimeHours: s.LifetimeHours, ExpiresAt: s.ExpiresAt,
		},
	}
}

// orcaWaitRunning polls until the sandbox runs, failing on any state it
// cannot run from.
func (a *app) orcaWaitRunning(client *api.Client, workspace, id string) (*api.Sandbox, error) {
	deadline := a.Now().Add(orcaRunningWait)
	last := ""
	for {
		s, err := client.SandboxGet(a.ctx, workspace, id)
		if err != nil {
			return nil, apiError(err, "")
		}
		switch s.Status {
		case "running":
			return s, nil
		case "failed":
			msg := "the sandbox failed to start"
			if s.Error != nil {
				msg = s.Error.Message
			}
			return nil, newError("sandbox_failed", exitFailure, "Sandbox %s failed: %s", id, displayLog(msg))
		case "expired":
			return nil, newError("sandbox_expired", exitFailure, "Sandbox %s reached its lifetime and was deleted with its files. Delete this Orca workspace and create a new one.", id)
		case "deleting", "deleted", "cleanup_pending":
			return nil, newError("sandbox_deleted", exitFailure, "Sandbox %s was deleted. Delete this Orca workspace and create a new one.", id)
		}
		if s.Status != last {
			a.orcaLog("Sandbox %s is %s...", id, statusLabel(s.Status))
			last = s.Status
		}
		if a.Now().After(deadline) {
			return nil, newError("sandbox_start_timeout", exitFailure, "Sandbox %s did not start within %s. It was still %s.", id, orcaRunningWait, statusLabel(s.Status))
		}
		if err := sleepContext(a.ctx, orcaPollEvery); err != nil {
			return nil, err
		}
	}
}

func (a *app) orcaWriteSSHConfig(cmd *command, id, workspace string, client *api.Client) error {
	config, err := a.sandboxSSHConfigText(cmd, id, workspace, client)
	if err != nil {
		return err
	}
	return a.writeSSHConfig(id, config)
}

// orcaCleanupByName deletes the sandbox a cut-off create request may have
// made; the name is unique in the workspace.
func (a *app) orcaCleanupByName(client *api.Client, workspace, name string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(a.ctx), orcaCleanupWait)
	defer cancel()
	listing, err := client.SandboxList(ctx, workspace)
	if err != nil {
		a.orcaLog("Could not check for a sandbox named %s after the cancelled create. Check with `rzv orca status`.", name)
		return
	}
	for _, s := range listing.Sandboxes {
		if s.Name == name {
			a.orcaCleanup(client, workspace, s.ID)
		}
	}
}

// orcaCleanup deletes a sandbox create made, on a context that outlives a
// cancelled create but ends before Orca's SIGKILL.
func (a *app) orcaCleanup(client *api.Client, workspace, id string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(a.ctx), orcaCleanupWait)
	defer cancel()
	if _, err := client.SandboxDelete(ctx, workspace, id); err != nil && !isSandboxGone(err) {
		a.orcaLog("Could not delete sandbox %s after the failed create: %s. Delete it with `rzv sandboxes delete %s`.", id, displayLog(asError(apiError(err, "")).Message), id)
		return
	}
	a.removeOrcaSSHConfig(id)
	a.orcaLog("Deleted sandbox %s after the failed create.", id)
}

func isSandboxGone(err error) bool {
	var e *api.Error
	return errors.As(err, &e) && (e.Status == 404 || e.Code == "sandbox_expired" || e.Code == "sandbox_deleted")
}

// removeOrcaSSHConfig removes the sandbox's ~/.ssh/rzv config file, only if
// rzv wrote it. The Include line stays for other sandboxes.
func (a *app) removeOrcaSSHConfig(id string) {
	home := a.Getenv("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home == "" {
		return
	}
	file := filepath.Join(home, ".ssh", "rzv", id+".conf")
	if data, err := os.ReadFile(file); err == nil && bytes.HasPrefix(data, []byte("# Managed by rzv.")) {
		_ = os.Remove(file)
	}
}

// orcaSourceInfo is the commit Orca selected and where it goes.
type orcaSourceInfo struct {
	repoPath  string
	commit    string
	ref       string
	branch    string
	repoName  string
	remotes   [][2]string
	userName  string
	userEmail string
}

func (a *app) orcaGit(dir string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	if err := orcaRun(a.ctx, dir, nil, &stdout, &stderr, "git", args...); err != nil {
		return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// orcaSource reads the pinned source from Orca's environment. `orca vm
// recipe doctor --provision` passes no source, so it falls back to HEAD.
func (a *app) orcaSource() (*orcaSourceInfo, error) {
	repoPath := a.Getenv("ORCA_REPO_PATH")
	if repoPath == "" {
		var err error
		if repoPath, err = os.Getwd(); err != nil {
			return nil, err
		}
	}
	s := &orcaSourceInfo{repoPath: repoPath, commit: a.Getenv("ORCA_REPO_REF_HEAD"), branch: a.Getenv("ORCA_REPO_BRANCH")}
	fail := func(format string, args ...any) (*orcaSourceInfo, error) {
		return nil, newError("orca_source", exitFailure, format, args...)
	}
	if s.commit == "" {
		head, err := a.orcaGit(repoPath, "rev-parse", "HEAD")
		if err != nil {
			return fail("%s is not a Git repository with a commit: %s", repoPath, displayLog(err.Error()))
		}
		s.commit = head
		a.orcaLog("Orca sent no pinned commit (a recipe self-test?); using HEAD %s.", head[:min(12, len(head))])
	}
	if !orcaCommitPattern.MatchString(s.commit) {
		return fail("The commit Orca selected is not a full commit id.")
	}
	if s.branch == "" {
		s.branch = "orca-" + s.commit[:12]
	}
	if !orcaBranchPattern.MatchString(s.branch) || strings.HasPrefix(s.branch, "-") || strings.Contains(s.branch, "..") {
		return fail("Branch %q cannot be used in the sandbox.", s.branch)
	}
	if _, err := a.orcaGit(repoPath, "cat-file", "-e", s.commit+"^{commit}"); err != nil {
		// The commit is on a remote but not here: fetch it with the user's own
		// local Git credentials, which never reach the sandbox.
		repoURL, ref := a.Getenv("ORCA_REPO_URL"), a.Getenv("ORCA_REPO_REF")
		if repoURL == "" || ref == "" {
			return fail("Commit %s is not in %s.", s.commit[:12], repoPath)
		}
		a.orcaLog("Fetching %s...", ref)
		if _, err := a.orcaGit(repoPath, "fetch", "--quiet", "--", repoURL, ref); err != nil {
			return fail("Could not fetch %s: %s", ref, displayLog(err.Error()))
		}
		if _, err := a.orcaGit(repoPath, "cat-file", "-e", s.commit+"^{commit}"); err != nil {
			return fail("Commit %s is not in %s after fetching %s.", s.commit[:12], repoPath, ref)
		}
	}
	names, _ := a.orcaGit(repoPath, "remote")
	for _, name := range strings.Fields(names) {
		if !orcaRemotePattern.MatchString(name) {
			continue
		}
		u, err := a.orcaGit(repoPath, "remote", "get-url", name)
		if err != nil || u == "" {
			continue
		}
		if clean, ok := stripURLCredentials(u); ok {
			s.remotes = append(s.remotes, [2]string{name, clean})
		} else {
			a.orcaLog("Leaving out remote %s: its URL could not be checked for credentials.", name)
		}
	}
	s.repoName = orcaRepoName(a.Getenv("ORCA_REPO_URL"), s.remotes, repoPath)
	s.userName, _ = a.orcaGit(repoPath, "config", "user.name")
	s.userEmail, _ = a.orcaGit(repoPath, "config", "user.email")
	return s, nil
}

// stripURLCredentials drops a user and password from an http(s) URL so a
// token in a local remote never reaches the sandbox.
// An http(s) URL that does not parse but may hold user info is dropped
// rather than copied as it is.
func stripURLCredentials(raw string) (string, bool) {
	lower := strings.ToLower(raw)
	web := strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
	u, err := url.Parse(raw)
	if err != nil {
		if web {
			return "", false
		}
		return raw, true
	}
	if !web || u.User == nil {
		return raw, true
	}
	u.User = nil
	return u.String(), true
}

func orcaRepoName(repoURL string, remotes [][2]string, repoPath string) string {
	candidates := []string{repoURL}
	for _, r := range remotes {
		if r[0] == "origin" {
			candidates = append(candidates, r[1])
		}
	}
	candidates = append(candidates, repoPath)
	for _, c := range candidates {
		c = strings.TrimRight(strings.TrimSpace(c), "/")
		if c == "" {
			continue
		}
		if i := strings.LastIndexAny(c, "/:"); i >= 0 {
			c = c[i+1:]
		}
		c = strings.TrimSuffix(c, ".git")
		c = orcaRepoNameUnsafe.ReplaceAllString(c, "-")
		for strings.Contains(c, "..") {
			c = strings.ReplaceAll(c, "..", ".")
		}
		c = strings.Trim(c, "-.")
		if c != "" {
			return c[:min(len(c), 80)]
		}
	}
	return "repo"
}

// orcaSSH runs one command in the sandbox through its rzv SSH config,
// retrying while the guest's SSH comes up.
func (a *app) orcaSSH(id string, stdin func() (io.Reader, func(), error), script string) error {
	var last string
	for attempt := 1; ; attempt++ {
		var in io.Reader
		done := func() {}
		if stdin != nil {
			var err error
			if in, done, err = stdin(); err != nil {
				return err
			}
		}
		var stderr bytes.Buffer
		err := orcaRun(a.ctx, "", in, a.Stderr, &stderr, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=30", id+".rzv", script)
		done()
		if err == nil {
			return nil
		}
		last = strings.TrimSpace(stderr.String())
		var exitErr *exec.ExitError
		// 255 is OpenSSH's own failure (connection or authentication); any
		// other status is the remote script's answer and is not retried.
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 255 || attempt >= orcaSSHAttempts || a.ctx.Err() != nil {
			if last == "" {
				last = err.Error()
			}
			return newError("orca_ssh", exitFailure, "SSH to sandbox %s failed: %s", id, displayLog(last))
		}
		if err := sleepContext(a.ctx, orcaSSHRetry); err != nil {
			return err
		}
	}
}

// orcaPushSource bundles the pinned commit locally and checks it out at root
// in the sandbox, so no Git credential reaches the sandbox.
func (a *app) orcaPushSource(id, root string, s *orcaSourceInfo) error {
	dir, err := os.MkdirTemp("", "rzv-orca-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	ref := fmt.Sprintf("refs/rzv-orca/%d-%d", os.Getpid(), time.Now().UnixNano())
	file := filepath.Join(dir, "repo.bundle")
	if _, err := a.orcaGit(s.repoPath, "update-ref", ref, s.commit); err != nil {
		return newError("orca_source", exitFailure, "Could not bundle %s: %s", s.commit[:12], displayLog(err.Error()))
	}
	_, bundleErr := a.orcaGit(s.repoPath, "bundle", "create", "--quiet", file, ref)
	_, _ = a.orcaGit(s.repoPath, "update-ref", "-d", ref)
	if bundleErr != nil {
		return newError("orca_source", exitFailure, "Could not bundle %s: %s", s.commit[:12], displayLog(bundleErr.Error()))
	}
	if info, err := os.Stat(file); err == nil {
		a.orcaLog("Copying %s (%.1f MB) to the sandbox...", s.commit[:12], float64(info.Size())/1e6)
	}
	q := shellArg
	var b strings.Builder
	fmt.Fprintf(&b, "set -eu\nroot=%s\ncommit=%s\nbranch=%s\nref=%s\n", q(root), q(s.commit), q(s.branch), q(ref))
	b.WriteString(`command -v git >/dev/null 2>&1 || { echo "git is not installed in this sandbox image" >&2; exit 3; }
bundle="$(mktemp "${TMPDIR:-/tmp}/rzv-orca.XXXXXX")"
trap 'rm -f "$bundle"' EXIT
cat > "$bundle"
if [ -e "$root/.git" ]; then
  # A retried create reached the same sandbox; accept only the same commit.
  [ "$(git -C "$root" rev-parse HEAD)" = "$commit" ] || { echo "$root already holds another checkout" >&2; exit 4; }
  exit 0
fi
mkdir -p "$root"
git init --quiet "$root"
cd "$root"
git fetch --quiet "$bundle" "$ref:refs/rzv-orca/source"
git checkout --quiet -B "$branch" "$commit"
git update-ref -d refs/rzv-orca/source
`)
	for _, r := range s.remotes {
		fmt.Fprintf(&b, "git remote add %s %s\n", q(r[0]), q(r[1]))
	}
	if s.userName != "" {
		fmt.Fprintf(&b, "git config user.name %s\n", q(s.userName))
	}
	if s.userEmail != "" {
		fmt.Fprintf(&b, "git config user.email %s\n", q(s.userEmail))
	}
	open := func() (io.Reader, func(), error) {
		f, err := os.Open(file)
		if err != nil {
			return nil, nil, err
		}
		return f, func() { f.Close() }, nil
	}
	if err := a.orcaSSH(id, open, b.String()); err != nil {
		return err
	}
	a.orcaLog("Checked out %s on branch %s at %s.", s.commit[:12], s.branch, root)
	return nil
}

// orcaPayload reads Orca's lifecycle JSON and points this run at the profile
// and workspace create used.
func (a *app) orcaPayload(cmd *command) (*orcaLifecyclePayload, error) {
	if err := cmd.allow(); err != nil {
		return nil, err
	}
	if err := cmd.expectArgs(1, "rzv orca "+cmd.args[0]); err != nil {
		return nil, err
	}
	if a.Stdin == nil {
		return nil, newError("orca_payload", exitUsage, "Orca sends this command its workspace on standard input.")
	}
	data, err := io.ReadAll(io.LimitReader(a.Stdin, 1<<20))
	if err != nil {
		return nil, err
	}
	var p orcaLifecyclePayload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, newError("orca_payload", exitUsage, "Orca's lifecycle input is not valid JSON.")
	}
	u := p.RecipeResult.UserData
	if u.Provider != orcaProvider {
		return nil, newError("orca_payload", exitUsage, "This Orca workspace was not created by the Runzivo recipe.")
	}
	if _, err := sandboxID(u.SandboxID); err != nil {
		return nil, err
	}
	if u.WorkspaceID == "" || strings.ContainsAny(u.WorkspaceID, "/?#\\ ") {
		return nil, newError("orca_payload", exitUsage, "Orca's lifecycle input has no valid workspace.")
	}
	if u.Profile != "" && u.Profile != a.profileName() {
		if err := config.ValidProfileName(u.Profile); err != nil {
			return nil, newError("orca_payload", exitUsage, "Orca's lifecycle input names an invalid rzv profile.")
		}
		if a.Store == nil {
			return nil, newError("no_config", exitFailure, "No configuration directory is available.")
		}
		a.Store = a.Store.ForProfile(u.Profile)
	}
	cmd.options["workspace"] = u.WorkspaceID
	return &p, nil
}

func orcaSuspend(a *app, cmd *command) error {
	p, err := a.orcaPayload(cmd)
	if err != nil {
		return err
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	id := p.RecipeResult.UserData.SandboxID
	s, err := client.SandboxGet(a.ctx, workspace, id)
	if err != nil {
		return apiError(err, "")
	}
	switch s.Status {
	case "stopped", "expired", "deleted", "deleting", "cleanup_pending", "failed":
		a.orcaLog("Sandbox %s is already %s.", id, statusLabel(s.Status))
		return nil
	}
	if _, err := client.SandboxStop(a.ctx, workspace, id); err != nil {
		return apiError(err, "")
	}
	deadline := a.Now().Add(orcaStoppedWait)
	for {
		if s, err = client.SandboxGet(a.ctx, workspace, id); err != nil {
			return apiError(err, "")
		}
		if s.Status == "stopped" {
			break
		}
		if s.Status != "stopping" && s.Status != "running" {
			return newError("sandbox_stop", exitFailure, "Sandbox %s is %s instead of stopped.", id, statusLabel(s.Status))
		}
		if a.Now().After(deadline) {
			return newError("sandbox_stop_timeout", exitFailure, "Sandbox %s did not stop within %s.", id, orcaStoppedWait)
		}
		if err := sleepContext(a.ctx, orcaPollEvery); err != nil {
			return err
		}
	}
	a.orcaLog("Stopped sandbox %s. Its files are kept and compute is not charged; it still expires %s.", id, expiryText(*s))
	return nil
}

func orcaResume(a *app, cmd *command, out io.Writer) error {
	p, err := a.orcaPayload(cmd)
	if err != nil {
		return err
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	id := p.RecipeResult.UserData.SandboxID
	root := p.RecipeResult.Connection.ProjectRoot
	if !strings.HasPrefix(root, orcaWorkspaceRoot+"/") || strings.Contains(root, "..") {
		return newError("orca_payload", exitUsage, "Orca's lifecycle input has no valid project root.")
	}
	s, err := client.SandboxGet(a.ctx, workspace, id)
	if err != nil {
		return apiError(err, "")
	}
	if s.Status == "stopped" || s.Status == "stopping" {
		if s.Status == "stopping" {
			// Start is refused mid-stop, so let the stop finish first.
			deadline := a.Now().Add(orcaStoppedWait)
			for s.Status == "stopping" {
				if a.Now().After(deadline) {
					return newError("sandbox_stop_timeout", exitFailure, "Sandbox %s is still stopping. Wake the workspace again in a minute.", id)
				}
				if err := sleepContext(a.ctx, orcaPollEvery); err != nil {
					return err
				}
				if s, err = client.SandboxGet(a.ctx, workspace, id); err != nil {
					return apiError(err, "")
				}
			}
		}
		a.orcaLog("Starting sandbox %s...", id)
		if _, err := client.SandboxStart(a.ctx, workspace, id); err != nil {
			return apiError(err, "")
		}
	}
	if s, err = a.orcaWaitRunning(client, workspace, id); err != nil {
		return err
	}
	if err := a.orcaWriteSSHConfig(cmd, id, workspace, client); err != nil {
		return err
	}
	if err := a.orcaSSH(id, nil, "test -d "+shellArg(root+"/.git")); err != nil {
		return newError("orca_checkout_missing", exitFailure, "The checkout at %s is missing from sandbox %s. Delete this Orca workspace and create a new one.", root, id)
	}
	a.orcaSummary(s)
	writeJSON(out, a.orcaResultFor(s, workspace, root))
	return nil
}

func orcaDestroy(a *app, cmd *command) error {
	p, err := a.orcaPayload(cmd)
	if err != nil {
		return err
	}
	workspace, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	id := p.RecipeResult.UserData.SandboxID
	if _, err := client.SandboxDelete(a.ctx, workspace, id); err != nil && !isSandboxGone(err) {
		return apiError(err, "")
	}
	a.removeOrcaSSHConfig(id)
	a.orcaLog("Deleted sandbox %s: the virtual machine, its checkout in %s and every other file in it. Work you did not push is gone.", id, orcaWorkspaceRoot)
	return nil
}
