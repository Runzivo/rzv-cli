# Codex in a sandbox

Every sandbox includes OpenAI's [Codex CLI](https://github.com/openai/codex) (Apache-2.0) as `codex`. It runs with your own OpenAI API key or ChatGPT sign-in. Codex sends requests straight from the sandbox to OpenAI, and OpenAI bills you for them. Runzivo charges only the sandbox's compute ([billing](sandboxes-billing.md)) and keeps no copy of your key or sign-in.

## Sign in

Connect once with `rzv sandboxes ssh-config <sandbox-id> --write` ([SSH](sandboxes-ssh.md)). With an API key, pipe it from your password manager over SSH, which is encrypted from your computer to the sandbox:

```sh
<password-manager-command> | ssh <sandbox-id>.rzv 'codex login --with-api-key'
```

With a ChatGPT plan, run `codex login --device-auth` in the sandbox (`rzv sandboxes ssh <sandbox-id>`). Open the link it prints on your computer and enter the code. OpenAI calls device code sign-in [beta](https://learn.chatgpt.com/docs/auth#login-on-headless-devices). It must be turned on in your ChatGPT security settings, or by your workspace admin.

Either way, Codex saves the sign-in in `~/.codex/auth.json` in the sandbox. Treat that file like a password, and never copy it into `/workspace`. `codex login status` shows how you are signed in.

Do not pass keys with `rzv sandboxes exec --env` or in an `exec` command. Runzivo keeps the exec request until the sandbox host receives it, and your local shell history records it.

## Sandbox mode

Codex's own `workspace-write` sandbox needs `bubblewrap`, which the sandbox image does not include. Without it, Codex cannot edit files and says that bubblewrap is unavailable.

The Runzivo sandbox is already its own virtual machine, which is the isolation boundary. So turn off Codex's own sandbox in `~/.codex/config.toml`:

```toml
sandbox_mode = "danger-full-access"
```

`codex exec --sandbox danger-full-access` does the same for one run. In this mode Codex can change any file in the sandbox and reach the network.

## After a stop and start

A stop and start keeps the sandbox's disk: `/workspace` and `~/.codex` (your sign-in, `config.toml` and history) are still there after `start`. Running sessions end. A new sandbox always starts without `~/.codex`.

## Sign out and clean up

`codex logout` deletes `~/.codex/auth.json`. Run it before you stop a sandbox if you don't want the sign-in kept, because a stop keeps the file. Expiry and `rzv sandboxes delete <sandbox-id>` remove the sandbox and the file. To stop using a key everywhere, revoke it in the OpenAI dashboard.

## Limits

- The sandbox needs outbound HTTPS to OpenAI.
- Codex can run commands and change files in the sandbox. Review what it does before you push.
- `scripts/21-codex-e2e.sh` runs these steps against one sandbox and deletes it afterwards.

Back to [Sandboxes](sandboxes.md).
