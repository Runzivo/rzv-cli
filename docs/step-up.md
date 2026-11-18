# Confirming an action with an emailed code

Some actions need a fresh confirmation: billing changes, creating an API key, and workspace security changes (workspace type, owner and admin roles, removing members, invitations with an owner or admin role or custom teams, teams and closure requests). Without one, the service refuses with `reauth_required` (exit code 3).

## In a terminal

- When you sign in with GitHub, Google or GitLab, the only fresh confirmation is a one-time code sent to your account's verified email. Signing in again does not help.
- `rzv` asks the service whether it can send you a code. If it can, `rzv` shows the masked address (such as `d…@example.com`), sends the code and asks for the 6 digits.
- A wrong code can be entered again. After five wrong codes, `rzv` stops; run the command again for a new code.
- If the code expires, `rzv` offers to send one new code.
- After the code is accepted, `rzv` sends the original request once more, with the same idempotency key or operation id. The confirmation lasts 15 minutes.
- `rzv` never prints, logs or stores the code. Prompts go to standard error.

## When no code is offered

- No verified email: confirm your email in Sign-in methods in the Runzivo portal, then retry.
- The service cannot offer a code here, or does not support it yet: do the action in the Runzivo portal, or, if you sign in another way, run `rzv login` and retry.
- Not in a terminal, or with `--json`: `rzv` never prompts. It exits with code 3 and says to run the command again in a terminal.
- With `RZV_API_KEY`, `rzv` never prompts.

Back to [Workspace commands](workspace-commands.md).
