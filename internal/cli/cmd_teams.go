package cli

import (
	"fmt"
	"strings"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const teamsUsage = `Usage:
  rzv teams list [--workspace <id>] [--json]
  rzv teams show <team-id> [--workspace <id>] [--json]
  rzv teams create --name <name> --permissions <permission,...>
      [--description <text>] [--workspace <id>] [--json]
  rzv teams rename <team-id> [--name <name>] [--description <text>] [--workspace <id>] [--json]
  rzv teams permissions <team-id> --permissions <permission,...> [--workspace <id>] [--json]
  rzv teams add-member <team-id> <identity-id> [--workspace <id>] [--json]
  rzv teams remove-member <team-id> <identity-id> [--workspace <id>] [--json]
  rzv teams delete <team-id> [--workspace <id>] [--json]

Changing teams needs the Manage members permission, and you can only grant
permissions you hold yourself. System teams (Owners, Admins, Members, Viewers)
cannot be renamed or deleted. Run rzv permissions to see the permission names.`

const permissionsUsage = `Usage:
  rzv permissions [--workspace <id>] [--json]

Shows your permissions in the workspace and every permission that can be
granted to a team.`

func init() {
	register("teams", "List, show or change workspace teams", runTeams)
	commandUsage["teams"] = teamsUsage
	register("permissions", "Show your permissions and the permission catalogue", runPermissions)
	commandUsage["permissions"] = permissionsUsage
}

func runTeams(a *app, cmd *command) error {
	if len(cmd.args) == 0 {
		return usageError("%s", teamsUsage)
	}
	switch cmd.args[0] {
	case "list":
		return teamsList(a, cmd)
	case "show":
		return teamsShow(a, cmd)
	case "create":
		return teamsCreate(a, cmd)
	case "rename":
		return teamsRename(a, cmd)
	case "permissions":
		return teamsPermissions(a, cmd)
	case "add-member":
		return teamsMember(a, cmd, true)
	case "remove-member":
		return teamsMember(a, cmd, false)
	case "delete":
		return teamsDelete(a, cmd)
	default:
		return usageError("%s", teamsUsage)
	}
}

const teamNotFound = "The team was not found in this workspace."

func teamsList(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv teams list [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	teams, err := client.Teams(a.ctx, id)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "teams": teams}, renderTeams(id, teams))
	return nil
}

func renderTeams(id string, teams []api.Team) string {
	if len(teams) == 0 {
		return "No teams in workspace " + display(id) + "."
	}
	lines := []string{"Teams in workspace " + display(id) + ":"}
	for _, t := range teams {
		kind := ""
		if t.Locked {
			kind = ", system team"
		}
		lines = append(lines, fmt.Sprintf("- %s [%s] %d %s%s", display(t.Name), display(t.TeamID), t.MemberCount, plural(t.MemberCount, "member", "members"), kind))
	}
	return strings.Join(lines, "\n")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func teamsShow(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv teams show <team-id>"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	team, err := client.Team(a.ctx, id, cmd.args[1])
	if err != nil {
		return apiError(err, teamNotFound)
	}
	a.output(cmd, map[string]any{"workspace_id": id, "team": team}, renderTeam(team))
	return nil
}

func renderTeam(t *api.Team) string {
	lines := []string{fmt.Sprintf("%s [%s]", display(t.Name), display(t.TeamID))}
	if t.Description != "" {
		lines = append(lines, "Description: "+display(t.Description))
	}
	if t.Locked {
		lines = append(lines, "System team: cannot be renamed, deleted or given other permissions.")
	}
	lines = append(lines, "Permissions: "+displayList(t.Permissions))
	lines = append(lines, fmt.Sprintf("Members (%d):", t.MemberCount))
	for _, m := range t.Members {
		email := "no email"
		if m.Email != nil {
			email = display(*m.Email)
		}
		lines = append(lines, fmt.Sprintf("- %s (%s)", display(m.IdentityID), email))
	}
	return strings.Join(lines, "\n")
}

func displayList(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	shown := make([]string, len(values))
	for i, v := range values {
		shown[i] = display(v)
	}
	return strings.Join(shown, ", ")
}

// parsePermissions reads --permissions: a comma-separated list without
// repeats. An empty list is allowed (a team with no permissions); the API
// checks the names.
func parsePermissions(cmd *command) ([]string, error) {
	raw, err := cmd.required("permissions")
	if err != nil {
		return nil, err
	}
	grants := []string{}
	seen := map[string]bool{}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if seen[p] {
			return nil, usageError("--permissions lists %s more than once.", display(p))
		}
		seen[p] = true
		grants = append(grants, p)
	}
	return grants, nil
}

func teamsCreate(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "name", "description", "permissions"); err != nil {
		return err
	}
	if err := cmd.expectArgs(1, "rzv teams create --name <name> --permissions <permission,...>"); err != nil {
		return err
	}
	name, err := cmd.required("name")
	if err != nil {
		return err
	}
	grants, err := parsePermissions(cmd)
	if err != nil {
		return err
	}
	description, err := cmd.option("description")
	if err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	team, err := client.CreateTeam(a.ctx, id, api.CreateTeamInput{Name: name, Description: description, Permissions: grants})
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "team": team}, fmt.Sprintf("Created team %s [%s].", display(team.Name), display(team.TeamID)))
	return nil
}

func teamsRename(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "name", "description"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv teams rename <team-id> [--name <name>] [--description <text>]"); err != nil {
		return err
	}
	name, err := cmd.option("name")
	if err != nil {
		return err
	}
	description, err := cmd.option("description")
	if err != nil {
		return err
	}
	var namePtr, descriptionPtr *string
	if _, ok := cmd.options["name"]; ok {
		namePtr = &name
	}
	if _, ok := cmd.options["description"]; ok {
		descriptionPtr = &description
	}
	if namePtr == nil && descriptionPtr == nil {
		return usageError("rzv teams rename needs --name, --description or both.")
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	team, err := client.UpdateTeam(a.ctx, id, cmd.args[1], namePtr, descriptionPtr)
	if err != nil {
		return apiError(err, teamNotFound)
	}
	a.output(cmd, map[string]any{"workspace_id": id, "team": team}, fmt.Sprintf("Updated team %s [%s].", display(team.Name), display(team.TeamID)))
	return nil
}

func teamsPermissions(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account", "permissions"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv teams permissions <team-id> --permissions <permission,...>"); err != nil {
		return err
	}
	grants, err := parsePermissions(cmd)
	if err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	team, err := client.SetTeamGrants(a.ctx, id, cmd.args[1], grants)
	if err != nil {
		return apiError(err, teamNotFound)
	}
	a.output(cmd, map[string]any{"workspace_id": id, "team": team}, fmt.Sprintf("Team %s now has: %s.", display(team.Name), displayList(team.Permissions)))
	return nil
}

func teamsMember(a *app, cmd *command, add bool) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	verb := "remove-member"
	if add {
		verb = "add-member"
	}
	if err := cmd.expectArgs(3, "rzv teams "+verb+" <team-id> <identity-id>"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	teamID, identityID := cmd.args[1], cmd.args[2]
	if add {
		team, err := client.AddTeamMember(a.ctx, id, teamID, identityID)
		if err != nil {
			return apiError(err, "The team or the member was not found in this workspace.")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "team": team}, fmt.Sprintf("Added %s to team %s.", display(identityID), display(team.Name)))
		return nil
	}
	if err := client.RemoveTeamMember(a.ctx, id, teamID, identityID); err != nil {
		return apiError(err, "The team or the member was not found in this workspace.")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "team_id": teamID, "removed_identity_id": identityID}, fmt.Sprintf("Removed %s from team %s.", display(identityID), display(teamID)))
	return nil
}

func teamsDelete(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(2, "rzv teams delete <team-id>"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	teamID := cmd.args[1]
	if err := client.DeleteTeam(a.ctx, id, teamID); err != nil {
		return apiError(err, teamNotFound)
	}
	a.output(cmd, map[string]any{"workspace_id": id, "deleted_team_id": teamID}, "Deleted team "+display(teamID)+".")
	return nil
}

func runPermissions(a *app, cmd *command) error {
	if err := cmd.allow("workspace", "account"); err != nil {
		return err
	}
	if err := cmd.expectArgs(0, "rzv permissions [--workspace <id>] [--json]"); err != nil {
		return err
	}
	id, client, err := a.workspaceClient(cmd)
	if err != nil {
		return err
	}
	permissions, err := client.Permissions(a.ctx, id)
	if err != nil {
		return apiError(err, "")
	}
	a.output(cmd, map[string]any{"workspace_id": id, "permissions": permissions.Permissions, "catalogue": permissions.Catalogue, "presets": permissions.Presets}, renderPermissions(id, permissions))
	return nil
}

func renderPermissions(id string, p *api.Permissions) string {
	held := map[string]bool{}
	for _, name := range p.Permissions {
		held[name] = true
	}
	lines := []string{"Your permissions in workspace " + display(id) + ": " + displayList(p.Permissions), "", "All permissions:"}
	for _, d := range p.Catalogue {
		mark := " "
		if held[d.Name] {
			mark = "*"
		}
		lines = append(lines, fmt.Sprintf("%s %s: %s", mark, display(d.Name), display(d.Description)))
	}
	lines = append(lines, "", "* you hold it.")
	return strings.Join(lines, "\n")
}
