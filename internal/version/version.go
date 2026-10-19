// Package version holds build metadata. Release builds set these with
// -ldflags "-X github.com/Runzivo/rzv-cli/internal/version.Version=...".
package version

var (
	// Version is the rzv release number, for example 0.9.0.
	Version = "dev"
	// Commit is the source commit the binary was built from.
	Commit = "unknown"
	// Date is the build time in RFC 3339.
	Date = "unknown"
)
