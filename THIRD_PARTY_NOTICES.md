# Third-party notices

## rzv 0.9.0 and later

Each `rzv` executable is written in Go and statically links the Go standard library and runtime. It uses no other third-party code.

| Component                       | Version | License      | Source and full license text                     |
| ------------------------------- | ------- | ------------ | ------------------------------------------------ |
| Go standard library and runtime | 1.27.2  | BSD-3-Clause | https://go.dev/LICENSE (source: https://go.dev/) |

The Go version for each release is the one in `go.mod` at the release tag.

`install.sh` is a POSIX shell script written by Runzivo and uses only tools already on your system (`curl`, `shasum` or `sha256sum`).

## Earlier releases (rzv 0.1.0)

`release-2026-10-03-033543` and `release-2026-10-04-161209` (`rzv-darwin-arm64`, `rzv-linux-x64`) were the official Node.js runtime with the rzv program embedded through Node's single executable application feature. They include these open-source components, under their own licenses:

| Component                  | Version | License                                                                                                    | Source and full license text                                                                           |
| -------------------------- | ------- | ---------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| Node.js                    | 26.8.2  | MIT, plus the licenses of the libraries Node.js bundles (V8, OpenSSL, libuv, ICU, llhttp, zlib and others) | https://github.com/nodejs/node/blob/v26.8.2/LICENSE                                                    |
| jose                       | 5.10.0  | MIT                                                                                                        | https://github.com/panva/jose/blob/v5.10.0/LICENSE.md                                                  |
| openapi-fetch              | 0.17.0  | MIT                                                                                                        | https://github.com/openapi-ts/openapi-typescript/blob/main/packages/openapi-fetch/LICENSE              |
| openapi-typescript-helpers | 0.1.0   | MIT                                                                                                        | https://github.com/openapi-ts/openapi-typescript/blob/main/packages/openapi-typescript-helpers/LICENSE |
