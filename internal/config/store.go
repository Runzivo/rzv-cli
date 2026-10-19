package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const credentialsFile = "credentials.json"

// Credentials is what `rzv login` stores. The access token is never
// printed or logged.
type Credentials struct {
	Version     int    `json:"version"`
	Environment string `json:"environment,omitempty"`
	// Identity is a display label (username or email) from the ID token.
	Identity          string `json:"identity,omitempty"`
	Issuer            string `json:"issuer"`
	ClientID          string `json:"clientId"`
	Audience          string `json:"audience"`
	GitHubBrokerClaim string `json:"githubBrokerClaim"`
	APIBaseURL        string `json:"apiBaseUrl"`
	AccessToken       string `json:"accessToken"`
	SelectedWorkspace string `json:"selectedAccountId,omitempty"`
}

func (c *Credentials) valid() bool {
	return c.Version == 1 && c.Issuer != "" && c.ClientID != "" && c.Audience != "" &&
		c.APIBaseURL != "" && c.AccessToken != ""
}

// StoreError is a local credential problem. Its message never includes the
// file's contents.
type StoreError struct {
	Code    string
	Message string
}

func (e *StoreError) Error() string     { return e.Message }
func (e *StoreError) ErrorCode() string { return e.Code }
func (e *StoreError) ExitCode() int     { return 1 }

// Store reads and writes the credentials file in one directory.
type Store struct {
	Dir string
}

// Directory returns where rzv keeps its credentials: $RZV_CONFIG_DIR when
// set, otherwise the OS user config directory plus "runzivo"
// (~/.config/runzivo on Linux, ~/Library/Application Support/runzivo on
// macOS, %AppData%\runzivo on Windows).
func Directory(getenv func(string) string) (string, error) {
	for _, key := range []string{"RZV_CONFIG_DIR", "RUNZIVO_CLI_CONFIG_DIR"} {
		if value := strings.TrimSpace(getenv(key)); value != "" {
			return value, nil
		}
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", &StoreError{"local_state_unavailable", "Unable to find a configuration directory. Set RZV_CONFIG_DIR."}
	}
	return filepath.Join(base, "runzivo"), nil
}

func (s *Store) path() string { return filepath.Join(s.Dir, credentialsFile) }

// Read returns the stored credentials, or nil when there are none.
func (s *Store) Read() (*Credentials, error) {
	info, err := os.Lstat(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, &StoreError{"local_state_unavailable", "Unable to read local rzv credentials."}
	}
	if !info.Mode().IsRegular() {
		return nil, &StoreError{"local_state_invalid", "Local rzv credentials are not a regular file."}
	}
	data, err := os.ReadFile(s.path())
	if err != nil {
		return nil, &StoreError{"local_state_unavailable", "Unable to read local rzv credentials."}
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil || !creds.valid() {
		return nil, &StoreError{"local_state_invalid", "Local rzv credentials are invalid. Run `rzv logout` and sign in again."}
	}
	return &creds, nil
}

// Write saves credentials atomically with owner-only permissions: the
// directory is 0700 and the file 0600.
func (s *Store) Write(creds *Credentials) error {
	fail := &StoreError{"local_state_unavailable", "Unable to securely save local rzv credentials."}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fail
	}
	if err := os.Chmod(s.Dir, 0o700); err != nil {
		return fail
	}
	data, err := json.Marshal(creds)
	if err != nil {
		return fail
	}
	suffix := make([]byte, 12)
	if _, err := rand.Read(suffix); err != nil {
		return fail
	}
	tmp := filepath.Join(s.Dir, "."+credentialsFile+"."+hex.EncodeToString(suffix)+".tmp")
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fail
	}
	_, werr := f.Write(data)
	serr := f.Sync()
	cerr := f.Close()
	if werr != nil || serr != nil || cerr != nil {
		_ = os.Remove(tmp)
		return fail
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fail
	}
	if err := os.Rename(tmp, s.path()); err != nil {
		_ = os.Remove(tmp)
		return fail
	}
	return nil
}

// Clear removes the credentials file. A missing file is not an error.
func (s *Store) Clear() error {
	info, err := os.Lstat(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return &StoreError{"local_state_unavailable", "Unable to remove local rzv credentials."}
	}
	if !info.Mode().IsRegular() {
		return &StoreError{"local_state_invalid", "Local rzv credentials are not a regular file."}
	}
	if err := os.Remove(s.path()); err != nil {
		return &StoreError{"local_state_unavailable", "Unable to remove local rzv credentials."}
	}
	return nil
}
