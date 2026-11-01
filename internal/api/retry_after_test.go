package api

import (
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	for value, want := range map[string]time.Duration{
		"":       0,
		"1":      time.Second,
		" 3 ":    3 * time.Second,
		"0":      0,
		"-2":     0,
		"soon":   0,
		"999999": maxRetryAfter,
		now.Add(4 * time.Second).Format(http.TimeFormat): 4 * time.Second,
		now.Add(-time.Hour).Format(http.TimeFormat):      0,
	} {
		if got := parseRetryAfter(value, now); got != want {
			t.Errorf("parseRetryAfter(%q) = %s, want %s", value, got, want)
		}
	}
}

func TestDecodeErrorKeepsRetryAfter(t *testing.T) {
	err := decodeError(http.StatusServiceUnavailable, []byte(`{"error":{"code":"sandbox_ssh_starting"}}`), "2")
	if err.Code != "sandbox_ssh_starting" || err.RetryAfter != 2*time.Second {
		t.Fatalf("decoded = %+v", err)
	}
	if err := decodeError(http.StatusServiceUnavailable, []byte(`not json`), "2"); err.Code != "invalid_error_response" || err.RetryAfter != 0 {
		t.Fatalf("malformed = %+v", err)
	}
}
