# Third-party notices

Each `rzv` executable (`rzv-darwin-arm64`, `rzv-linux-x64`) is the official Node.js runtime with the rzv program embedded through Node's single executable application feature. It includes these open-source components, which stay under their own licenses.

Versions are for `release-2026-10-03-033543` and `release-2026-10-04-161209` (same executables).

| Component | Version | License | Source and full license text |
| --- | --- | --- | --- |
| Node.js | 26.8.2 | MIT, plus the licenses of the libraries Node.js bundles (V8, OpenSSL, libuv, ICU, llhttp, zlib and others) | https://github.com/nodejs/node/blob/v26.8.2/LICENSE |
| jose | 5.10.0 | MIT | https://github.com/panva/jose/blob/v5.10.0/LICENSE.md |
| openapi-fetch | 0.17.0 | MIT | https://github.com/openapi-ts/openapi-typescript/blob/main/packages/openapi-fetch/LICENSE |
| openapi-typescript-helpers | 0.1.0 | MIT | https://github.com/openapi-ts/openapi-typescript/blob/main/packages/openapi-typescript-helpers/LICENSE |

`install.sh` is a POSIX shell script written by Runzivo and uses only tools already on your system (`curl`, `shasum` or `sha256sum`).
