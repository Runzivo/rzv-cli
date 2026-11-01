package cli

import "strings"

// text selects translations for new messages using the process message locale.
// Existing commands retain their English output until translated individually.
func (a *app) text(en, frCA string) string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if locale := strings.ToLower(strings.TrimSpace(a.Getenv(key))); locale != "" {
			if strings.HasPrefix(locale, "fr_ca") || strings.HasPrefix(locale, "fr-ca") {
				return frCA
			}
			break
		}
	}
	return en
}
