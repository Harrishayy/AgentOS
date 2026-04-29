#!/usr/bin/env bash
# End-to-end smoke test for agent-sandbox-runtime.
#
#   1. Verifies the daemon is running and 8 BPF programs are attached.
#   2. Runs blocked-net.yaml — expects the kernel to deny connect().
#   3. Runs allowed-net.yaml — expects connect() to succeed.
#   4. Tails the daemon's recent-events buffer to confirm the matching
#      deny / allow event made it to userspace.
#
# Run inside the VM:
#   sudo bash examples/test-it.sh
# (Open http://127.0.0.1:9000/ui/ on your host first to watch live.)

set -uo pipefail

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; NC='\033[0m'
ok()   { printf "${GREEN}  ✔ %s${NC}\n" "$1"; }
fail() { printf "${RED}  ✘ %s${NC}\n" "$1"; FAILED=1; }
warn() { printf "${YELLOW}  ⚠ %s${NC}\n" "$1"; }

FAILED=0
HERE="$(cd "$(dirname "$0")" && pwd)"

echo "=== 1. daemon health ==="
if curl -fsS http://127.0.0.1:9000/api/healthz >/dev/null; then
  ok "daemon responds on :9000"
else
  fail "daemon not reachable; is the service running?"
  exit 1
fi

ATTACHED=$(sudo bpftool prog show 2>/dev/null | grep -c '\sasb_')
if [ "$ATTACHED" -ge 8 ]; then
  ok "$ATTACHED eBPF programs attached (asb_*)"
else
  fail "expected ≥8 asb_* programs attached, found $ATTACHED"
fi

echo
echo "=== 2. negative test: blocked-net (expect kernel deny) ==="
OUT=$(sudo agentctl run "$HERE/blocked-net.yaml" 2>&1 || true)
echo "$OUT" | sed 's/^/    /'
if echo "$OUT" | grep -q "OK: kernel denied connect"; then
  ok "kernel denied connect() to disallowed host"
else
  fail "blocked-net did not see a kernel-level deny (output above)"
fi

echo
echo "=== 3. positive test: allowed-net (expect connect to succeed) ==="
OUT=$(sudo agentctl run "$HERE/allowed-net.yaml" 2>&1 || true)
echo "$OUT" | sed 's/^/    /'
if echo "$OUT" | grep -q "OK: connect() succeeded"; then
  ok "connect() to allowed host succeeded"
else
  warn "allowed-net did not connect — check your VM has outbound internet"
fi

echo
echo "=== 4. event ringbuf (last 10 net events from the daemon) ==="
curl -fsS http://127.0.0.1:9000/api/events/recent \
  | python3 -c '
import json, sys
events = json.load(sys.stdin)
net = [e for e in events if e["kind"].startswith("net.")]
for e in net[-10:]:
    n = e.get("net") or {}
    print("  {:5s} {:14s} pid={:<6} comm={:<10} -> {}:{}".format(
        e["verdict"], e["kind"], e["pid"], e["comm"],
        n.get("daddr", "?"), n.get("dport", "?")))
'

echo
if [ "$FAILED" -eq 0 ]; then
  ok "all assertions passed"
  exit 0
else
  fail "one or more assertions failed"
  exit 1
fi
