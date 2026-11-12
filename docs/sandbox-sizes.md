# Sandbox sizes and plans

`rzv sandboxes sizes` lists every supported size, including the ones your plan does not allow:

```text
SIZE         VCPU  RAM     RATE (eq-min/min)  PLAN           AVAILABILITY
2vcpu-8gb    2     8 GiB   1                  included       available
4vcpu-16gb   4     16 GiB  2                  included       busy (all pools full now)
8vcpu-32gb   8     32 GiB  4                  Team (locked)  available
16vcpu-64gb  16    64 GiB  8                  Team (locked)  unsupported (no host fits)

Locked: 8vcpu-32gb needs the Team plan; 16vcpu-64gb needs the Team plan. Upgrade: <portal link>
```

- RATE is the equivalent minutes a running minute costs.
- PLAN is `included` or the plan the size needs. A locked size stays listed and visible. It does not affect sandboxes that already exist: starting one is never refused for its plan.
- AVAILABILITY is the capacity now: `available`, `busy` (every pool that can hold it is full; try later or a smaller size) or `unsupported` here.
- The upgrade link points at the billing page of the portal that belongs to the Runzivo environment you are signed in to.
- `--json` returns the server's entries as `size_availability` (`vcpu`, `mem_mib`, `equivalent_minutes_per_minute`, `plan_eligible`, `required_plan`). `rzv sandboxes list --json` includes the same `size_availability` when the server sends it.

Creating a locked size fails with exit code 4 and names the plan, for example `rzv sandboxes create --name dev --size 8vcpu-32gb` prints `8vcpu-32gb needs the Team plan. Upgrade: <portal link>`. A size that needs the Enterprise plan says `needs the Enterprise plan. Contact Runzivo.` While Runzivo has no plan limits turned on, every size shows `included`. A Runzivo service from before plan-based sizes shows `-` for vCPU, RAM and rate and treats every size as included.

Back to [Sandboxes](sandboxes.md).
