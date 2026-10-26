package cli

import (
	"regexp"
	"strings"

	"github.com/Runzivo/rzv-cli/internal/api"
	"github.com/Runzivo/rzv-cli/internal/config"
)

const gitlabUsage = `Usage:
  rzv gitlab connections [--workspace <id>] [--json]     GitLab connections for the workspace
  rzv gitlab disconnect <connection-id> [--workspace <id>] [--json]
                                                          Revoke and erase a connection
  rzv gitlab connect [--open]                             Print the portal page that connects GitLab

GitLab is connected in the portal. This command never handles GitLab
credentials. Connecting from the terminal is not supported yet.`

// gitlabConnectionIDPattern matches the server's connection id format.
var gitlabConnectionIDPattern = regexp.MustCompile(`^glc_[0-9a-f]{24}$`)

// gitlabIntegrationsPath is the settings tab that holds the GitLab panel.
const gitlabIntegrationsPath = "/settings?tab=integrations"

func init() {
	register("gitlab", "List or disconnect GitLab connections, or open the connect page", runGitLab)
	commandUsage["gitlab"] = gitlabUsage
}

func runGitLab(a *app, cmd *command) error {
	if len(cmd.args) == 0 {
		return usageError("%s", gitlabUsage)
	}
	switch cmd.args[0] {
	case "connections":
		if err := cmd.allow("workspace", "account"); err != nil {
			return err
		}
		if err := cmd.expectArgs(1, "rzv gitlab connections [--workspace <id>] [--json]"); err != nil {
			return err
		}
		id, client, err := a.workspaceClient(cmd)
		if err != nil {
			return err
		}
		connections, err := client.GitLabConnections(a.ctx, id)
		if err != nil {
			return apiError(err, "")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "connections": connections}, renderGitLabConnections(id, connections))
		return nil
	case "disconnect":
		if err := cmd.allow("workspace", "account"); err != nil {
			return err
		}
		if err := cmd.expectArgs(2, "rzv gitlab disconnect <connection-id> [--workspace <id>] [--json]"); err != nil {
			return err
		}
		connectionID := strings.TrimSpace(cmd.args[1])
		if !gitlabConnectionIDPattern.MatchString(connectionID) {
			return usageError("connection id must look like glc_ followed by 24 lowercase hex characters (see rzv gitlab connections).")
		}
		id, client, err := a.workspaceClient(cmd)
		if err != nil {
			return err
		}
		if err := client.DisconnectGitLabConnection(a.ctx, id, connectionID); err != nil {
			return apiError(err, "")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "connection_id": connectionID, "disconnected": true}, "Disconnected GitLab connection "+display(connectionID)+" from workspace "+display(id)+".")
		return nil
	case "connect":
		if err := cmd.allow("open"); err != nil {
			return err
		}
		if err := cmd.expectArgs(1, "rzv gitlab connect [--open]"); err != nil {
			return err
		}
		env, err := config.LookupEnvironment(a.Getenv("RZV_ENV"))
		if err != nil {
			return usageError("%s", err.Error())
		}
		target := env.PortalURL + gitlabIntegrationsPath
		open := cmd.flags["open"]
		if open && a.OpenBrowser != nil {
			if err := a.OpenBrowser(target); err != nil {
				return unavailableError("unable to open the browser; open this page yourself: %s", target)
			}
		}
		a.output(cmd, map[string]any{"url": target, "opened": open}, "Connect GitLab in the portal: "+target)
		return nil
	}
	return usageError("%s", gitlabUsage)
}

func renderGitLabConnections(id string, connections []api.GitLabConnection) string {
	if len(connections) == 0 {
		return "No GitLab connections for workspace " + display(id) + "."
	}
	lines := []string{"GitLab connections for workspace " + display(id) + ":"}
	for _, c := range connections {
		reconnect := ""
		if c.ReconnectRequired {
			reconnect = " (reconnect required)"
		}
		lines = append(lines, "  "+display(c.ConnectionID)+"  "+display(c.GitLabUsername)+"  "+display(c.Status)+reconnect)
	}
	return strings.Join(lines, "\n")
}
