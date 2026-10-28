package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestValidProfileName(t *testing.T) {
	for _, name := range []string{"default", "work", "a", "team-2", strings.Repeat("a", 32)} {
		if err := ValidProfileName(name); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
	for _, name := range []string{"", "-work", "Work", "work_1", "a.b", "a/b", "../x", strings.Repeat("a", 33)} {
		if err := ValidProfileName(name); err == nil {
			t.Errorf("%q accepted", name)
		}
	}
}

func TestProfilesStoreSeparatelyAndListSorted(t *testing.T) {
	dir := t.TempDir()
	root := &Store{Dir: dir}
	work := root.ForProfile("work")
	if err := root.Write(sample()); err != nil {
		t.Fatal(err)
	}
	other := sample()
	other.AccessToken = "another-token"
	if err := work.Write(other); err != nil {
		t.Fatal(err)
	}
	if got, _ := root.Read(); got == nil || got.AccessToken != "not-a-real-token" {
		t.Fatalf("default profile changed: %+v", got)
	}
	if got, _ := work.Read(); got == nil || got.AccessToken != "another-token" {
		t.Fatalf("work profile: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "profiles", "work.json")); err != nil {
		t.Fatalf("work profile file: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(dir, "profiles", "work.json"))
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("profile file mode %v", info.Mode().Perm())
		}
	}
	names, err := root.ListProfiles()
	if err != nil || !reflect.DeepEqual(names, []string{"default", "work"}) {
		t.Fatalf("ListProfiles = %v, %v", names, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profiles", "Bad.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	names, _ = root.ListProfiles()
	if !reflect.DeepEqual(names, []string{"default", "work"}) {
		t.Fatalf("invalid file listed: %v", names)
	}
	if err := work.Clear(); err != nil {
		t.Fatal(err)
	}
	if got, _ := work.Read(); got != nil {
		t.Fatal("cleared profile still readable")
	}
}

func TestActiveProfileRoundTripAndInvalid(t *testing.T) {
	dir := t.TempDir()
	store := &Store{Dir: dir}
	if active, err := store.ActiveProfile(); err != nil || active != "" {
		t.Fatalf("no active profile: %q, %v", active, err)
	}
	if err := store.SetActiveProfile("work"); err != nil {
		t.Fatal(err)
	}
	if active, err := store.ActiveProfile(); err != nil || active != "work" {
		t.Fatalf("active = %q, %v", active, err)
	}
	if err := store.ClearActiveProfile(); err != nil {
		t.Fatal(err)
	}
	if err := store.ClearActiveProfile(); err != nil {
		t.Fatalf("second clear: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, activeFile), []byte(`{"version":1,"profile":"../etc"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ActiveProfile(); err == nil {
		t.Fatal("invalid active profile accepted")
	}
}
