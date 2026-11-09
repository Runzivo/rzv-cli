# Billing

| Command                                           | What it does                                                                                                                                                  |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `rzv billing`                                     | Minute balance for the period and the packs on sale.                                                                                                          |
| `rzv billing purchases`                           | Recent minute-pack orders.                                                                                                                                    |
| `rzv billing invoices`                            | Receipts and invoices. `--json` for the full rows.                                                                                                            |
| `rzv billing auto-recharge`                       | Auto-recharge settings. `set --enabled <true\|false> --threshold <minutes> --pack <id>` changes them (billing:write).                                         |
| `rzv billing country`                             | The workspace billing country. `set <code>` changes it for future purchases and `reset` clears it while the workspace has no credit purchase (billing:write). |
| `rzv billing marketplace`                         | The GitHub Marketplace plan for the workspace, if any.                                                                                                        |
| `rzv billing marketplace claimable`               | GitHub Marketplace purchases held because several workspaces own the buying account, which this workspace can claim. Owner only.                              |
| `rzv billing marketplace claim --account-id <id>` | Links a held purchase to this workspace. Owner only; needs a recent sign-in.                                                                                  |
| `rzv billing checkout`                            | Starts a hosted checkout: `--pack <id> --operation-id <id>`. Reuse the same `--operation-id` when retrying.                                                   |
| `rzv billing portal`                              | A link to the payment provider's billing portal. Do not store it.                                                                                             |
| `rzv billing codes redeem --code-file <path>`     | Redeem a promo code or prepaid voucher.                                                                                                                       |

Checkout and portal links are sign-in capabilities. Live payments stay off until Runzivo enables them.

Save a redemption code in a file and pass its path with `--code-file`. The code may contain at most 64 bytes. The command does not store the input and only displays the safe code representation returned by the service.

Code redemption requires the workspace's `billing:write` permission. It works with a human sign-in or with an API key that has the `all:all` scope and whose creator still has that permission.

## GitHub Marketplace purchases held for several workspaces

- When several workspaces are connected to the same GitHub account, its Marketplace purchase is held until one of them claims it.
- `rzv billing marketplace claimable` lists the held purchases (`ACCOUNT ID`, `ACCOUNT`) that this workspace can claim, or says none is waiting.
- `--json` prints `{"workspace_id", "claimable":[{"account_id","account_login"}]}`.
- `rzv billing marketplace claim --account-id <id>` links the held purchase of that GitHub account to this workspace. The plan applies after the next billing check.
- `--json` prints `{"workspace_id", "marketplace_claim":{"account_id","account_login","already_linked"}}`.
- Repeating a claim reports that the purchase is already linked and changes nothing (`already_linked` is true).
- Both need a workspace owner signed in as a person. API keys are refused.
- `claim` also needs a recent sign-in. If `rzv` says to sign in again, run `rzv login`, then repeat the command.
- Refusals: this workspace is not connected to that GitHub account, no held purchase for that account (linked elsewhere, cancelled or none), or the workspace already has another active plan. Each exits non-zero with a one-line reason.

Back to [Workspace commands](workspace-commands.md).
