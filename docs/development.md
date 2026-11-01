# Development

`rzv` is a single Go module using golang.org/x/crypto for OpenSSH keys and certificates. The versions of this dependency and its indirect golang.org/x/sys dependency are pinned in go.mod and go.sum. It needs the Go version in `go.mod`.

```sh
go build -o rzv ./cmd/rzv   # build for this machine
go test ./...               # unit tests
gofmt -l . && go vet ./...  # formatting and static checks
```

## Layout

| Path                  | Contents                                                     |
| --------------------- | ------------------------------------------------------------ |
| `cmd/rzv`             | Entry point                                                  |
| `internal/cli`        | Argument parsing, commands and output                        |
| `internal/api`        | Typed client for the customer API                            |
| `internal/oidc`       | Browser sign-in: PKCE, loopback callback, token verification |
| `internal/config`     | Environment presets and the credential store                 |
| `internal/githubauth` | GitHub device flow for `rzv github authorize`                |
| `internal/oidctest`   | Fake identity provider used by the tests                     |
| `internal/version`    | Build metadata, set at release time with `-ldflags`          |

The API client follows the Runzivo customer API contract (`api/customer.openapi.yaml` in the Runzivo service). It is written by hand against that contract and covers only the operations `rzv` calls. Tests use local fake servers and throwaway keys; they never reach Runzivo or use real credentials.

The SSH integration test runs a private local sshd behind a fake relay. It covers ssh, scp, sftp, rsync, local forwarding, certificate refresh, host-key rotation, CA rejection and installed-binary login/API-key access. Install these OpenSSH tools and rsync to run it; otherwise it reports a skip. It never changes the system sshd or contacts a real Runzivo service.

## CI

`.github/workflows/ci.yml` checks formatting, vets, runs the tests with the race detector, builds every release file (`scripts/build.sh`) and runs the installer against them (`scripts/test-install.sh`). Linux jobs run on Runzivo's own microVM runners (`runs-on: rzv-ubuntu-24.04-x64-2vcpu-8gb`). macOS and Windows builds are cross-compiled only; smoke tests on those systems come later. Third-party actions are pinned to full commit SHAs. CI downloads the pinned Go modules; `go mod verify` checks their cached contents.

Releases: [Releasing](releasing.md).

Back to the [documentation index](README.md).
