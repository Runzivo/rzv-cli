# rzv-cli agent guide

Runzivo-wide rules (autonomy, reviews, testing, shared/prd rollout, approvals, coordination): read [rzv-business/operating-policy.md](https://github.com/Runzivo/rzv-business/blob/main/operating-policy.md) (local checkout: ../rzv-business/operating-policy.md) and its operating-decisions.md before any work. This file adds only rzv-cli-specific guidance.

`rzv` is the Runzivo customer CLI: one Go module, one static executable.

## Layout and commands

The package layout is in [docs/development.md](docs/development.md):
`cmd/rzv` is the entry point, `internal/cli` holds commands and output,
`internal/api` is the typed customer API client.

```sh
go build -o rzv ./cmd/rzv
gofmt -l . && go vet ./...
go test -race -count=1 ./...
bash scripts/build.sh <dist-dir> release-0000-00-00-000000
bash scripts/test-install.sh <dist-dir>
```

CI (`.github/workflows/ci.yml`) runs the same steps. Tests use local fake
servers and throwaway keys; they never reach Runzivo or use real credentials.

## API contract and parity

- `internal/api` is written by hand against the Runzivo customer API contract
  (`api/customer.openapi.yaml` in the Runzivo service) and covers only the operations
  `rzv` calls. Match field names and error shapes to that contract.
- Every customer action in the portal needs an `rzv` command, with `--json`
  output. Document new commands in `docs/` and the `## Unreleased` section of
  `CHANGELOG.md`.
- Keep working against older services: omit optional new request fields when
  they hold the default, and show a clear message or `-` when a service lacks a
  field or operation.
- No staff commands. Staff work stays in the staff console; `rzv` covers the
  customer API only.

## Public repository

- The source is public so customers can read what they run. It does not accept
  outside contributions; fork pull requests never run on Runzivo runners.
- Commit nothing internal: no hostnames, inventory, internal URLs, secrets or
  unreleased plans. `rzv` never prints access tokens.
- Third-party actions stay pinned to full commit SHAs; dependency changes
  update `THIRD_PARTY_NOTICES.md`.

## Releases

Follow [docs/releasing.md](docs/releasing.md): tags are
`release-YYYY-MM-DD-HHMMSS` (UTC), `VERSION` holds the rzv version,
`release.yml` builds a draft release from the pushed tag, and publishing the
draft moves the installer to the new version.
