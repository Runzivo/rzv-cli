# Persistent disks

A persistent disk keeps files after a sandbox ends. Sandboxes last eight hours; a
disk lasts until you delete it.

```sh
rzv disks create --name data --size-gib 10      # 1 to 64 GiB
rzv disks list                                  # disks, where they are attached, quota
rzv sandboxes create --name dev --disk vol_...  # mounted at /mnt/disk while it runs
rzv disks detach vol_...                        # the sandbox must be stopped
rzv disks attach vol_... --sandbox sbx_...      # the sandbox must be stopped
rzv disks delete vol_...                        # detached disks only; see below
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
- If the disk fails to mount in a running sandbox, `rzv sandboxes get` shows
  `not mounted` with the reason, commands run in the sandbox are refused, and
  SSH and terminal sessions print a warning. While the sandbox is still checking
  the mount, `get` shows `checking`.
- Disks cannot be resized yet.
- Sizes are in GiB (1 GiB is 1024^3 bytes).
- Deleting a disk removes its files for good. The disk shows as `deleting` until
  its server confirms the files are removed, then as `deleted`. Check with
  `rzv disks get vol_...`.

## Availability, quota and charges

Persistent disks are off unless Runzivo has turned them on for your environment
and given your workspace a disk quota. `rzv disks list` shows the quota. No disk
price is published and disk charges are off; `list` shows the metered GB-hours
(decimal GB of the disk's size, by the hour) for information. Disks use the
`sandboxes:read` and `sandboxes:write` permissions, and API keys need the same
scopes.

## Scheduled deletion

If a disk's charges go unpaid, or the workspace is closed, Runzivo deletes the
disk 30 days later. `rzv disks list` and `rzv disks get` show the deletion time
in UTC and the reason, and `--json` carries them as `scheduled_deletion`.

- Workspace owners and admins get a notice when the 30 days start, 7 days
  before the deletion and 1 day before it. Each notice gives the exact UTC time.
- Paying what is owed, or reopening the workspace, cancels the deletion. If
  charges go unpaid again later, a new 30 days starts.
- A staff hold on the workspace (a block, a suspension or a credit review)
  pauses the countdown instead of running it down.
- Runzivo keeps no backup of a disk. Once it is deleted, its files are gone.

Back to [sandboxes](sandboxes.md) or the [documentation index](README.md).
