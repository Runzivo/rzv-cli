# Claude Code in a sandbox

[Claude Code](https://code.claude.com/docs/en/overview) is Anthropic's coding agent. It is preinstalled in Runzivo sandboxes at `/usr/local/bin/claude`; `claude --version` prints the version. Use it with your own Anthropic account. Claude Code sends requests straight from the sandbox to Anthropic, and Anthropic bills you for them. Runzivo charges only the sandbox's compute ([billing](sandboxes-billing.md)) and keeps no copy of your key or login.

Open a shell with `rzv sandboxes ssh <sandbox-id>`, then `cd /workspace` and sign in one of the ways below.

## Sign in with a Claude subscription

Run `claude` and follow the login prompt, or run `claude auth login` first. No browser opens in the sandbox, so open the login URL that Claude Code shows on your computer (press `c` to copy it), sign in, and paste the code you get back at `Paste code here if prompted`. Claude Code saves the login in `~/.claude/.credentials.json` (mode 600) in the sandbox. `/logout` removes it. See Anthropic's [authentication guide](https://code.claude.com/docs/en/authentication).

## Use an API key

Enter the key in the sandbox, not on your computer's command line. That keeps it out of your shell history and out of Runzivo:

```sh
read -rs ANTHROPIC_API_KEY && export ANTHROPIC_API_KEY   # paste, then Enter
claude                                                    # or: claude -p "..."
```

The key lasts only as long as that shell. To copy a key from a password manager into a file instead, pipe it over SSH, which is encrypted from your computer to the sandbox. This needs the SSH alias from `rzv sandboxes ssh-config <sandbox-id> --write` ([SSH](sandboxes-ssh.md)):

```sh
<password-manager-command> | ssh <sandbox-id>.rzv 'umask 077; mkdir -p ~/.secrets && cat > ~/.secrets/anthropic.key'
```

Then start Claude Code with the key from that file. The command line holds only the file name, not the key:

```sh
ANTHROPIC_API_KEY="$(cat ~/.secrets/anthropic.key)" claude   # or: claude -p "..."
```

Do not pass keys with `rzv sandboxes exec --env` or in an `exec` command. Runzivo keeps the exec request until the sandbox host receives it, and your local shell history records it. Never put a key in `/workspace`.

## After a stop and start

At the moment a stop and start removes every file outside `/workspace`: your login in `~/.claude`, `~/.claude/settings.json`, any key file and Claude Code's session history. Claude Code itself is still installed. After each `start`, and in each new sandbox, sign in or add your key again. Keep work you want to keep in `/workspace`, or push it. `rzv sandboxes get <sandbox-id>` shows the running time and credits used.

## Clean up

- Run `/logout` in Claude Code, or `rm ~/.secrets/anthropic.key`, before you share the sandbox or when you are done.
- `rzv sandboxes delete <sandbox-id>` deletes the sandbox and all its files, including any saved login or key. At expiry the sandbox stops and its files are deleted too.
- Revoke a key you no longer need in the [Claude Console](https://platform.claude.com).

## Errors

- No key and no login: `claude -p` prints `Not logged in · Please run /login`.
- A wrong or revoked key: `Invalid API key · Fix external API key`.
- Add a valid key or sign in again, then rerun the command.
- Claude Code can run commands and change files in the sandbox. Review what it does before you push.
- `claude -p` uses an `ANTHROPIC_API_KEY` that is set. Interactive `claude` asks once whether to use it; `/status` shows which sign-in is in use.

Running Claude Code's cloud sessions on your own sandbox (Anthropic's self-hosted environments) is not supported yet: Anthropic offers it as a beta.

Back to [Sandboxes](sandboxes.md).
