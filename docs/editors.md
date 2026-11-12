# VS Code and Cursor Remote SSH

Open a sandbox's `/workspace` in desktop VS Code or Cursor over the same SSH connection `rzv` sets up for `ssh`. You need macOS or Linux, OpenSSH, a signed-in `rzv` profile and a running sandbox. You don't need a password, a public guest address or an extra key.

## Set up

1. Install the editor's Remote SSH extension: Microsoft's **Remote - SSH** for VS Code, or Anysphere's **Remote - SSH** for Cursor (`cursor --install-extension anysphere.remote-ssh`).
2. Write the configuration and check plain SSH:

   ```sh
   rzv sandboxes ssh-config <sandbox-id> --write
   ssh <sandbox-id>.rzv -- true
   ```

   `--write` prints the commands for the next step.

3. Open the sandbox:
   - VS Code: `code --remote ssh-remote+<sandbox-id>.rzv /workspace`, or run **Remote-SSH: Connect to Host...** and choose `<sandbox-id>.rzv`. On macOS, `code` is on your `PATH` only after you run **Shell Command: Install 'code' command in PATH** once in VS Code.
   - Cursor: `cursor --remote ssh-remote+<sandbox-id>.rzv /workspace`, or run **Remote-SSH: Connect to Host...** and choose `<sandbox-id>.rzv`. On macOS, `cursor` is on your `PATH` only after you run **Shell Command: Install 'cursor' command** once in Cursor.

4. If the editor asks for the platform, choose **Linux**. To skip that prompt, set it in user settings:

   ```json
   {
     "remote.SSH.remotePlatform": { "<sandbox-id>.rzv": "linux" },
     "remote.SSH.connectTimeout": 90
   }
   ```

   A sandbox that has just started can take up to 60 seconds before it accepts SSH. Certificate and relay setup come on top of that, so use 90 seconds or more. The default 15-second timeout can expire first.

The editor uses system OpenSSH and your `~/.ssh/config`. If you set `remote.SSH.configFile` to another file, add the `Include` line that `--write` put in `~/.ssh/config` to that file. If `remote.SSH.path` points to a different `ssh`, it must be OpenSSH, because the configuration uses `Match exec`.

## What happens on connect

Each connection runs the same steps as `ssh <sandbox-id>.rzv`. A `Match exec` step refreshes a 15-minute user certificate. `ProxyCommand rzv sandboxes ssh --stdio` then opens the authenticated relay. The host certificate is checked strictly against the sandbox host CA. The editor installs its server in `~/.vscode-server` or `~/.cursor-server` on the sandbox and reaches it through SSH dynamic forwarding. The window stays open after the login certificate expires. If the connection drops, the editor reconnects with a fresh certificate.

## Editor server download

By default (`"remote.SSH.localServerDownload": "auto"`) the sandbox downloads the editor server from the editor vendor over outbound HTTPS. If that fails, your computer downloads the server and copies it over SSH. If the sandbox has no outbound internet, set `"always"` to skip the failed attempt on the sandbox. Extensions from the Marketplace also need outbound access from the sandbox. Without it, download the extension `.vsix` on your computer and run **Extensions: Install from VSIX...** in the remote window.

## Ports and limits

Use the editor's **Ports** view, or `LocalForward` in your own SSH config, to reach services on the sandbox's localhost. The configuration doesn't enable agent forwarding, X11 forwarding or remote forwarding. A stopped, expired or deleted sandbox disconnects the editor. Windows clients are not supported yet.

A stop and start keeps the sandbox's disk, including `~/.vscode-server`, `~/.cursor-server` and the extensions you installed on the remote side, so the next connection doesn't reinstall the server. Open windows don't resume after a stop. Reconnect to the sandbox.

Tested with VS Code 1.145.0 and Remote - SSH 0.128.0 on macOS against an Ubuntu 24.04 sandbox. The tests covered the first connection and reconnecting after the login certificate expired. With the server already installed, a connection took about 6 seconds. In a later test, `~/.vscode-server` was still installed after a stop and start.

Also tested with Cursor 3.29.9 and Anysphere Remote - SSH 1.1.17 on macOS, opened with `cursor --remote`. The sandbox downloaded the Cursor server for the editor's commit and started it in about 27 seconds. After a stop and start, Cursor reconnected in about 8 seconds, with its server and `/workspace` kept.

References: [VS Code Remote SSH](https://code.visualstudio.com/docs/remote/ssh), [VS Code remote troubleshooting](https://code.visualstudio.com/docs/remote/troubleshooting) and [Cursor network troubleshooting](https://cursor.com/help/troubleshooting/network).

Back to [Sandbox SSH](sandboxes-ssh.md).
