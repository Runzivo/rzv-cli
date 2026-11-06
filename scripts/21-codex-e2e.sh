#!/usr/bin/env bash
# End-to-end check of the preinstalled Codex CLI in one Runzivo sandbox,
# following docs/codex.md. Uses the signed-in rzv profile (RZV_PROFILE) and
# needs ssh and python3.
#
#   RZV_PROFILE=<profile> KEY_FILE=<mode-600 OpenAI key file> \
#     [E2E_WORKSPACE=<id>] [E2E_MODEL=gpt-5-nano] scripts/21-codex-e2e.sh
#
# It runs only against the shared test environment: it stops unless
# `rzv whoami` reports `Environment: shared`. It creates one sandbox and
# deletes it on exit, including after a failure. The key goes to Codex over
# SSH stdin and is never printed or written to the evidence directory
# ($E2E_EVIDENCE, default: a temp directory, printed at the end).
set -euo pipefail

rzv="${RZV_BIN:-$(command -v rzv)}"
key_file="${KEY_FILE:?set KEY_FILE to a mode 600 file holding an OpenAI API key}"
model="${E2E_MODEL:-gpt-5-nano}"
ws=(${E2E_WORKSPACE:+--workspace "$E2E_WORKSPACE"})
work="$(mktemp -d)"
evidence="${E2E_EVIDENCE:-$work/evidence}"
mkdir -p "$evidence"
sandbox=""
name="codex-e2e-$(date +%s)-$$"
log() { printf '%s %s\n' "$(date -u +%H:%M:%S)" "$*" | tee -a "$evidence/log.txt" >&2; }
fail() { log "FAIL: $*"; exit 1; }
field() { python3 -I -c 'import json,sys; d=json.load(sys.stdin)
for k in sys.argv[1].split("."): d=d[k]
print(d)' "$1"; }
redact() { python3 -I -c 'import sys,re; k=open(sys.argv[1]).read().strip(); t=sys.stdin.read()
print(re.sub(r"(?<![A-Za-z0-9])sk-[A-Za-z0-9_-]{20,}","[redacted]",t.replace(k,"[redacted]") if k else t),end="")' "$key_file"; }

# Runs once, on normal exit or on INT/TERM. A signal during create is covered
# by looking the sandbox up by its unique name.
cleaned=""
cleanup() { # [exit status, set for signals]
  status=${1:-$?}
  [ -n "$cleaned" ] && return; cleaned=1
  trap '' INT TERM
  if [ -z "$sandbox" ] && [ -n "$name" ]; then
    sandbox="$("$rzv" sandboxes list ${ws[@]+"${ws[@]}"} --json 2>/dev/null | python3 -I -c 'import json,sys
for s in json.load(sys.stdin)["sandboxes"]:
    if s["name"] == sys.argv[1] and s["status"] not in ("deleted", "expired"): print(s["id"])' "$name" || true)"
  fi
  if [ -n "$sandbox" ]; then
    log "cleanup: deleting $sandbox"
    if "$rzv" sandboxes delete "$sandbox" ${ws[@]+"${ws[@]}"} --json >/dev/null 2>"$work/delete.err"; then
      rm -f "$HOME/.ssh/rzv/$sandbox.conf"
    else
      log "FAIL: could not delete $sandbox; delete it with: rzv sandboxes delete $sandbox"
      status=1
    fi
  fi
  if grep -rlE "(^|[^A-Za-z0-9])sk-[A-Za-z0-9_-]{20,}" "$evidence" >&2; then log "FAIL: key-like text in evidence"; status=1; fi
  if [ "$status" -eq 0 ]; then log "PASS"; else log "exit $status"; fi
  log "evidence: $evidence"
  exit "$status"
}
trap cleanup EXIT
trap 'cleanup 130' INT
trap 'cleanup 143' TERM

wait_status() { # <want> <seconds>
  local deadline=$((SECONDS + $2))
  until [ "$("$rzv" sandboxes get "$sandbox" ${ws[@]+"${ws[@]}"} --json | field sandbox.status)" = "$1" ]; do
    [ "$SECONDS" -lt "$deadline" ] || fail "sandbox not $1 after $2 s"
    sleep 5
  done
}
on() { ssh -o BatchMode=yes -o ConnectTimeout=90 "$sandbox.rzv" "$@"; }
# Runs a script in the sandbox; output (redacted) goes to the named evidence file.
run_in() { on 'bash -s' 2>&1 | redact > "$evidence/$1" || true; }

[ "$(stat -f %Lp "$key_file" 2>/dev/null || stat -c %a "$key_file")" = 600 ] || fail "KEY_FILE must be mode 600"
"$rzv" whoami | grep -q "^Environment: shared " || fail "not the shared environment"

"$rzv" sandboxes create --name "$name" --size 2vcpu-8gb ${ws[@]+"${ws[@]}"} --json > "$evidence/create.json" || fail "create failed"
sandbox="$(field sandbox.id < "$evidence/create.json")"
wait_status running 600
"$rzv" sandboxes ssh-config "$sandbox" ${ws[@]+"${ws[@]}"} --write >/dev/null
on true || fail "ssh"
log "1. $sandbox running, ssh works"

on 'codex login --with-api-key' < "$key_file" 2>&1 | redact > "$evidence/login.txt"
on 'codex login status; stat -c "%a" ~/.codex/auth.json' 2>&1 | redact > "$evidence/login-status.txt"
grep -qx 600 "$evidence/login-status.txt" || fail "auth.json is not mode 600 (login-status.txt)"
log "2. signed in with the key over SSH stdin"

# Functional check of the image's own bubblewrap, before this script installs
# one: a write inside the workspace works and a write outside it is blocked.
run_in image-bwrap.txt <<'EOF'
dir=/workspace/codex-image-check
mkdir -p "$dir"
if command -v bwrap >/dev/null; then
  codex sandbox -P :workspace -C "$dir" -- sh -c "touch $dir/ok && ! touch /home/user/x" && echo image-bwrap-ok || echo image-bwrap-failed
else
  echo "skipped: no bwrap in this image"
fi
EOF
if grep -q "skipped: no bwrap in this image" "$evidence/image-bwrap.txt"; then
  log "2b. skipped: no bwrap in this image"
else
  grep -q image-bwrap-ok "$evidence/image-bwrap.txt" || fail "Codex sandbox with the image's bwrap (image-bwrap.txt)"
  log "2b. Codex sandbox with the image's bwrap: inside write ok, outside write blocked"
fi

# Codex's own workspace-write sandbox needs bwrap. When the image has none,
# try the Ubuntu package and record why it failed; the run then goes on to
# the documented danger-full-access path.
run_in bwrap-install.txt <<'EOF'
command -v bwrap >/dev/null && { echo "bwrap already present"; exit 0; }
sudo apt-get update 2>&1 | tail -5
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y bubblewrap 2>&1 | tail -5
command -v bwrap >/dev/null && bwrap --version || echo "bwrap unavailable"
EOF
if grep -q "bwrap unavailable" "$evidence/bwrap-install.txt"; then
  log "3. skipped workspace-write: bubblewrap could not be installed (bwrap-install.txt)"
else
  run_in workspace-write.txt <<EOF
set -u
dir=/workspace/codex-e2e
mkdir -p "\$dir" && cd "\$dir" && git init -q
codex sandbox -P :workspace -C "\$dir" -- sh -c "touch \$dir/in.txt && ! touch /home/user/outside.txt" && echo confinement-ok || echo confinement-failed
timeout 300 codex exec -m $model --sandbox workspace-write "Create a file named ws.txt in the current directory containing exactly the text: runzivo-codex-ws-ok" </dev/null
echo "file=\$(cat ws.txt 2>/dev/null)"
EOF
  grep -q confinement-ok "$evidence/workspace-write.txt" || fail "workspace-write confinement (workspace-write.txt)"
  grep -q "file=runzivo-codex-ws-ok" "$evidence/workspace-write.txt" || fail "workspace-write task (workspace-write.txt)"
  log "3. workspace-write: inside write ok, outside write blocked, task passed"
fi

run_in full-access.txt <<EOF
set -u
mkdir -p ~/.codex && printf 'sandbox_mode = "danger-full-access"\n' > ~/.codex/config.toml
mkdir -p /workspace/codex-e2e && cd /workspace/codex-e2e
timeout 300 codex exec -m $model "Create a file named full.txt in the current directory containing exactly the text: runzivo-codex-full-ok" </dev/null
echo "file=\$(cat full.txt 2>/dev/null)"
EOF
grep -q "sandbox: danger-full-access" "$evidence/full-access.txt" || fail "config.toml sandbox_mode not applied (full-access.txt)"
grep -q "file=runzivo-codex-full-ok" "$evidence/full-access.txt" || fail "danger-full-access task (full-access.txt)"
log "4. config.toml sandbox_mode = danger-full-access task passed"

on 'codex logout; test ! -e ~/.codex/auth.json && echo auth-removed' > "$evidence/logout.txt" 2>&1 || true
grep -q auth-removed "$evidence/logout.txt" || fail "logout left ~/.codex/auth.json"
log "5. codex logout removed ~/.codex/auth.json"

"$rzv" sandboxes stop "$sandbox" ${ws[@]+"${ws[@]}"} --json > "$evidence/stop.json"; wait_status stopped 600
"$rzv" sandboxes start "$sandbox" ${ws[@]+"${ws[@]}"} --json > "$evidence/start.json"; wait_status running 600
on true || { sleep 20; on true; } || fail "ssh after start"
on 'for p in /workspace/codex-e2e/full.txt ~/.codex/config.toml /usr/bin/bwrap /usr/local/bin/codex; do test -e $p && echo "kept $p" || echo "gone $p"; done' > "$evidence/after-start.txt" 2>&1
grep -q "kept /workspace/codex-e2e/full.txt" "$evidence/after-start.txt" || fail "/workspace not kept (after-start.txt)"
log "6. after stop and start: $(tr '\n' ';' < "$evidence/after-start.txt")"

"$rzv" sandboxes get "$sandbox" ${ws[@]+"${ws[@]}"} --json > "$evidence/get.json"
log "7. metered: runtime_seconds=$(field sandbox.runtime_seconds < "$evidence/get.json") credits_used=$(field sandbox.credits_used < "$evidence/get.json")"
