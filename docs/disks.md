# Persistent disks

A persistent disk keeps files after a sandbox ends. Sandboxes default to eight
hours, with 12h and 24h choices where offered (see [Lifetime](sandboxes.md#lifetime)).
A persistent disk survives sandbox expiry and lasts until deleted, subject to
the scheduled deletion rules below.

```sh
rzv disks create --name data --size-gib 10      # 1 to 16384 GiB; Core enforces current limits
rzv disks list                                  # disks, where they are attached, quota
rzv disks get vol_...                           # status, usage and scheduled deletion
rzv sandboxes create --name dev --disk vol_...  # mounted at /mnt/disk while it runs
rzv disks detach vol_...                        # the sandbox must be stopped
rzv disks attach vol_... --sandbox sbx_...      # the sandbox must be stopped
rzv disks delete vol_...                        # detached disks only; see below
```

Add `--json` to any of them. `create` takes `--idempotency-key <key>` (8 to 200
characters).

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

Persistent disks must be enabled for your environment. `rzv disks list` shows
what your workspace can keep:

- In `grant` mode, Runzivo staff set a workspace quota. A zero quota means the
  workspace needs a grant from Runzivo support.
- In `capacity` mode, any workspace can create disks while the region has room.
  The workspace limit reflects its share of regional capacity and current free
  space, rather than its plan. A zero quota means no capacity is available.

`rzv disks list --json` includes `quota.mode`, `quota.region_capacity_gib` and
`quota.region_free_gib` when the service reports them, plus `min_size_gib` and
`max_size_gib`. Grant mode currently limits each disk to 64 GiB. In capacity mode,
`max_size_gib` changes with the workspace's share and the largest host's free
space. The CLI accepts 1 to 16384 GiB, the technical contract ceiling; Core
decides whether a request fits the current workspace and capacity limits.
Older services without the mode fields keep the grant-style quota display.

The published price is CA$0.10 per provisioned GiB-month (730 hours), prorated
while the disk exists, attached or detached, until deletion is confirmed. There
are no create, attach or detach fees. When charges are on, disks consume
equivalent minutes from the workspace balance. The customer disk estimate API,
`getCustomerDiskEstimate` (`GET /v1/customer/tenants/{tenantId}/disks/estimate`,
optional `size_gib` query), reports current CAD rates, reference USD/EUR/GBP
values, monthly estimates and `price.charges_enabled`. Disk charges are currently
off in production. Estimates are still available while charges are off, but
nothing is consumed. `list` shows metered GB-hours (decimal GB of the disk's size,
by the hour) and equivalent minutes used.

Disks use the `sandboxes:read` and `sandboxes:write` permissions. Grant these to
API keys with `rzv api-keys create --permissions`, or use the `all:all` scope.

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
