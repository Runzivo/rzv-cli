# Development

`rzv` is a single Go module with no third-party dependencies. It needs the Go version in `go.mod`.

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

## CI

`.github/workflows/ci.yml` checks formatting, vets, runs the tests with the race detector and cross-compiles every release target. Linux jobs run on Runzivo's own microVM runners (`runs-on: rzv-ubuntu-24.04-x64-2vcpu-8gb`). macOS and Windows builds are cross-compiled only; smoke tests on those systems come later. Third-party actions are pinned to full commit SHAs.

Back to the [documentation index](README.md).
