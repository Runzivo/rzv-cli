package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// DefaultProfile is the profile that keeps credentials.json. Commands use it
// when no profile is named.
const DefaultProfile = "default"

var profileName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// ValidProfileName checks a profile name: lowercase letters, digits and
// hyphens, starting with a letter or digit, at most 32 characters.
func ValidProfileName(name string) error {
	if !profileName.MatchString(name) {
		return &StoreError{"invalid_profile", "Profile names use lowercase letters, digits and hyphens (1 to 32 characters)."}
	}
	return nil
}

type activeProfile struct {
	Version int    `json:"version"`
	Profile string `json:"profile"`
}

// ActiveProfile returns the profile that commands use when none is named, or
// "" when none has been chosen.
func (s *Store) ActiveProfile() (string, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir, activeFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", &StoreError{"local_state_unavailable", "Unable to read the active profile."}
	}
	var active activeProfile
	if err := json.Unmarshal(data, &active); err != nil || active.Version != 1 || ValidProfileName(active.Profile) != nil {
		return "", &StoreError{"local_state_invalid", "The active profile is invalid. Run `rzv profiles use <name>`."}
	}
	return active.Profile, nil
}

// SetActiveProfile makes name the profile that commands use by default.
func (s *Store) SetActiveProfile(name string) error {
	data, err := json.Marshal(activeProfile{Version: 1, Profile: name})
	if err != nil {
		return &StoreError{"local_state_unavailable", "Unable to save the active profile."}
	}
	return writeAtomic(filepath.Join(s.Dir, activeFile), data)
}

// ClearActiveProfile forgets the active profile. A missing file is not an
// error.
func (s *Store) ClearActiveProfile() error {
	err := os.Remove(filepath.Join(s.Dir, activeFile))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return &StoreError{"local_state_unavailable", "Unable to clear the active profile."}
	}
	return nil
}

// ListProfiles returns the names that have a credentials file, sorted. The
// default profile is listed as "default".
func (s *Store) ListProfiles() ([]string, error) {
	var names []string
	if _, err := os.Lstat(filepath.Join(s.Dir, credentialsFile)); err == nil {
		names = append(names, DefaultProfile)
	}
	entries, err := os.ReadDir(filepath.Join(s.Dir, profilesDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, &StoreError{"local_state_unavailable", "Unable to list local rzv profiles."}
	}
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), ".json")
		if ok && name != DefaultProfile && entry.Type().IsRegular() && ValidProfileName(name) == nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}
