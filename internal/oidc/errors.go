// Package oidc signs a person in with the OpenID Connect authorization code
// flow, PKCE (S256) and a loopback redirect, then verifies the returned ID
// and access tokens before rzv stores the access token.
package oidc

// Error carries a stable code and exit code. Messages never include tokens,
// codes or verifiers.
type Error struct {
	Code    string
	Exit    int
	Message string
}

func (e *Error) Error() string     { return e.Message }
func (e *Error) ErrorCode() string { return e.Code }
func (e *Error) ExitCode() int     { return e.Exit }

func configError(msg string) *Error { return &Error{"invalid_configuration", 2, msg} }
func authError(msg string) *Error   { return &Error{"authentication_required", 3, msg} }
func unavailable(msg string) *Error { return &Error{"service_unavailable", 5, msg} }
