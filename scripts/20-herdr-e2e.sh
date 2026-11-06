#!/usr/bin/env bash
# End-to-end check of Herdr (https://github.com/herdrdev/herdr) against one
# Runzivo sandbox, following docs/herdr.md. Uses the signed-in rzv profile
# (RZV_PROFILE) and needs ssh, curl, shasum or sha256sum, and python3.
#
#   RZV_PROFILE=<profile> [E2E_WORKSPACE=<id>] scripts/20-herdr-e2e.sh
#
# It runs only against the shared test environment: it stops unless
# `rzv whoami` reports `Environment: shared`, so a profile mix-up cannot reach
# production.
# It creates one sandbox and deletes it on exit, including after a failure.
# Evidence goes to $E2E_EVIDENCE (default: a temp directory, printed at the
# end). No credential is written there.
set -euo pipefail

herdr_version=0.9.5
# SHA-256 values published for v0.9.5 in https://herdr.dev/latest.json and as
# GitHub release asset digests.
sha_linux_x86_64=14d9e8bc8b4a9e0118d77c32266f76d461f09b40342eb23752960bd89f59929d
sha_linux_aarch64=17d7536f2da50859523680ef279dffcac8e4a235f4e46fe8da6c2ec44141897c
sha_macos_x86_64=5fb0a908ec7f6745e3e245738b06bfad56c46391c391ab3c0955fbead01f1369
sha_macos_aarch64=fb77c2232a05a8035421382c894392f4174e4e38a18e48af3dd0290157c3a7e2
release="https://github.com/herdrdev/herdr/releases/download/v$herdr_version"

rzv="${RZV_BIN:-$(command -v rzv)}"
size="${E2E_SIZE:-2vcpu-8gb}"
ws=(${E2E_WORKSPACE:+--workspace "$E2E_WORKSPACE"})
work="$(mktemp -d /tmp/rzv-herdr.XXXXXX)" # short: Herdr's socket path must fit sun_path
evidence="${E2E_EVIDENCE:-$work/evidence}"
mkdir -p "$evidence"
sandbox=""
log() { printf '%s %s\n' "$(date -u +%H:%M:%S)" "$*" | tee -a "$evidence/log.txt" >&2; }
fail() { log "FAIL: $*"; exit 1; }
field() { python3 -I -c 'import json,sys; d=json.load(sys.stdin)
for k in sys.argv[1].split("."): d=d[k]
print(d)' "$1"; }
sha256() { if command -v sha256sum >/dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -d' ' -f1; }

cleanup() {
  status=$?
  if [ -n "$sandbox" ]; then
    log "cleanup: deleting $sandbox"
    if "$rzv" sandboxes delete "$sandbox" ${ws[@]+"${ws[@]}"} --json >/dev/null 2>"$work/delete.err"; then
      rm -f "$HOME/.ssh/rzv/$sandbox.conf"
    else
      log "FAIL: could not delete $sandbox ($(head -c 300 "$work/delete.err")); delete it with: rzv sandboxes delete $sandbox"
      status=1
    fi
  fi
  rm -rf "$work/herdr-local" "$work/herdr"
  if [ "$status" -eq 0 ]; then log "PASS"; else log "exit $status"; fi
  log "evidence: $evidence"
}
trap cleanup EXIT

wait_status() { # <want> <seconds>
  local deadline=$((SECONDS + $2)) st
  while :; do
    st="$("$rzv" sandboxes get "$sandbox" ${ws[@]+"${ws[@]}"} --json | field sandbox.status)"
    [ "$st" = "$1" ] && return 0
    [ "$SECONDS" -lt "$deadline" ] || fail "sandbox is $st, wanted $1"
    sleep 5
  done
}
# Remote helpers. Remote Herdr is called by absolute path: a non-interactive
# SSH shell may not have ~/.local/bin on PATH.
on() { ssh -o BatchMode=yes "$alias" "$@"; }
rh() { on "\$HOME/.local/bin/herdr $*"; }
# Local Herdr with its own config and state, so the check never touches the
# user's Herdr setup. OpenSSH still reads ~/.ssh/config, which holds the alias.
lh() { XDG_CONFIG_HOME="$work/herdr-local/c" XDG_STATE_HOME="$work/herdr-local/s" "$local_herdr" "$@" </dev/null; }

log "rzv: $("$rzv" --version)"
"$rzv" whoami | grep -q "^Environment: shared " || fail "the rzv profile is not signed in to the shared environment"

case "$(uname -s)-$(uname -m)" in
  Darwin-arm64) asset=macos-aarch64 sum=$sha_macos_aarch64 ;;
  Darwin-x86_64) asset=macos-x86_64 sum=$sha_macos_x86_64 ;;
  Linux-x86_64) asset=linux-x86_64 sum=$sha_linux_x86_64 ;;
  Linux-aarch64) asset=linux-aarch64 sum=$sha_linux_aarch64 ;;
  *) fail "no Herdr build for $(uname -sm)" ;;
esac
local_herdr="$work/herdr"
curl -fsSL -o "$local_herdr" "$release/herdr-$asset"
[ "$(sha256 "$local_herdr")" = "$sum" ] || fail "local Herdr checksum mismatch"
chmod +x "$local_herdr"
log "local: $("$local_herdr" --version) ($asset)"

log "1. create a sandbox and its SSH config"
name="herdr-e2e-$(date +%s)-$$"
# If create fails or its id cannot be read, find the sandbox by its unique
# name so the exit trap still deletes it.
by_name() { "$rzv" sandboxes list ${ws[@]+"${ws[@]}"} --json | python3 -I -c 'import json,sys
print(next((s["id"] for s in json.load(sys.stdin)["sandboxes"] if s["name"]==sys.argv[1] and s["status"]!="deleted"),""))' "$name"; }
"$rzv" sandboxes create --name "$name" --size "$size" ${ws[@]+"${ws[@]}"} --json > "$work/create.json" || { sandbox="$(by_name || true)"; fail "create failed"; }
sandbox="$(field sandbox.id < "$work/create.json" 2>/dev/null || true)"
[ -n "$sandbox" ] || { sandbox="$(by_name || true)"; fail "could not read the new sandbox id"; }
alias="$sandbox.rzv"
wait_status running 600
"$rzv" sandboxes ssh-config "$sandbox" ${ws[@]+"${ws[@]}"} --write >/dev/null
log "   $sandbox running"

log "2. install Herdr $herdr_version in the sandbox (pinned release, SHA-256 checked)"
on "sh -s" > "$evidence/install.txt" 2>&1 <<EOF || fail "install failed (see install.txt)"
set -eu
case "\$(uname -m)" in
  x86_64) a=linux-x86_64 s=$sha_linux_x86_64 ;;
  aarch64) a=linux-aarch64 s=$sha_linux_aarch64 ;;
  *) echo "unsupported \$(uname -m)"; exit 1 ;;
esac
mkdir -p "\$HOME/.local/bin"
curl -fsSL -o /tmp/herdr.download "$release/herdr-\$a"
echo "\$s  /tmp/herdr.download" | sha256sum -c -
install -m 0755 /tmp/herdr.download "\$HOME/.local/bin/herdr"
rm -f /tmp/herdr.download
"\$HOME/.local/bin/herdr" --version
uname -m
EOF
grep -q "herdr $herdr_version" "$evidence/install.txt" || fail "wrong Herdr version in the sandbox"

log "3. start the Herdr server and run panes (first SSH connection)"
# setsid: Herdr treats only a session-leader server as its detached daemon,
# which saved machines require. Herdr's own clients start it the same way.
# shellcheck disable=SC2016 # $HOME expands in the sandbox
start_server='setsid -f "$HOME/.local/bin/herdr" server >/dev/null 2>&1 </dev/null; sleep 3'
on "$start_server; pgrep -x herdr | head -1" > "$work/pid1"
rh status server > "$evidence/status-1.txt" || fail "server not running"
rh workspace create --cwd /workspace --label e2e --no-focus > "$work/ws.json"
pane="$(field result.root_pane.pane_id < "$work/ws.json")"
rh pane run "$pane" "'echo shell=\$\$ > /workspace/herdr-pane.txt; claude --version; echo HERDR_E2E_\"\"MARK'"
rh pane wait-output "$pane" --match HERDR_E2E_MARK --timeout 30000 > "$evidence/pane-1.json" || fail "pane output not seen"
shell_pid="$(on 'cut -d= -f2 /workspace/herdr-pane.txt')"
log "   server pid $(cat "$work/pid1"), pane $pane shell pid $shell_pid"

log "4. reattach from a second SSH connection: same server, pane still alive"
[ "$(on 'pgrep -x herdr | head -1')" = "$(cat "$work/pid1")" ] || fail "server changed"
on "kill -0 $shell_pid" || fail "pane shell died after disconnect"
boot1="$(on 'cat /proc/sys/kernel/random/boot_id')"
rh pane read "$pane" --source recent --lines 20 > "$evidence/pane-2.txt"
grep -q HERDR_E2E_MARK "$evidence/pane-2.txt" || fail "pane history missing on reattach"

log "5. Herdr's own SSH path from this machine (saved machine over the .rzv alias)"
lh machine add "$alias" --label rzv-e2e --remote-session default > "$evidence/machine-add.txt" 2>&1 || fail "herdr machine add failed (see machine-add.txt)"
lh --machine rzv-e2e pane read "$pane" --source recent --lines 20 > "$evidence/pane-3.txt" 2>&1 || fail "herdr --machine pane read failed"
grep -q HERDR_E2E_MARK "$evidence/pane-3.txt" || fail "remote pane read lacks the marker"
lh machine status rzv-e2e --json > "$evidence/machine-status.json" 2>&1 || fail "herdr machine status failed"
grep -q '"status": "reachable"' "$evidence/machine-status.json" || fail "saved machine not reachable"
lh machine list --json > "$evidence/machine-list.json"

log "6. stop and start: /workspace is kept; Herdr, its state and processes are not"
"$rzv" sandboxes stop "$sandbox" ${ws[@]+"${ws[@]}"} --json >/dev/null
wait_status stopped 300
"$rzv" sandboxes start "$sandbox" ${ws[@]+"${ws[@]}"} --json >/dev/null
wait_status running 600
"$rzv" sandboxes ssh-config "$sandbox" ${ws[@]+"${ws[@]}"} --write >/dev/null
[ "$(on 'cat /proc/sys/kernel/random/boot_id')" != "$boot1" ] || fail "the sandbox did not restart"
# shellcheck disable=SC2016 # $HOME and $f expand in the sandbox
on 'for f in /workspace/herdr-pane.txt "$HOME/.local/bin/herdr" "$HOME/.config/herdr/session.json"; do if test -e "$f"; then echo "kept $f"; else echo "lost $f"; fi; done; pgrep -x herdr >/dev/null && echo "herdr running" || echo "no herdr process"' > "$evidence/after-start.txt"
cat "$evidence/after-start.txt" >&2
grep -q '^kept /workspace/herdr-pane.txt' "$evidence/after-start.txt" || fail "/workspace lost across stop"
grep -q '^no herdr process' "$evidence/after-start.txt" || fail "a Herdr process survived the stop"
# Files outside /workspace are recorded, not asserted: what a stop keeps there
# depends on the Runzivo service.

log "7. delete"
"$rzv" sandboxes delete "$sandbox" ${ws[@]+"${ws[@]}"} --json >/dev/null
rm -f "$HOME/.ssh/rzv/$sandbox.conf"
sandbox=""
