# AI settings and usage

`rzv ai` manages the selected workspace's AI connections, settings, and usage.
Use `--workspace <id>` to select a different workspace and `--json` for the complete API response.

List connections with `rzv ai providers list`. Add a connection with a service provider identifier supplied by your deployment:

```sh
rzv ai providers add --kind <provider-kind> --label "Team connection" --key-file ./credential.txt
rzv ai providers models <provider-id> --refresh
rzv ai providers test <provider-id>
```

The credential is read from the named file only while making the request. It is never saved by the CLI or printed. Adding, updating, removing, and testing a provider require your own sign-in.

Read settings with `rzv ai settings show`. Update them with `rzv ai settings update --file settings.json`. The file is a JSON partial PUT: fields omitted from it keep their current values, and `null` clears either monthly limit. It accepts `mode`, `agents`, `autonomy`, `monthly_token_limit`, `monthly_cost_limit_cents`, and `allowed_roles`; the service validates their values.

```sh
rzv ai usage summary --from 2026-01-01 --to 2026-01-31 --group-by day
rzv ai usage calls --limit 50 --cursor <cursor>
rzv ai usage limit
```

Summary groups are `model`, `agent`, `member`, `provider`, and `day`. Date values accept RFC 3339 timestamps or `YYYY-MM-DD`. Reads use the permissions assigned to your signed-in identity or workspace API key. Mutations require your own sign-in.
