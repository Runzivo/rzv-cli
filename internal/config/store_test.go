package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func sample() *Credentials {
	return &Credentials{
		Version: 1, Environment: "prd", Issuer: "https://id.example.test/realms/x",
		ClientID: "rzv-cli", Audience: "rzv-customer-api", GitHubBrokerClaim: "identity_provider",
		APIBaseURL: "https://api.example.test", AccessToken: "not-a-real-token",
	}
}

func TestStoreRoundTripAndPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runzivo")
	store := &Store{Dir: dir}
	got, err := store.Read()
	if err != nil || got != nil {
		t.Fatalf("empty store: got %v, %v", got, err)
	}
	if err := store.Write(sample()); err != nil {
		t.Fatal(err)
	}
	got, err = store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if *got != *sample() {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(dir, credentialsFile))
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("file mode %v, want 0600", info.Mode().Perm())
		}
		dirInfo, _ := os.Stat(dir)
		if dirInfo.Mode().Perm() != 0o700 {
			t.Fatalf("dir mode %v, want 0700", dirInfo.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
	if err := store.Clear(); err != nil {
		t.Fatal(err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("second clear: %v", err)
	}
}

func TestStoreRejectsInvalidAndSymlink(t *testing.T) {
	dir := t.TempDir()
	store := &Store{Dir: dir}
	if err := os.WriteFile(filepath.Join(dir, credentialsFile), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("want invalid error, got %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	other := filepath.Join(t.TempDir(), "elsewhere.json")
	_ = os.WriteFile(other, []byte(`{}`), 0o600)
	_ = os.Remove(filepath.Join(dir, credentialsFile))
	if err := os.Symlink(other, filepath.Join(dir, credentialsFile)); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err := store.Read(); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("want symlink refusal, got %v", err)
	}
}

func TestDirectoryOverride(t *testing.T) {
	env := map[string]string{"RZV_CONFIG_DIR": "/tmp/rzv-test"}
	got, err := Directory(func(k string) string { return env[k] })
	if err != nil || got != "/tmp/rzv-test" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestLookupEnvironment(t *testing.T) {
	env, err := LookupEnvironment("")
	if err != nil || env.APIBaseURL != "https://api.runzivo.dev" {
		t.Fatalf("default: %+v %v", env, err)
	}
	env, err = LookupEnvironment("shrd")
	if err != nil || env.Name != "shared" {
		t.Fatalf("alias: %+v %v", env, err)
	}
	if _, err := LookupEnvironment("nope"); err == nil {
		t.Fatal("unknown environment accepted")
	}
}
