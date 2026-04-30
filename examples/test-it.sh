#!/usr/bin/env bash
# End-to-end smoke test for the agent-sandbox runtime.
#
# Pre-conditions:
#   - You have built the binaries: `make all` produced ./bin/agentd and ./bin/agentctl
#   - The daemon is running:
#       sudo ./bin/agentd -bpf-dir=$(pwd)/bpf -socket=/run/agent-sandbox.sock
#   - BPF LSM is active (see scripts/setup-vm.sh — usually requires a one-time reboot)
#
# What it asserts:
#   1.  IPC socket exists and the daemon answers DaemonStatus.
#   2.  The eight asb_* BPF programs are attached at the LSM/tracepoint hooks.
#   3.  blocked-net.yaml: the agent's connect() is denied at the kernel
#       (verdict:"deny" in the per-agent event log; agent stdout reports EPERM).
#   4.  allowed-net.yaml: same connect() succeeds (verdict:"allow") because
#       the host is in allowed_hosts.
#
# Run inside the VM:
#   sudo bash examples/test-it.sh
# (Open http://127.0.0.1:8765/ on your host to watch the dashboard live —
#  start the viewer with: bash viewer/scripts/start-viewer.sh)

set -uo pipefail

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; NC='\033[0m'
ok()   { printf "${GREEN}  ✔ %s${NC}\n" "$1"; }
fail() { printf "${RED}  ✘ %s${NC}\n" "$1"; FAILED=1; }
warn() { printf "${YELLOW}  ⚠ %s${NC}\n" "$1"; }

FAILED=0
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
SOCK="${AGENT_SANDBOX_SOCKET:-/run/agent-sandbox.sock}"
LOG_DIR="${AGENT_SANDBOX_LOG_DIR:-/var/log/agent-sandbox}"

# Resolve the agentctl binary: prefer the in-tree build, fall back to PATH.
AGENTCTL="${AGENTCTL:-$REPO/bin/agentctl}"
if [ ! -x "$AGENTCTL" ]; then
  AGENTCTL="$(command -v agentctl || true)"
fi
[ -x "$AGENTCTL" ] || { fail "agentctl not found — run 'make agentctl' first"; exit 1; }

echo "=== 1. daemon health (IPC socket + DaemonStatus RPC) ==="
if [ ! -S "$SOCK" ]; then
  fail "daemon socket $SOCK does not exist; is agentd running?"
  exit 1
fi
if sudo "$AGENTCTL" --socket="$SOCK" daemon status >/dev/null 2>&1; then
  ok "daemon answered DaemonStatus on $SOCK"
else
  fail "daemon did not respond on $SOCK"
  exit 1
fi

echo
echo "=== 2. eight asb_* BPF programs attached ==="
ATTACHED=$(sudo bpftool prog show 2>/dev/null | grep -c 'name asb_' || true)
if [ "$ATTACHED" -ge 8 ]; then
  ok "$ATTACHED eBPF programs attached"
else
  fail "expected ≥8 asb_* programs attached, found $ATTACHED"
fi

# run_agent MANIFEST:  starts the agent and prints the agent_id assigned by
# the daemon. Uses agentctl's JSON output so we don't have to grep the
# daemon's text log to learn the ID.
run_agent() {
  local manifest="$1"
  sudo "$AGENTCTL" --socket="$SOCK" --json run -f "$manifest" 2>/dev/null \
    | jq -r '.agent_id // empty'
}

# verdict_for AGENT_ID EVENT_TYPE:  prints the verdict ("allow"/"deny"/"audit")
# of the first matching event in the per-agent JSON log, or empty if no match.
verdict_for() {
  local agent_id="$1" type="$2"
  sudo cat "$LOG_DIR/${agent_id}.log" 2>/dev/null \
    | jq -r --arg t "$type" 'select(.type==$t) | .details.verdict' \
    | head -1
}

echo
echo "=== 3. blocked-net (manifest with no allowed_hosts; expect deny) ==="
AGENT=$(run_agent "$HERE/blocked-net.yaml")
sleep 1
if [ -z "$AGENT" ]; then
  fail "blocked-net: agentctl run did not return an agent_id"
else
  V=$(verdict_for "$AGENT" "net.connect")
  if [ "$V" = "deny" ]; then
    ok "kernel denied connect() (agent $AGENT, verdict=deny)"
  else
    fail "expected verdict=deny for blocked-net, got '${V:-<none>}' (agent $AGENT)"
  fi
fi

echo
echo "=== 4. allowed-net (1.1.1.1:80 in allowed_hosts; expect allow) ==="
AGENT=$(run_agent "$HERE/allowed-net.yaml")
sleep 1
if [ -z "$AGENT" ]; then
  fail "allowed-net: agentctl run did not return an agent_id"
else
  V=$(verdict_for "$AGENT" "net.connect")
  if [ "$V" = "allow" ]; then
    ok "kernel allowed connect() (agent $AGENT, verdict=allow)"
  else
    warn "expected verdict=allow for allowed-net, got '${V:-<none>}' (agent $AGENT) — check VM has outbound internet"
  fi
fi

echo
echo "=== 5. last 10 net.connect events across all agents ==="
sudo cat "$LOG_DIR"/*.log 2>/dev/null \
  | jq -r 'select(.type=="net.connect") | "  \(.details.verdict | ascii_upcase | .[0:5]) pid=\(.pid)  comm=\(.details.comm)  -> \(.details.daddr):\(.details.dport)"' \
  | tail -10

echo
if [ "$FAILED" -eq 0 ]; then
  ok "all assertions passed"
  exit 0
else
  fail "one or more assertions failed"
  exit 1
fi
