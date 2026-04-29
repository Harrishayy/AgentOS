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
command -v node >/dev/null 2>&1 || die "node not found on PATH — install Node 22+ first"
NODE_MAJOR="$(node -e 'process.stdout.write(process.versions.node.split(".")[0])')"
if [ "$NODE_MAJOR" -lt 22 ]; then
  die "Node $NODE_MAJOR detected — need Node 22+ (mock_kernel_sender.js uses built-in WebSocket)"
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

# exec replaces this shell so SIGINT / SIGTERM go straight to Node — no orphan
# child process if the user Ctrl-Cs.
cd "$SERVER_DIR"
export SERVE_STATIC=1
export PORT
exec node server.js
