# Sandboxes

A sandbox is a Linux virtual machine in your workspace. Your files live in `/workspace` and survive a stop and a start. It lasts **eight hours from its first start**, running or stopped; then the machine and its files are deleted. Time stopped counts toward the eight hours, and the lifetime cannot be extended.

```sh
rzv sandboxes create --name dev [--size 2vcpu-8gb|4vcpu-16gb|8vcpu-32gb|16vcpu-64gb] [--disk-gib 20]
rzv sandboxes list                       # status, time left and credits used
rzv sandboxes get <sandbox-id>
rzv sandboxes stop <sandbox-id>          # keeps the files, stops the charge
rzv sandboxes start <sandbox-id>
rzv sandboxes delete <sandbox-id>        # deletes the machine and its files
```

Add `--json` to list, get, create, start, stop, delete, exec or terminal-ticket. A workspace may run a limited number of sandboxes at once; `list` shows the limit and the sizes this Runzivo environment offers.

`create` takes `--idempotency-key <key>` (8 to 200 characters). Running it again with the same key returns the sandbox the first run created instead of a second one.

| Page                                            | What it covers                                    |
| ----------------------------------------------- | ------------------------------------------------- |
| [Running commands](sandboxes-exec.md)           | `rzv sandboxes exec`, its options and exit status |
| [SSH and file transfers](sandboxes-ssh.md) | OpenSSH, scp, sftp, rsync and Remote SSH editors |
| [Terminal](sandboxes-terminal.md)               | `terminal`, `ssh` and `terminal-ticket`         |
| [Billing and permissions](sandboxes-billing.md) | What is charged and who may change sandboxes      |

Back to the [documentation index](README.md).
