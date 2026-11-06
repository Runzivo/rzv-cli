#!/usr/bin/env bash
# End-to-end check of docs/cursor-worker.md against one Runzivo sandbox.
#
#   RZV_PROFILE=<profile> [E2E_CURSOR_KEY_FILE=<file>] scripts/21-cursor-worker-e2e.sh
#
# Runs only against the shared test environment (`rzv whoami` must report
# `Environment: shared`). It creates one sandbox and deletes it on exit,
# including after a failure. Without E2E_CURSOR_KEY_FILE it installs the Cursor
# CLI and checks the worker's errors with no key and with a fake key. With a
# Cursor user API key in E2E_CURSOR_KEY_FILE it also starts the worker and
# checks it stays connected; then start a Cloud Agent on the machine in Cursor.
# The key goes over SSH stdin into a mode 600 file and never into evidence.
set -euo pipefail

rzv="${RZV_BIN:-$(command -v rzv)}"
evidence="${E2E_EVIDENCE:-$(mktemp -d /tmp/rzv-cursor-worker.XXXXXX)}"
mkdir -p "$evidence"
sandbox=""
log() { printf '%s %s\n' "$(date -u +%H:%M:%S)" "$*" | tee -a "$evidence/log.txt" >&2; }
fail() { log "FAIL: $*"; exit 1; }
field() { python3 -I -c 'import json,sys; d=json.load(sys.stdin)
for k in sys.argv[1].split("."): d=d[k]
print(d)' "$1"; }
cleanup() {
  status=$?
  if [ -n "$sandbox" ]; then
    "$rzv" sandboxes delete "$sandbox" --json >/dev/null 2>&1 || { log "FAIL: delete $sandbox"; status=1; }
    rm -f "$HOME/.ssh/rzv/$sandbox.conf"
    log "deleted $sandbox"
  fi
  if [ "$status" -eq 0 ]; then log "PASS (evidence: $evidence)"; else log "exit $status (evidence: $evidence)"; fi
}
trap cleanup EXIT
"$rzv" whoami | grep -q "^Environment: shared " || fail "not the shared environment"

"$rzv" sandboxes create --name "e2e-cursor-worker-$(date +%s)" --size 2vcpu-8gb --json > "$evidence/create.json"
sandbox="$(field sandbox.id < "$evidence/create.json")"
deadline=$((SECONDS + 600))
until [ "$("$rzv" sandboxes get "$sandbox" --json 2>/dev/null | field sandbox.status 2>/dev/null)" = running ]; do
  [ "$SECONDS" -lt "$deadline" ] || fail "not running after 600 s"; sleep 5; done
"$rzv" sandboxes ssh-config "$sandbox" --write > /dev/null
on() { ssh -o BatchMode=yes -o ConnectTimeout=90 "$sandbox.rzv" "$@"; }
on true || fail "ssh $sandbox.rzv"

log "1. install the Cursor CLI"
on 'curl -fsS https://cursor.com/install | bash >/dev/null && ~/.local/bin/agent --version' > "$evidence/version.txt" 2> "$evidence/install.err" || fail "install"
log "   agent $(cat "$evidence/version.txt")"

run_worker() { # $1: seconds; worker reads the key from the file, if any
  on "export PATH=\$HOME/.local/bin:\$PATH; cd /workspace; k=~/.config/cursor-worker/api-key
    [ -f \$k ] && export CURSOR_API_KEY=\"\$(cat \$k)\"; timeout $1 agent worker --worker-dir /workspace --name $sandbox start </dev/null; echo rc=\$?" 2>&1; }
put_key() { on 'umask 077; mkdir -p ~/.config/cursor-worker && cat > ~/.config/cursor-worker/api-key'; }

log "2. no key"
run_worker 60 > "$evidence/nokey.txt" || true
grep -q 'Authentication required' "$evidence/nokey.txt" || fail "no-key error missing"

log "3. fake key"
printf 'key_e2e_fake_%064d' 0 | put_key
run_worker 60 | sed 's/key_e2e_fake_[0-9]*/REDACTED/g' > "$evidence/badkey.txt" || true
grep -q 'API key is invalid' "$evidence/badkey.txt" || fail "fake-key error missing"

if [ -n "${E2E_CURSOR_KEY_FILE:-}" ]; then
  log "4. real key: the worker must stay connected for 60 s"
  put_key < "$E2E_CURSOR_KEY_FILE"
  run_worker 60 > "$evidence/worker.txt" || true
  grep -q 'rc=124' "$evidence/worker.txt" || fail "worker exited early: $(tail -3 "$evidence/worker.txt")"
fi
on 'rm -f ~/.config/cursor-worker/api-key'
"$rzv" sandboxes get "$sandbox" > "$evidence/get.txt"
grep 'credits used' "$evidence/get.txt" | tee -a "$evidence/log.txt" >&2
