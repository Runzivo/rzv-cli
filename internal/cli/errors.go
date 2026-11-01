package cli

import (
	"errors"
	"fmt"
)

// Exit codes. They match the 0.8.0 TypeScript CLI so scripts keep working.
const (
	exitFailure      = 1
	exitUsage        = 2
	exitAuth         = 3
	exitAccessDenied = 4
	exitUnavailable  = 5
	exitNotFound     = 6
)

// Error is a failure with a stable machine code, an exit code and a
// message that is safe to print. Messages never contain tokens.
type Error struct {
	Code    string
	Exit    int
	Message string
	// Quiet errors set the exit code without printing: a later step reports
	// the same failure once.
	Quiet bool
}

func (e *Error) Error() string { return e.Message }

func newError(code string, exit int, format string, args ...any) *Error {
	return &Error{Code: code, Exit: exit, Message: fmt.Sprintf(format, args...)}
}

func usageError(format string, args ...any) *Error {
	return newError("usage", exitUsage, format, args...)
}

func authError(format string, args ...any) *Error {
	return newError("authentication_required", exitAuth, format, args...)
}

func accessError(format string, args ...any) *Error {
	return newError("access_denied", exitAccessDenied, format, args...)
}

func unavailableError(format string, args ...any) *Error {
	return newError("service_unavailable", exitUnavailable, format, args...)
}

func notFoundError(format string, args ...any) *Error {
	return newError("not_found", exitNotFound, format, args...)
}

func conflictError(format string, args ...any) *Error {
	return newError("conflict", exitFailure, format, args...)
}

// asError converts any error into a printable *Error. Errors from other
// packages that already carry a code and exit code keep them; anything else
// becomes a generic failure so internal details are not printed.
func asError(err error) *Error {
	var cliErr *Error
	if errors.As(err, &cliErr) {
		return cliErr
	}
	var coded interface {
		error
		ErrorCode() string
		ExitCode() int
	}
	if errors.As(err, &coded) {
		return &Error{Code: coded.ErrorCode(), Exit: coded.ExitCode(), Message: coded.Error()}
	}
	return &Error{Code: "unexpected_error", Exit: exitFailure, Message: "rzv could not complete the request."}
}
