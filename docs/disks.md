# Persistent disks

A persistent disk keeps files after a sandbox ends. Sandboxes last eight hours; a
disk lasts until you delete it.

```sh
rzv disks create --name data --size-gib 10      # 1 to 64 GiB
rzv disks list                                  # disks, where they are attached, quota
rzv sandboxes create --name dev --disk vol_...  # mounted at /mnt/disk while it runs
rzv disks detach vol_...                        # the sandbox must be stopped
rzv disks attach vol_... --sandbox sbx_...      # the sandbox must be stopped
rzv disks delete vol_...                        # detached disks only; files are gone
```

Add `--json` to any of them. `create` takes `--idempotency-key <key>`.

## How a disk behaves

- A disk lives on one Runzivo server. The first sandbox that uses it places it
  there. Every later sandbox that uses it runs on that server or waits for it.
  If that server is unavailable, so is the disk. It is not copied to another
  server and it is not a backup.
- One sandbox uses a disk at a time. Stop the sandbox before you detach the disk
  or attach it to another stopped sandbox. A deleted or expired sandbox releases
  its disk.
- Disks cannot be resized yet.

## Availability, quota and charges

Persistent disks are off unless Runzivo has turned them on for your environment
and given your workspace a disk quota. `rzv disks list` shows the quota. No disk
price is published and disk charges are off; `list` shows the metered GB-hours
for information. Disks use the `sandboxes:read` and `sandboxes:write`
permissions, and API keys need the same scopes.

When a workspace is closed, its disks are deleted once no sandbox holds them.

Back to [sandboxes](sandboxes.md) or the [documentation index](README.md).
