# OpenCode in a sandbox

[OpenCode](https://github.com/anomalyco/opencode) (MIT) is an open-source coding agent. You can run it in a Runzivo sandbox with your own AI provider key. OpenCode sends requests straight from the sandbox to your provider, and your provider bills you for them. Runzivo charges only the sandbox's compute ([billing](sandboxes-billing.md)) and keeps no copy of your AI key outside the sandbox.

## Install

Open a shell in a running sandbox with `rzv sandboxes ssh <sandbox-id>`, then install the pinned release. The script checks the release's SHA-256 checksum:

```sh
version=1.19.5
sha256=588310a336fa3b9df36b3f0225b26f679d453efaad0cf3d66c3651ef3ec1647a
tmp=$(mktemp -d)
curl -fsSL --proto '=https' -o "$tmp/opencode.tar.gz" \
  "https://github.com/anomalyco/opencode/releases/download/v$version/opencode-linux-x64.tar.gz"
echo "$sha256  $tmp/opencode.tar.gz" | sha256sum -c - \
  && mkdir -p ~/.local/bin && tar -xzf "$tmp/opencode.tar.gz" -C ~/.local/bin opencode
rm -rf "$tmp"
export PATH="$HOME/.local/bin:$PATH"
opencode --version   # 1.19.5
```

For a newer release, take its version and the `sha256` digest of `opencode-linux-x64.tar.gz` from the [releases page](https://github.com/anomalyco/opencode/releases).

## After a stop and start

At the moment a stop and start removes every file outside `/workspace`: the OpenCode install, `~/.config/opencode` and any key you saved. After each `start`, and in each new sandbox, install OpenCode and add your key again.

## Add your key

Enter the key in the sandbox, not on your computer's command line. That keeps it out of your shell history and out of Runzivo:

```sh
read -rs ANTHROPIC_API_KEY && export ANTHROPIC_API_KEY   # paste, then Enter
opencode                                                  # or: opencode run -m anthropic/<model> "..."
```

The key lasts only as long as that shell. Use `OPENAI_API_KEY` for OpenAI. Once a key is set, `opencode models <provider>` lists that provider's models. To use the key in other shells while the sandbox runs, run `opencode auth login` (or `/connect` in OpenCode). OpenCode then saves it in `~/.local/share/opencode/auth.json` in the sandbox. A stop, expiry or deletion removes that file; `opencode auth logout` removes it sooner. Never put a key in `/workspace`.

To copy a key from a password manager, pipe it over SSH, which is encrypted from your computer to the sandbox. This needs the SSH alias from `rzv sandboxes ssh-config <sandbox-id> --write` ([SSH](sandboxes-ssh.md)):

```sh
<password-manager-command> | ssh <sandbox-id>.rzv 'umask 077; mkdir -p ~/.config/opencode && cat > ~/.config/opencode/anthropic.key'
```

Then point the provider at that file in `~/.config/opencode/opencode.json`: `{"provider": {"anthropic": {"options": {"apiKey": "{file:~/.config/opencode/anthropic.key}"}}}}`.

Do not pass keys with `rzv sandboxes exec --env` or in an `exec` command. Runzivo keeps the exec request until the sandbox host receives it, and your local shell history records it.

## Limits

- The sandbox needs outbound HTTPS to your provider and to `github.com` for the download.
- A wrong key fails with the provider's own authentication error, such as `invalid x-api-key` from Anthropic. With no key set, OpenCode 1.19.5 prints `UnknownError` with the message `Unexpected server error`. That error does not name the key. Add your key and run it again.
- OpenCode can run commands and change files in the sandbox. Review what it does before you push.

Back to [Sandboxes](sandboxes.md).
