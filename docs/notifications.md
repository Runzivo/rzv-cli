# Notifications

Use `rzv notifications` to read workspace notifications and manage your category settings. The command uses the selected workspace unless you pass `--workspace <id>` or `--account <id>`.

```sh
rzv notifications list
rzv notifications list --category billing --unread true --limit 25
rzv notifications list --cursor '<next-cursor>'
rzv notifications read 12 13
rzv notifications read --all
rzv notifications settings show
rzv notifications settings update --category support --in-app true --email false
```

`list` prints the unread count and, when another page exists, its cursor. `--limit` is 1 to 100. Pass that cursor unchanged to read the next page. Categories are `billing`, `jobs`, `account`, `support`, and `system`. `read` takes up to 100 ids, or `--all`.

For a settings update, both `--in-app` and `--email` are required because the API replaces the category's complete in-app and email setting. Use `settings show` first if you need to preserve one existing value. The returned `email_locked` value shows settings the server has locked.

Notifications are personal to the signed-in member in the selected workspace. They need a human sign-in, so `RZV_API_KEY` is refused before the CLI makes a network request.

Back to [Workspace commands](workspace-commands.md).
