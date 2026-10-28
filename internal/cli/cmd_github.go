package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/Runzivo/rzv-cli/internal/api"
	"github.com/Runzivo/rzv-cli/internal/githubauth"
)

const githubUsage = `Usage:
  rzv github status [--workspace <id>] [--json]     The linked GitHub App installation
  rzv github installations [--workspace <id>] [--json]
                                                    Every linked installation and its status
  rzv github connect [--workspace <id>] [--json]    Start linking an installation
  rzv github authorize --state <state> --installation-id <id>
      --github-client-id <id> [--github-scope <scope>] [--workspace <id>] [--json]
                                                    Finish the link with GitHub's device flow
  rzv github workspaces start [--json]              Start GitHub-backed workspace onboarding
  rzv github workspaces complete --state <state> --installation-id <id>
      --github-client-id <id> [--github-scope <scope>] [--json]
                                                    Finish onboarding with GitHub's device flow`

const repositoriesUsage = "Usage: rzv repositories list [--workspace <id>] [--json]"

const runnerProfilesUsage = `Usage: rzv runner-profiles <repository-id> [--workspace <id>] [--json]

<repository-id> is GitHub's numeric repository id (see rzv repositories list).
Each available profile's label is the exact runs-on value for a workflow.`

// githubBaseURL is overridden in tests.
var githubBaseURL = githubauth.DefaultBaseURL

func init() {
	register("github", "Show or link GitHub App installations and onboard a workspace", runGitHub)
	commandUsage["github"] = githubUsage
	register("repositories", "List repositories the GitHub App covers", runRepositories)
	commandUsage["repositories"] = repositoriesUsage
	register("runner-profiles", "Show which runner sizes a repository can use", runRunnerProfiles)
	commandUsage["runner-profiles"] = runnerProfilesUsage
}

func runGitHub(a *app, cmd *command) error {
	if len(cmd.args) == 0 {
		return usageError("%s", githubUsage)
	}
	switch cmd.args[0] {
	case "status":
		if err := cmd.allow("workspace", "account"); err != nil {
			return err
		}
		if err := cmd.expectArgs(1, "rzv github status [--workspace <id>] [--json]"); err != nil {
			return err
		}
		id, client, err := a.workspaceClient(cmd)
		if err != nil {
			return err
		}
		inst, err := client.Installation(a.ctx, id)
		if err != nil {
			return apiError(err, "")
		}
		kind := "linked"
		if inst == nil {
			kind = "not_linked"
		}
		a.output(cmd, map[string]any{"workspace_id": id, "kind": kind, "installation": inst}, renderInstallation(id, inst))
		return nil
	case "connect":
		if err := cmd.allow("workspace", "account"); err != nil {
			return err
		}
		if err := cmd.expectArgs(1, "rzv github connect [--workspace <id>] [--json]"); err != nil {
			return err
		}
		id, client, err := a.workspaceClient(cmd)
		if err != nil {
			return err
		}
		start, err := client.StartInstallationLink(a.ctx, id)
		if err != nil {
			return apiError(err, "")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "state": start.State, "expires_at": start.ExpiresAt}, strings.Join([]string{
			"Started linking a GitHub App installation to workspace " + display(id) + ".",
			"State: " + display(start.State),
			"Expires: " + display(start.ExpiresAt),
			"Install the GitHub App, then before it expires run:",
			"  rzv github authorize --state <this state> --installation-id <id from GitHub> --github-client-id <app client id>",
		}, "\n"))
		return nil
	case "installations":
		return githubInstallations(a, cmd)
	case "workspaces":
		return githubWorkspaces(a, cmd)
	case "authorize":
		return githubAuthorize(a, cmd)
	default:
		return usageError("%s", githubUsage)
	}
}

func githubInstallations(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv github installations [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	installations, err := client.Installations(a.ctx, id)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "installations": installations}, renderInstallations(id, installations))
	return nil
}

func renderInstallations(id string, installations []api.Installation) string {
	if len(installations) == 0 {
		return "No GitHub App installations are linked to workspace " + display(id) + "."
	}
	var table bytes.Buffer
	var details strings.Builder
	w := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tSTATE\tACCOUNT\tMANAGE URL\tSETUP\tRETRY\tACCOUNT ARCHIVED\tREPOSITORIES")
	for _, inst := range installations {
		archived := "-"
		if inst.AccountArchived {
			archived = displayOrDash(inst.AccountArchivedAt)
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\n", display(inst.InstallationID), display(inst.State), displayOrDash(inst.AccountName), displayOrDash(inst.ManageURL), displayOrDashString(inst.SetupState), displayOrDash(inst.NextRetryAt), archived, len(inst.Repositories))
		if inst.RunnerPermissionMissing {
			_, _ = fmt.Fprintln(&details, "  "+display(inst.InstallationID)+": runner permission required")
		}
		for _, repository := range inst.Repositories {
			state := "not authorized"
			if repository.Authorized {
				state = "authorized"
			}
			archivedAt := "-"
			if repository.Archived {
				state += ", archived"
				archivedAt = displayOrDash(repository.ArchivedAt)
			}
			_, _ = fmt.Fprintf(&details, "  - %s [%s] (%s; archived: %s)\n", displayOrDash(repository.Name), display(repository.RepositoryID), state, archivedAt)
		}
	}
	_ = w.Flush()
	return "GitHub installations for workspace " + display(id) + ":\n" + strings.TrimSuffix(table.String()+details.String(), "\n")
}

func displayOrDashString(value string) string {
	if value == "" {
		return "-"
	}
	return display(value)
}

func githubWorkspaces(a *app, cmd *command) error {
	if len(cmd.args) < 2 {
		return usageError("%s", githubUsage)
	}
	switch cmd.args[1] {
	case "start":
		return githubWorkspaceStart(a, cmd)
	case "complete":
		return githubWorkspaceComplete(a, cmd)
	default:
		return usageError("%s", githubUsage)
	}
}

func githubWorkspaceStart(a *app, cmd *command) error {
	if err := cmd.allow(); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv github workspaces start [--json]"); err != nil {
		return err
	}
	client, _, err := a.humanClient()
	if err != nil {
		return err
	}
	start, err := client.StartWorkspaceConnection(a.ctx)
	if err != nil {
		return githubWorkspaceError(err)
	}
	a.output(cmd, map[string]string{"state": start.State, "expires_at": start.ExpiresAt}, strings.Join([]string{
		"Started GitHub-backed workspace onboarding.",
		"State: " + display(start.State),
		"Expires: " + display(start.ExpiresAt),
		"Install the GitHub App, then before it expires run:",
		"  rzv github workspaces complete --state <this state> --installation-id <id from GitHub> --github-client-id <app client id>",
	}, "\n"))
	return nil
}

func githubWorkspaceComplete(a *app, cmd *command) error {
	if err := cmd.allow("state", "installation-id", "github-client-id", "github-scope"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv github workspaces complete --state <state> --installation-id <id> --github-client-id <id>"); err != nil {
		return err
	}
	if err := a.requireHuman(); err != nil {
		return err
	}
	state, err := cmd.required("state")
	if err != nil {
		return err
	}
	rawInstallation, err := cmd.required("installation-id")
	if err != nil {
		return err
	}
	installationID, err := strconv.ParseInt(rawInstallation, 10, 64)
	if err != nil || installationID < 1 {
		return usageError("--installation-id must be the GitHub installation's numeric id.")
	}
	clientID, err := cmd.required("github-client-id")
	if err != nil {
		return err
	}
	scope, err := cmd.option("github-scope")
	if err != nil {
		return err
	}
	client, creds, err := a.humanClient()
	if err != nil {
		return err
	}
	token, err := githubDeviceToken(a, clientID, scope)
	if err != nil {
		return err
	}
	connection, err := client.CompleteWorkspaceConnection(a.ctx, api.CompleteWorkspaceConnectionInput{State: state, InstallationID: installationID, UserAccessToken: token})
	if err != nil {
		return githubWorkspaceError(err)
	}
	if connection.Workspace != nil {
		creds.SelectedWorkspace = connection.Workspace.TenantID
		if err := a.Store.Write(creds); err != nil {
			return err
		}
	}
	a.output(cmd, connection, renderWorkspaceConnection(connection))
	return nil
}

func githubDeviceToken(a *app, clientID, scope string) (string, error) {
	token, err := githubauth.Authorize(a.ctx, githubauth.Options{
		ClientID: clientID, Scope: scope, BaseURL: githubBaseURL, HTTPClient: a.HTTPClient,
		OpenBrowser: a.OpenBrowser,
		Prompt: func(uri, code string) {
			fmt.Fprintf(a.Stderr, "Go to %s and enter this code: %s\n", display(uri), display(code))
		},
	})
	if err == nil {
		return token, nil
	}
	var ghErr *githubauth.Error
	if errors.As(err, &ghErr) && (ghErr.Code == githubauth.CodeDenied || ghErr.Code == githubauth.CodeExpired) {
		return "", accessError("%s", ghErr.Message)
	}
	return "", unavailableError("%s", err.Error())
}

func renderWorkspaceConnection(connection *api.WorkspaceConnection) string {
	lines := []string{
		"GitHub account: " + display(connection.AccountLogin) + " (" + display(connection.AccountType) + ")",
		"Outcome: " + display(connection.Outcome),
	}
	if connection.Workspace != nil {
		lines = append(lines, "Workspace: "+display(connection.Workspace.TenantName)+" ["+display(connection.Workspace.TenantID)+"] (selected)")
	} else {
		lines = append(lines, "Workspace access is required before this workspace can be selected.")
	}
	if connection.Notice != "" {
		lines = append(lines, "Notice: "+display(connection.Notice))
	}
	return strings.Join(lines, "\n")
}

func githubWorkspaceError(err error) error {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	switch {
	case apiErr.Status == 403 && apiErr.Code == "signups_closed":
		return newError(apiErr.Code, exitAccessDenied, "Self-service sign-up is closed. Existing customers can still sign in and use their workspaces.")
	case apiErr.Status == 403 && apiErr.Code == "captcha_verification_failed":
		return accessError("Workspace sign-up needs a CAPTCHA check. Complete workspace setup in the Runzivo portal.")
	case apiErr.Status == 503 && apiErr.Code == "captcha_verification_unavailable":
		return unavailableError("Workspace protection is temporarily unavailable. Try again shortly.")
	case apiErr.Status == 503 && apiErr.Code == "installation_authority_unverified":
		return unavailableError("GitHub's authority check could not complete. Try again.")
	case apiErr.Status == 403 && apiErr.Code == "installation_authority_denied":
		return accessError("GitHub denied authority over this installation: you must be an organization owner or the account owner.")
	case apiErr.Status == 409 && apiErr.Code == "link_state_invalid":
		return conflictError("The workspace link state is unknown, expired or already used. Run `rzv github workspaces start` again.")
	case apiErr.Status == 409 && apiErr.Code == "github_workspace_connection_conflict":
		return conflictError("This GitHub account or installation is already connected to a different workspace.")
	case apiErr.Status == 409 && apiErr.Code == "github_account_archived":
		return conflictError("GitHub reports this organization as archived, so a new workspace cannot be created.")
	}
	return apiError(err, "")
}

func githubAuthorize(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "state", "installation-id", "github-client-id", "github-scope"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv github authorize --state <state> --installation-id <id> --github-client-id <id>"); err != nil {
		return err
	}
	if err := a.requireHuman(); err != nil {
		return err
	}
	state, err := cmd.required("state")
	if err != nil {
		return err
	}
	rawInstallation, err := cmd.required("installation-id")
	if err != nil {
		return err
	}
	installationID, err := strconv.ParseInt(rawInstallation, 10, 64)
	if err != nil || installationID < 1 {
		return usageError("--installation-id must be the GitHub installation's numeric id.")
	}
	clientID, err := cmd.required("github-client-id")
	if err != nil {
		return err
	}
	scope, err := cmd.option("github-scope")
	if err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	token, err := githubauth.Authorize(a.ctx, githubauth.Options{
		ClientID: clientID, Scope: scope, BaseURL: githubBaseURL, HTTPClient: a.HTTPClient,
		OpenBrowser: a.OpenBrowser,
		Prompt: func(uri, code string) {
			fmt.Fprintf(a.Stderr, "Go to %s and enter this code: %s\n", display(uri), display(code))
		},
	})
	if err != nil {
		var ghErr *githubauth.Error
		if errors.As(err, &ghErr) && (ghErr.Code == githubauth.CodeDenied || ghErr.Code == githubauth.CodeExpired) {
			return accessError("%s", ghErr.Message)
		}
		return unavailableError("%s", err.Error())
	}
	inst, outcome, err := client.CompleteInstallationLink(a.ctx, id, api.CompleteLinkInput{State: state, InstallationID: installationID, UserAccessToken: token})
	if err != nil {
		return apiError(err, "")
	}
	switch outcome {
	case api.LinkAuthorityDenied:
		return accessError("GitHub denied authority over this installation: you must be an organization owner or the account owner.")
	case api.LinkStateInvalid:
		return conflictError("The link state is unknown, expired or already used, or the installation is linked elsewhere. Run `rzv github connect` again.")
	case api.LinkAuthorityUnverified:
		return unavailableError("GitHub's authority check could not complete. Try again.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "kind": "linked", "installation": inst}, renderInstallation(id, inst))
	return nil
}

func renderInstallation(id string, inst *api.Installation) string {
	if inst == nil {
		return "No GitHub App installation is linked to workspace " + display(id) + "."
	}
	lines := []string{
		"GitHub installation for workspace " + display(id) + ":",
		"State: " + display(inst.State),
		"Account: " + displayOrDash(inst.AccountName),
		"Manage URL: " + displayOrDash(inst.ManageURL),
	}
	return strings.Join(append(lines, renderRepositoryLines(inst.Repositories)...), "\n")
}

func renderRepositoryLines(repos []api.Repository) []string {
	if len(repos) == 0 {
		return []string{"Repositories: none"}
	}
	lines := []string{"Repositories:"}
	for _, r := range repos {
		auth := "not authorized"
		if r.Authorized {
			auth = "authorized"
		}
		lines = append(lines, fmt.Sprintf("- %s [%s] (%s)", displayOrDash(r.Name), display(r.RepositoryID), auth))
	}
	return lines
}

func runRepositories(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if len(cmd.args) != 1 || cmd.args[0] != "list" {
		return usageError("%s", repositoriesUsage)
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	installations, err := client.Installations(a.ctx, id)
	if err != nil {
		return apiError(err, "")
	}
	type repoRow struct {
		api.Repository
		InstallationID string  `json:"installation_id"`
		AccountName    *string `json:"account_name"`
	}
	rows := []repoRow{}
	for _, inst := range installations {
		for _, r := range inst.Repositories {
			rows = append(rows, repoRow{Repository: r, InstallationID: inst.InstallationID, AccountName: inst.AccountName})
		}
	}
	text := "No repositories are linked to workspace " + display(id) + ". Run `rzv github status`."
	if len(rows) > 0 {
		lines := []string{"Repositories for workspace " + display(id) + ":"}
		for _, inst := range installations {
			lines = append(lines, renderRepositoryLines(inst.Repositories)[1:]...)
		}
		text = strings.Join(lines, "\n")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "repositories": rows}, text)
	return nil
}

func runRunnerProfiles(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv runner-profiles <repository-id> [--workspace <id>] [--json]"); err != nil {
		return err
	}
	repoID, err := strconv.ParseInt(cmd.args[0], 10, 64)
	if err != nil || repoID < 1 {
		return usageError("<repository-id> must be GitHub's numeric repository id, not its name. See `rzv repositories list`.")
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	profiles, err := client.RunnerProfiles(a.ctx, id, repoID)
	if err != nil {
		return apiError(err, "")
	}
	if profiles == nil {
		a.output(cmd, map[string]any{"workspace_id": id, "repository_id": repoID, "kind": "not_authorized"},
			fmt.Sprintf("Repository %d is not authorized for workspace %s.", repoID, display(id)))
		return nil
	}
	a.output(cmd, map[string]any{"workspace_id": id, "repository_id": repoID, "kind": "available", "runner_profiles": profiles}, renderProfiles(id, repoID, profiles))
	return nil
}

func renderProfiles(id string, repoID int64, p *api.RunnerProfiles) string {
	if len(p.Profiles) == 0 {
		return fmt.Sprintf("No published runner profiles for repository %d.", repoID)
	}
	lines := []string{
		fmt.Sprintf("Runner profiles for repository %d (workspace %s):", repoID, display(id)),
		"Label format: " + display(p.WorkflowLabelFormat),
	}
	for _, prof := range p.Profiles {
		status := "available"
		if !prof.Available {
			reason := "unavailable"
			if prof.Reason != nil {
				reason = *prof.Reason
			}
			status = "blocked (" + display(reason) + ")"
		}
		lines = append(lines, fmt.Sprintf("- %s: %d vCPU, %d MiB, runs-on: %s", status, prof.VCPUCount, prof.MemSizeMiB, display(prof.Label)))
	}
	return strings.Join(lines, "\n")
}
