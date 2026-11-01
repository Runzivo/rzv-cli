package cli

import "testing"

func TestMessageLocalePrecedence(t *testing.T) {
	for _, tc := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{}, "English"},
		{map[string]string{"LANG": "fr_CA.UTF-8"}, "Français"},
		{map[string]string{"LANG": "en_CA.UTF-8", "LC_MESSAGES": "fr-CA"}, "Français"},
		{map[string]string{"LANG": "fr_CA.UTF-8", "LC_ALL": "C"}, "English"},
	} {
		a := &app{Deps: Deps{Getenv: func(k string) string { return tc.env[k] }}}
		if got := a.text("English", "Français"); got != tc.want {
			t.Fatalf("%v: %q", tc.env, got)
		}
	}
}
