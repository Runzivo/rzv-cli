package cli

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/Runzivo/rzv-cli/internal/config"
	"github.com/Runzivo/rzv-cli/internal/oidc"
	"github.com/Runzivo/rzv-cli/internal/version"
)

func init() {
	register("version", "Show the rzv version", runVersion)
	commandUsage["version"] = "Usage: rzv version [--json]"

	register("login", "Sign in through your browser", runLogin)
	commandUsage["login"] = `Usage: rzv login [-p <profile>] [--env prd|shared] [--workspace <workspace>] [--no-browser] [--json]

Signs in through your browser (GitHub via Runzivo's identity provider) and
stores the access token in your user config directory, readable only by you.
The token is never printed.

Each profile holds one sign-in. -p / --profile picks the profile (default
"default", or $RZV_PROFILE). Signing in makes that profile the active one.

  -p, --profile  Profile to sign in to (lowercase letters, digits, hyphens)
  -w, --workspace  Workspace to select, by id, slug or name; must be one you belong to
  --env          Runzivo environment (default prd, or $RZV_ENV)
  --no-browser   Print the sign-in URL without opening a browser

Advanced overrides: --issuer, --client-id, --audience, --api-base-url,
--github-broker-claim, --allow-insecure-loopback (loopback testing only).`

	register("logout", "Remove the stored sign-in", runLogout)
	commandUsage["logout"] = "Usage: rzv logout [-p <profile>] [--json]"

	register("whoami", "Show who you are signed in as", runWhoami)
	commandUsage["whoami"] = `Usage: rzv whoami [--json]

Shows the signed-in identity, environment, token expiry and selected
workspace, and checks the sign-in against the API.`
}

func runVersion(a *app, cmd *command) error {
	if err := cmd.allow("version"); err != nil {
		return err
	}
	if err := cmd.expectArgs(0, "rzv version [--json]"); err != nil {
		return err
	}
	a.output(cmd, map[string]string{
		"version": version.Version, "commit": version.Commit, "date": version.Date,
		"os": runtime.GOOS, "arch": runtime.GOARCH,
	}, fmt.Sprintf("rzv %s (commit %s, built %s, %s/%s)", version.Version, version.Commit, version.Date, runtime.GOOS, runtime.GOARCH))
	return nil
}

func runLogin(a *app, cmd *command) error {
	if err := cmd.allow("env", "workspace", "issuer", "client-id", "audience", "api-base-url", "github-broker-claim", "allow-insecure-loopback", "no-browser"); err != nil {
		return err
	}
	if err := cmd.expectArgs(0, "rzv login [-p <profile>] [--env prd|shared] [--workspace <workspace>] [--json]"); err != nil {
		return err
	}
	workspace, err := cmd.option("workspace")
	if err != nil {
		return err
	}
	envName, err := cmd.option("env")
	if err != nil {
		return err
	}
	if envName == "" {
		envName = a.Getenv("RZV_ENV")
	}
	env, err := config.LookupEnvironment(envName)
	if err != nil {
		return usageError("%s", err.Error())
	}
	for flag, target := range map[string]*string{
		"issuer": &env.Issuer, "client-id": &env.ClientID, "audience": &env.Audience,
		"api-base-url": &env.APIBaseURL, "github-broker-claim": &env.BrokerClaim,
	} {
		value, err := cmd.option(flag)
		if err != nil {
			return err
		}
		if value != "" {
			*target = value
		}
	}
	insecure := cmd.flags["allow-insecure-loopback"]
	apiURL, err := oidc.NormalizeURL(env.APIBaseURL, "API base URL", insecure)
	if err != nil {
		return err
	}
	opener := a.OpenBrowser
	if cmd.flags["no-browser"] {
		opener = nil
	}
	result, err := oidc.Login(a.ctx, oidc.Options{
		Issuer: env.Issuer, ClientID: env.ClientID, Audience: env.Audience, BrokerClaim: env.BrokerClaim,
		AllowInsecureLoopback: insecure, CallbackPort: a.CallbackPort, Timeout: a.LoginTimeout,
		HTTPClient: a.HTTPClient, OpenBrowser: opener, Now: a.Now,
		ShowURL: func(u string) {
			if opener == nil {
				fmt.Fprintf(a.Stderr, "Open this URL in a browser on this machine to sign in:\n%s\n", u)
				return
			}
			fmt.Fprintf(a.Stderr, "Opening your browser to sign in. If it does not open, visit:\n%s\n", u)
		},
	})
	if err != nil {
		return err
	}
	if a.Store == nil {
		return newError("local_state_unavailable", exitFailure, "No credential store is available.")
	}
	creds := &config.Credentials{
		Version: 1, Environment: env.Name, Identity: identityLabel(result.Claims), Issuer: result.Issuer, ClientID: env.ClientID,
		Audience: env.Audience, GitHubBrokerClaim: env.BrokerClaim,
		APIBaseURL: apiURL.Scheme + "://" + apiURL.Host, AccessToken: result.AccessToken,
	}
	// The workspace is resolved before anything is saved, so a workspace that
	// cannot be found leaves the profile's previous sign-in untouched.
	if workspace != "" {
		client, err := a.client(creds.APIBaseURL, creds.AccessToken)
		if err != nil {
			return err
		}
		memberships, err := client.Memberships(a.ctx)
		if err != nil {
			return apiError(err, "")
		}
		id, err := a.matchWorkspace(memberships, workspace)
		if err != nil {
			return err
		}
		if id == "" {
			return accessError("You are not a member of workspace %s. Nothing was saved; run `rzv login` again with a workspace you belong to.", display(workspace))
		}
		creds.SelectedWorkspace = id
	}
	if err := a.Store.Write(creds); err != nil {
		return err
	}
	profile := a.profileName()
	if err := a.Store.SetActiveProfile(profile); err != nil {
		return err
	}
	var selected any
	text := fmt.Sprintf("Signed in to profile %s (%s) as %s. No workspace is selected; run `rzv workspaces list`.", display(profile), env.Name, display(creds.Identity))
	if creds.SelectedWorkspace != "" {
		selected = creds.SelectedWorkspace
		text = fmt.Sprintf("Signed in to profile %s (%s) as %s. Selected workspace: %s.", display(profile), env.Name, display(creds.Identity), display(creds.SelectedWorkspace))
	}
	a.output(cmd, map[string]any{"authenticated": true, "profile": profile, "environment": env.Name, "selected_workspace_id": selected}, text)
	return nil
}

func runLogout(a *app, cmd *command) error {
	if err := cmd.allow(); err != nil {
		return err
	}
	if err := cmd.expectArgs(0, "rzv logout [--json]"); err != nil {
		return err
	}
	if a.Store != nil {
		if err := a.Store.Clear(); err != nil {
			return err
		}
		active, err := a.Store.ActiveProfile()
		if err != nil {
			return err
		}
		if active == a.profileName() {
			if err := a.Store.ClearActiveProfile(); err != nil {
				return err
			}
		}
	}
	a.output(cmd, map[string]bool{"logged_out": true}, "Logged out.")
	return nil
}

func runWhoami(a *app, cmd *command) error {
	if err := cmd.allow(); err != nil {
		return err
	}
	if err := cmd.expectArgs(0, "rzv whoami [--json]"); err != nil {
		return err
	}
	client, creds, err := a.humanClient()
	if err != nil {
		return err
	}
	claims, _ := oidc.DecodeUnverified(creds.AccessToken)
	memberships, err := client.Memberships(a.ctx)
	if err != nil {
		return apiError(err, "")
	}
	envName := creds.Environment
	if envName == "" {
		envName = "custom"
	}
	expires := claims.Time("exp")
	var selected any
	if creds.SelectedWorkspace != "" {
		selected = creds.SelectedWorkspace
	}
	value := map[string]any{
		"subject":               claims.String("sub"),
		"username":              claims.String("preferred_username"),
		"email":                 claims.String("email"),
		"environment":           envName,
		"api_base_url":          creds.APIBaseURL,
		"issuer":                creds.Issuer,
		"expires_at":            formatTime(expires),
		"workspace_count":       len(memberships),
		"selected_workspace_id": selected,
	}
	who := creds.Identity
	if who == "" {
		who = identityLabel(claims)
	}
	value["identity"] = who
	value["profile"] = a.profileName()
	lines := []string{
		"Signed in as " + display(who) + " (profile " + display(a.profileName()) + ")",
		"Environment: " + display(envName) + " (" + display(creds.APIBaseURL) + ")",
		"Subject: " + display(claims.String("sub")),
		"Session expires: " + formatTime(expires),
		fmt.Sprintf("Workspaces: %d", len(memberships)),
	}
	if creds.SelectedWorkspace != "" {
		lines = append(lines, a.text("Selected workspace: ", "Espace de travail sélectionné : ")+workspaceLabel(memberships, creds.SelectedWorkspace))
	} else {
		lines = append(lines, "Selected workspace: none")
	}
	a.output(cmd, value, strings.Join(lines, "\n"))
	return nil
}

func identityLabel(claims oidc.Claims) string {
	for _, name := range []string{"preferred_username", "email", "name", "sub"} {
		if v := claims.String(name); v != "" {
			return v
		}
	}
	return "unknown"
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.UTC().Format(time.RFC3339)
}
