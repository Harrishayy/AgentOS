#!/usr/bin/env bash
# P5 viewer — one-command startup.
#
# Brings the viewer up from a clean checkout:
#   1. confirm Node is recent enough (>=22, matches mock_kernel_sender.js)
#   2. install server/ + viewer-app/ deps if their node_modules is missing
#   3. build the React app (viewer-app/dist)
#   4. exec server.js with SERVE_STATIC=1 so HTTP and WebSocket share port 8765
#
# After this prints "starting on http://localhost:8765" the dashboard, the
# WebSocket relay, and the optional mock emitter all live in one Node process.
#
# Honours PORT=<n> from the caller's environment to override the default 8765.

set -euo pipefail

# Resolve the viewer/ root from this script's location so the script works
# regardless of the user's current directory.
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
VIEWER_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"
SERVER_DIR="$VIEWER_ROOT/server"
APP_DIR="$VIEWER_ROOT/viewer-app"

PORT="${PORT:-8765}"

say() { printf '[start-viewer] %s\n' "$*"; }
die() { printf '[start-viewer] error: %s\n' "$*" >&2; exit 1; }

# --- Node version check ----------------------------------------------------
# Production pieces (server.js, bridge.js) use the `ws` npm package and run on
# Node ≥ 20, which is what scripts/setup-vm.sh installs. The optional dev
# helper viewer/scripts/mock_kernel_sender.js needs Node 22+ for its built-in
# global WebSocket — we don't gate startup on that.
command -v node >/dev/null 2>&1 || die "node not found on PATH — install Node 20+ first"
NODE_MAJOR="$(node -e 'process.stdout.write(process.versions.node.split(".")[0])')"
if [ "$NODE_MAJOR" -lt 20 ]; then
  die "Node $NODE_MAJOR detected — need Node 20+"
fi
say "node $(node -v) OK"

command -v npm >/dev/null 2>&1 || die "npm not found on PATH"

# --- Install deps if needed ------------------------------------------------
if [ ! -d "$SERVER_DIR/node_modules" ]; then
  say "installing server deps (one-time)…"
  ( cd "$SERVER_DIR" && npm install --silent --no-audit --no-fund )
else
  say "server deps already installed"
fi

if [ ! -d "$APP_DIR/node_modules" ]; then
  say "installing viewer-app deps (one-time)…"
  ( cd "$APP_DIR" && npm install --silent --no-audit --no-fund )
else
  say "viewer-app deps already installed"
fi

# --- Build the React app ---------------------------------------------------
say "building viewer-app…"
( cd "$APP_DIR" && npm run build --silent )

# --- Launch ----------------------------------------------------------------
say "starting on http://localhost:${PORT}  (Ctrl-C to stop)"
say "open the URL above in your browser; mocks can connect on the same port"

cd "$SERVER_DIR"
export SERVE_STATIC=1
export PORT

# Optionally spawn the daemon→viewer bridge alongside the relay so the
# dashboard sees real kernel events. Skip with BRIDGE=0 (or simply don't
# run the daemon).
if [ "${BRIDGE:-1}" = "1" ]; then
  if [ -f "$SERVER_DIR/bridge.js" ]; then
    say "starting daemon→viewer bridge (daemon WS :7443 → viewer :${PORT})"
    node "$SERVER_DIR/bridge.js" &
    BRIDGE_PID=$!
    # Forward SIGINT / SIGTERM to the bridge so Ctrl-C tears down both processes.
    trap 'kill $BRIDGE_PID 2>/dev/null || true' INT TERM EXIT
  else
    say "bridge.js not found, skipping bridge"
  fi
fi

exec node server.js
