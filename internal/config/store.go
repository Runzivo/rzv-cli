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
	"time"
)

const (
	credentialsFile = "credentials.json"
	profilesDir     = "profiles"
	activeFile      = "profile.json"
)

// Credentials is what `rzv login` stores. The access and refresh tokens are
// never printed or logged, and the file is owner-only (0600 in a 0700
// directory). RefreshToken renews the access token within the identity
// provider's session limits; RefreshExpiresAt is when the provider said it
// stops working if unused (zero when unknown).
type Credentials struct {
	Version     int    `json:"version"`
	Environment string `json:"environment,omitempty"`
	// Identity is a display label (username or email) from the ID token.
	Identity          string    `json:"identity,omitempty"`
	Issuer            string    `json:"issuer"`
	ClientID          string    `json:"clientId"`
	Audience          string    `json:"audience"`
	GitHubBrokerClaim string    `json:"githubBrokerClaim"`
	APIBaseURL        string    `json:"apiBaseUrl"`
	AccessToken       string    `json:"accessToken"`
	RefreshToken      string    `json:"refreshToken,omitempty"`
	RefreshExpiresAt  time.Time `json:"refreshExpiresAt,omitzero"`
	SelectedWorkspace string    `json:"selectedAccountId,omitempty"`
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

// Store reads and writes the credentials of one profile in one directory.
// The default profile keeps credentials.json in the directory itself; every
// other profile keeps profiles/<name>.json.
type Store struct {
	Dir string
	// Profile is "" or DefaultProfile for the default profile.
	Profile string
}

// ForProfile returns a store for another profile in the same directory.
func (s *Store) ForProfile(name string) *Store {
	return &Store{Dir: s.Dir, Profile: name}
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

func (s *Store) path() string {
	if s.Profile == "" || s.Profile == DefaultProfile {
		return filepath.Join(s.Dir, credentialsFile)
	}
	return filepath.Join(s.Dir, profilesDir, s.Profile+".json")
}

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
	data, err := json.Marshal(creds)
	if err != nil {
		return fail
	}
	return writeAtomic(s.path(), data)
}

// writeAtomic replaces path with data through a temporary file in the same
// directory. The directory is 0700 and the file 0600.
func writeAtomic(path string, data []byte) error {
	fail := &StoreError{"local_state_unavailable", "Unable to securely save local rzv credentials."}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fail
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fail
	}
	suffix := make([]byte, 12)
	if _, err := rand.Read(suffix); err != nil {
		return fail
	}
	tmp := filepath.Join(dir, "."+filepath.Base(path)+"."+hex.EncodeToString(suffix)+".tmp")
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
	// Windows can refuse the replace briefly while another rzv reads the
	// file, so retry before giving up.
	var renameErr error
	for attempt := 0; attempt < 10; attempt++ {
		if renameErr = os.Rename(tmp, path); renameErr == nil {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = os.Remove(tmp)
	return fail
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

// Lock takes this profile's credential lock, waiting up to wait. It is an
// operating-system file lock, so it is released when the process exits, even
// when killed. Every change to the credentials file happens under it: the
// identity provider rotates refresh tokens and refuses a spent one, so a
// stale copy written back would end the session. The returned function
// releases the lock.
func (s *Store) Lock(wait time.Duration) (func(), error) {
	fail := &StoreError{"local_state_unavailable", "Another rzv process is using the sign-in. Try again shortly."}
	path := s.path() + ".lock"
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fail
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, &StoreError{"local_state_invalid", "The rzv credential lock is not a regular file."}
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fail
	}
	deadline := time.Now().Add(wait)
	for {
		ok, err := tryLockFile(f)
		if err != nil {
			_ = f.Close()
			return nil, fail
		}
		if ok {
			return func() { _ = unlockFile(f); _ = f.Close() }, nil
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fail
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// Update changes the stored credentials under the lock: it reads the file
// again, applies change and writes the result, so a field it does not change
// (such as a refresh token another process just rotated) is kept.
func (s *Store) Update(wait time.Duration, change func(*Credentials) error) error {
	unlock, err := s.Lock(wait)
	if err != nil {
		return err
	}
	defer unlock()
	creds, err := s.Read()
	if err != nil {
		return err
	}
	if creds == nil {
		return &StoreError{"authentication_required", "Sign in with `rzv login` first."}
	}
	if err := change(creds); err != nil {
		return err
	}
	return s.Write(creds)
}
