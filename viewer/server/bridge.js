#!/usr/bin/env node
// =============================================================================
// Agent Sandbox — daemon → viewer WebSocket bridge
//
// The daemon (P2) emits live kernel events on ws://127.0.0.1:7443/events.
// The viewer relay (P5) accepts connections on ws://127.0.0.1:8765 and
// expects each client to identify itself as either a "sender" (pushes events)
// or a "viewer" (receives them).
//
// Without something connecting both, the dashboard never sees real kernel
// events — only mock events or whatever the orchestrator (P4) chooses to
// push. This bridge fills that gap: it subscribes to the daemon and replays
// every frame to the viewer relay tagged as a sender.
//
// Usage:
//   node viewer/scripts/bridge.js
//
// Env vars (all optional):
//   DAEMON_WS    daemon event stream URL  (default ws://127.0.0.1:7443/events)
//   VIEWER_WS    viewer relay URL         (default ws://127.0.0.1:8765)
//   SENDER_NAME  human-readable label     (default "agentd-bridge")
// =============================================================================

const WebSocket = require('ws');

const DAEMON_WS  = process.env.DAEMON_WS  || 'ws://127.0.0.1:7443/events';
const VIEWER_WS  = process.env.VIEWER_WS  || 'ws://127.0.0.1:8765';
const SENDER     = process.env.SENDER_NAME || 'agentd-bridge';

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS  = 30_000;

function ts() { return new Date().toISOString(); }
function log(...a) { console.log(`[${ts()}] [bridge]`, ...a); }
function warn(...a) { console.warn(`[${ts()}] [bridge]`, ...a); }

// Connection state. Each side reconnects independently with exponential
// backoff; messages are dropped while the other side is down (events are
// observability, not durable). The dashboard catches up once both sides
// reconnect.
let daemonWS = null;
let viewerWS = null;
let viewerReady = false;
let daemonBackoff = RECONNECT_BASE_MS;
let viewerBackoff = RECONNECT_BASE_MS;

function connectViewer() {
  log(`viewer: dial ${VIEWER_WS}`);
  viewerReady = false;
  viewerWS = new WebSocket(VIEWER_WS);

  viewerWS.on('open', () => {
    log('viewer: connected; sending sender handshake');
    viewerWS.send(JSON.stringify({ role: 'sender', name: SENDER }));
    viewerReady = true;
    viewerBackoff = RECONNECT_BASE_MS;
  });
  viewerWS.on('close', (code) => {
    warn(`viewer: closed (code ${code}); reconnecting in ${viewerBackoff}ms`);
    viewerReady = false;
    setTimeout(connectViewer, viewerBackoff);
    viewerBackoff = Math.min(viewerBackoff * 2, RECONNECT_MAX_MS);
  });
  viewerWS.on('error', (err) => {
    warn(`viewer: error ${err.code || err.message}`);
    // 'close' will follow.
  });
}

function connectDaemon() {
  log(`daemon: dial ${DAEMON_WS}`);
  daemonWS = new WebSocket(DAEMON_WS);

  daemonWS.on('open', () => {
    log('daemon: connected; relaying events');
    daemonBackoff = RECONNECT_BASE_MS;
  });
  daemonWS.on('message', (data) => {
    if (!viewerReady || viewerWS.readyState !== WebSocket.OPEN) return;
    // Daemon sends one JSON event per frame; pass through verbatim.
    viewerWS.send(data);
  });
  daemonWS.on('close', (code) => {
    warn(`daemon: closed (code ${code}); reconnecting in ${daemonBackoff}ms`);
    setTimeout(connectDaemon, daemonBackoff);
    daemonBackoff = Math.min(daemonBackoff * 2, RECONNECT_MAX_MS);
  });
  daemonWS.on('error', (err) => {
    warn(`daemon: error ${err.code || err.message}`);
    // 'close' will follow.
  });
}

connectViewer();
connectDaemon();

// Graceful shutdown so systemd / Ctrl-C doesn't leave half-open sockets.
function shutdown() {
  log('shutting down');
  try { daemonWS && daemonWS.close(); } catch {}
  try { viewerWS && viewerWS.close(); } catch {}
  setTimeout(() => process.exit(0), 200);
}
process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);
