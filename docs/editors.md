# VS Code and Cursor Remote SSH

Open a sandbox's `/workspace` in desktop VS Code or Cursor over the same SSH connection `rzv` sets up for `ssh`. You need macOS or Linux, OpenSSH, a signed-in `rzv` profile and a running sandbox. You don't need a password, a public guest address or an extra key.

## Set up

1. Install the editor's Remote SSH extension: Microsoft's **Remote - SSH** for VS Code. Cursor ships its own Remote SSH extension.
2. Write the configuration and check plain SSH:

   ```sh
   rzv sandboxes ssh-config <sandbox-id> --write
   ssh <sandbox-id>.rzv -- true
   ```

   `--write` prints the commands for the next step.

3. Open the sandbox:
   - VS Code: `code --remote ssh-remote+<sandbox-id>.rzv /workspace`, or run **Remote-SSH: Connect to Host...** and choose `<sandbox-id>.rzv`.
   - Cursor: run **Remote-SSH: Connect to Host...**, choose `<sandbox-id>.rzv`, then open `/workspace`.

4. If the editor asks for the platform, choose **Linux**. To skip that prompt, set it in user settings:

   ```json
   {
     "remote.SSH.remotePlatform": { "<sandbox-id>.rzv": "linux" },
     "remote.SSH.connectTimeout": 60
   }
   ```

   A sandbox that has just started can take up to 60 seconds before it accepts SSH. The default 15-second timeout can expire first.

The editor uses system OpenSSH and your `~/.ssh/config`. If you set `remote.SSH.configFile` to another file, add the `Include` line that `--write` put in `~/.ssh/config` to that file. If `remote.SSH.path` points to a different `ssh`, it must be OpenSSH, because the configuration uses `Match exec`.

## What happens on connect

Each connection runs the same steps as `ssh <sandbox-id>.rzv`. A `Match exec` step refreshes a 15-minute user certificate. `ProxyCommand rzv sandboxes ssh --stdio` then opens the authenticated relay. The host certificate is checked strictly against the sandbox host CA. The editor installs its server in `~/.vscode-server` or `~/.cursor-server` on the sandbox and reaches it through SSH dynamic forwarding. The window stays open after the login certificate expires. If the connection drops, the editor reconnects with a fresh certificate.

## Editor server download

By default the sandbox downloads the editor server from the editor vendor over outbound HTTPS. If the sandbox has no outbound internet, set `"remote.SSH.localServerDownload": "always"`. Your computer then downloads the server and copies it over SSH. Extensions from the Marketplace need outbound access the same way.

## Ports and limits

Use the editor's **Ports** view, or `LocalForward` in your own SSH config, to reach services on the sandbox's localhost. The configuration doesn't enable agent forwarding, X11 forwarding or remote forwarding. A stopped, expired or deleted sandbox disconnects the editor. Windows clients are not supported yet.

Known limitation: stopping and starting a sandbox currently clears everything outside `/workspace`, including `~/.vscode-server` and `~/.cursor-server`. After a start, the editor installs its server again on the next connection, so that connection is slower. Extensions you installed on the remote side are gone and must be installed again. Your files in `/workspace` are kept. Open windows don't resume after a stop. Reconnect to the sandbox.

Tested with VS Code 1.145.0 and Remote - SSH 0.128.0 on macOS against an Ubuntu 24.04 sandbox. Cursor uses the same configuration but has not been tested against a live sandbox yet.

References: [VS Code Remote SSH](https://code.visualstudio.com/docs/remote/ssh), [VS Code remote troubleshooting](https://code.visualstudio.com/docs/remote/troubleshooting) and [Cursor network troubleshooting](https://cursor.com/help/troubleshooting/network).

Back to [Sandbox SSH](sandboxes-ssh.md).
