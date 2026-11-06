# Custom runner profiles

A custom runner profile is a runner shape you choose: vCPU, memory and
temporary disk, each set independently, saved under a label. Put its
workflow label in `runs-on`. Built-in labels keep working unchanged.

| Command                               | What it does                                                  |
| ------------------------------------- | ------------------------------------------------------------- |
| `rzv runners profiles list`           | The workspace's profiles, with each `runs-on` workflow label. |
| `rzv runners profiles create ...`     | Save a profile (see below). Prints the label to use.          |
| `rzv runners profiles delete <id>`    | Archive a profile. Jobs already admitted keep their shape.    |
| `rzv runners availability`            | Whether each profile can run now, per region, and its rate.   |
| `rzv runners availability --vcpu ...` | The same for a shape you have not saved.                      |

```sh
rzv runners profiles create --name Wide --label rzv-wide --vcpu 32 --mem-gib 16
rzv runners availability --vcpu 32 --mem-gib 16 --disk-gib 64
```

- `--vcpu` is 2, 4, 8, 16 or 32; `--mem-gib` 4 to 128; `--disk-gib` 16 to 256. 16 GiB of disk is included and is the default.
- The label is `rzv-` then lowercase letters, digits or single hyphens, and
  must not start with `rzv-ubuntu-`. The workflow label adds this
  environment's prefix when it has one; use the one `rzv` prints.
- Availability is "Available now", "Busy - jobs will queue" or "Not
  supported", overall and per region. "Not available yet: pricing not
  enabled" means profiles cannot run here yet. "Shared CPU (overcommitted)"
  means the shape has more vCPU than the host's physical CPU.
- The rate is equivalent minutes per runner minute, billed from the same
  balance as every other runner.
- With an API key, the reads need `runners:read` or `runners:write`; create
  and delete need `runners:write`. Availability may be asked 60 times a
  minute per workspace.
- These commands answer that profiles are not turned on until Runzivo
  enables them for your environment.

Back to [Workspace commands](workspace-commands.md).
