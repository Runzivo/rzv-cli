package cli

import (
	"fmt"
	"strings"

	"github.com/Runzivo/rzv-cli/internal/config"
)

const profilesUsage = `Usage: rzv profiles list [--json]
       rzv profiles use <profile> [--json]
       rzv profiles remove <profile> [--json]

A profile is one sign-in on this machine. Sign in to another profile with
` + "`rzv login -p <profile>`" + `. Use -p <profile> on any command, or set RZV_PROFILE, to
use a profile for one run. Removing a profile only deletes its local
credentials; it does not revoke the session.`

func init() {
	register("profiles", "List, switch or remove local sign-in profiles", runProfiles)
	commandUsage["profiles"] = profilesUsage
}

func runProfiles(a *app, cmd *command) error {
	if err := cmd.allow(); err != nil {
		return err
	}
	if len(cmd.args) == 0 {
		return usageError("Use `rzv profiles list`, `rzv profiles use <profile>` or `rzv profiles remove <profile>`. Run `rzv help profiles`.")
	}
	switch cmd.args[0] {
	case "list":
		return profilesList(a, cmd)
	case "use":
		return profilesUse(a, cmd)
	case "remove":
		return profilesRemove(a, cmd)
	default:
		return usageError("Use `rzv profiles list`, `rzv profiles use <profile>` or `rzv profiles remove <profile>`. Run `rzv help profiles`.")
	}
}

// profileSummary is one profile as list shows it. It never carries the
// access token.
type profileSummary struct {
	Name        string  `json:"name"`
	Active      bool    `json:"active"`
	State       string  `json:"state"`
	Environment string  `json:"environment,omitempty"`
	Identity    string  `json:"identity,omitempty"`
	APIBaseURL  string  `json:"api_base_url,omitempty"`
	Workspace   *string `json:"selected_workspace_id"`
}

func profilesList(a *app, cmd *command) error {
	if err := cmd.expectArgs(1, "rzv profiles list [--json]"); err != nil {
		return err
	}
	active, err := a.Store.ActiveProfile()
	if err != nil {
		return err
	}
	names, err := a.Store.ListProfiles()
	if err != nil {
		return err
	}
	summaries := make([]profileSummary, 0, len(names))
	for _, name := range names {
		summary := profileSummary{Name: name, Active: name == active, State: "signed_in"}
		creds, err := a.Store.ForProfile(name).Read()
		if err != nil {
			summary.State = "invalid"
		} else if creds != nil {
			summary.Environment = creds.Environment
			summary.Identity = creds.Identity
			summary.APIBaseURL = creds.APIBaseURL
			if creds.SelectedWorkspace != "" {
				id := creds.SelectedWorkspace
				summary.Workspace = &id
			}
		}
		summaries = append(summaries, summary)
	}
	var activeValue any
	if active != "" {
		activeValue = active
	}
	lines := []string{}
	for _, s := range summaries {
		marker := "  "
		if s.Active {
			marker = "* "
		}
		line := marker + display(s.Name)
		if s.State == "invalid" {
			line += " (invalid: run `rzv login -p " + display(s.Name) + "`)"
		} else {
			workspace := "no workspace"
			if s.Workspace != nil {
				workspace = "workspace " + display(*s.Workspace)
			}
			line += fmt.Sprintf(" %s %s, %s", display(s.Environment), display(s.Identity), workspace)
		}
		lines = append(lines, line)
	}
	text := "No profiles. Sign in with `rzv login`."
	if len(lines) > 0 {
		text = strings.Join(lines, "\n")
	}
	a.output(cmd, map[string]any{"active_profile": activeValue, "profiles": summaries}, text)
	return nil
}

func profilesUse(a *app, cmd *command) error {
	if err := cmd.expectArgs(2, "rzv profiles use <profile> [--json]"); err != nil {
		return err
	}
	name := cmd.args[1]
	if err := config.ValidProfileName(name); err != nil {
		return usageError("%s", err.Error())
	}
	creds, err := a.Store.ForProfile(name).Read()
	if err != nil {
		return err
	}
	if creds == nil {
		return notFoundError("No sign-in for profile %s. Run `rzv login -p %s`.", display(name), display(name))
	}
	if err := a.Store.SetActiveProfile(name); err != nil {
		return err
	}
	a.output(cmd, map[string]string{"active_profile": name}, "Active profile: "+display(name))
	return nil
}

func profilesRemove(a *app, cmd *command) error {
	if err := cmd.expectArgs(2, "rzv profiles remove <profile> [--json]"); err != nil {
		return err
	}
	name := cmd.args[1]
	if err := config.ValidProfileName(name); err != nil {
		return usageError("%s", err.Error())
	}
	store := a.Store.ForProfile(name)
	creds, err := store.Read()
	if err != nil {
		return err
	}
	if creds == nil {
		return notFoundError("No sign-in for profile %s.", display(name))
	}
	if err := store.Clear(); err != nil {
		return err
	}
	active, err := a.Store.ActiveProfile()
	if err != nil {
		return err
	}
	if active == name {
		if err := a.Store.ClearActiveProfile(); err != nil {
			return err
		}
	}
	a.output(cmd, map[string]string{"removed_profile": name},
		"Removed profile "+display(name)+" from this machine. Its session is not revoked.")
	return nil
}
