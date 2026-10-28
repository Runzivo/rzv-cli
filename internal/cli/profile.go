package cli

import (
	"strings"

	"github.com/Runzivo/rzv-cli/internal/config"
)

// selectProfile points the app's store at the profile this run uses. The
// order is -p / --profile, then RZV_PROFILE, then the active profile chosen
// with `rzv profiles use` (or `rzv login`), then the default profile.
func (a *app) selectProfile(cmd *command) error {
	// `profiles` manages every profile, so a damaged active-profile file
	// must not stop it from repairing that file.
	if a.Store == nil || cmd.name == "profiles" {
		return nil
	}
	name := ""
	if cmd.hasProfile {
		name = strings.TrimSpace(cmd.profile)
	} else if env := strings.TrimSpace(a.Getenv("RZV_PROFILE")); env != "" {
		name = env
	}
	if name == "" {
		active, err := a.Store.ActiveProfile()
		if err != nil {
			return err
		}
		name = active
	}
	if name == "" {
		name = config.DefaultProfile
	}
	if err := config.ValidProfileName(name); err != nil {
		return usageError("%s", err.Error())
	}
	a.Store = a.Store.ForProfile(name)
	return nil
}

// profileName is the profile this run uses, after selectProfile.
func (a *app) profileName() string {
	if a.Store == nil || a.Store.Profile == "" {
		return config.DefaultProfile
	}
	return a.Store.Profile
}
