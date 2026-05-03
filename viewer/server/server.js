// P5 viewer — WebSocket relay + (optionally) static-dashboard server
//
// Sits between event senders (P2 daemon, P4 orchestrator) and event viewers
// (the React dashboard in the browser). Does not generate real events itself;
// it just forwards JSON messages from senders to all connected viewers.
//
// Each new client must send a handshake within HANDSHAKE_TIMEOUT_MS:
//   { "role": "sender", "name": "p4-orchestrator" }   // pushes events in
//   { "role": "viewer" }                               // receives events out
// If no handshake arrives in time, the client is classified as a viewer.
//
// Run modes:
//   node server.js                  # relay only (HTTP root prints a hint)
//   MOCK_EVENTS=1 node server.js    # also emit fake events every 2s
//   SERVE_STATIC=1 node server.js   # also serve viewer-app/dist on same port
//                                   #   (this is what start-viewer.sh uses)

const http = require('http');
const fs = require('fs');
const path = require('path');
const { WebSocketServer, WebSocket } = require('ws');

const PORT = Number(process.env.PORT) || 8765;
const HANDSHAKE_TIMEOUT_MS = 3000;
const MOCK_INTERVAL_MS = 2000;
const MOCK_ENABLED = process.env.MOCK_EVENTS === '1';
const SERVE_STATIC = process.env.SERVE_STATIC === '1';

// dist/ lives next to viewer-app/, which lives next to server/.
const DIST_ROOT = path.resolve(__dirname, '..', 'viewer-app', 'dist');

const senders = new Set();
const viewers = new Set();

let nextClientId = 1;

const recentEventsBuffer = [];
const BUFFER_MAX = 20;

function getRecentEvents() {
  return recentEventsBuffer.slice();
}

function ts() {
  return new Date().toISOString();
}

function log(...args) {
  console.log(`[${ts()}]`, ...args);
}

function warn(...args) {
  console.warn(`[${ts()}]`, ...args);
}

function describe(client) {
  const role = client.role || 'unknown';
  const name = client.name ? ` "${client.name}"` : '';
  return `#${client.id} ${role}${name}`;
}

function broadcastToViewers(rawJson, fromClient) {
  try {
    const parsed = JSON.parse(rawJson);
    if (parsed && typeof parsed === 'object') {
      recentEventsBuffer.push(parsed);
      if (recentEventsBuffer.length > BUFFER_MAX) recentEventsBuffer.shift();
    }
  } catch {
    // malformed JSON — skip buffering, still relay
  }

  if (viewers.size === 0) return;
  let delivered = 0;
  for (const viewer of viewers) {
    if (viewer.ws.readyState === WebSocket.OPEN) {
      viewer.ws.send(rawJson);
      delivered += 1;
    }
  }
  if (fromClient) log(`relayed event from ${describe(fromClient)} → ${delivered} viewer(s)`);
}

function handleHandshake(client, msg) {
  const role = msg && msg.role;
  if (role === 'sender') {
    client.role = 'sender';
    client.name = typeof msg.name === 'string' ? msg.name : 'unnamed-sender';
    senders.add(client);
    log(`handshake: ${describe(client)} registered (${senders.size} sender(s) total)`);
  } else if (role === 'viewer') {
    client.role = 'viewer';
    viewers.add(client);
    log(`handshake: ${describe(client)} registered (${viewers.size} viewer(s) total)`);
  } else {
    warn(`handshake: ${describe(client)} sent unknown role "${role}", defaulting to viewer`);
    client.role = 'viewer';
    viewers.add(client);
  }
}

function defaultToViewer(client) {
  if (client.role) return;
  client.role = 'viewer';
  viewers.add(client);
  warn(
    `handshake timeout after ${HANDSHAKE_TIMEOUT_MS}ms for ${describe(client)}, ` +
      `defaulting to viewer (${viewers.size} viewer(s) total)`
  );
}

function removeClient(client) {
  if (client.role === 'sender') senders.delete(client);
  else if (client.role === 'viewer') viewers.delete(client);
}

// ---------------------------------------------------------------------------
// Static-file handler — serves viewer-app/dist/ when SERVE_STATIC=1.
// Hand-rolled to avoid pulling in serve-static / express. Vite emits a tiny
// set of file types so the content-type table stays small.
// ---------------------------------------------------------------------------
const CONTENT_TYPES = {
  '.html':  'text/html; charset=utf-8',
  '.js':    'application/javascript; charset=utf-8',
  '.mjs':   'application/javascript; charset=utf-8',
  '.css':   'text/css; charset=utf-8',
  '.json':  'application/json; charset=utf-8',
  '.map':   'application/json; charset=utf-8',
  '.svg':   'image/svg+xml',
  '.png':   'image/png',
  '.jpg':   'image/jpeg',
  '.jpeg':  'image/jpeg',
  '.ico':   'image/x-icon',
  '.woff':  'font/woff',
  '.woff2': 'font/woff2',
  '.ttf':   'font/ttf',
  '.txt':   'text/plain; charset=utf-8',
};

function contentTypeFor(filePath) {
  const ext = path.extname(filePath).toLowerCase();
  return CONTENT_TYPES[ext] || 'application/octet-stream';
}

function sendPlain(res, status, body) {
  res.writeHead(status, { 'Content-Type': 'text/plain; charset=utf-8' });
  res.end(body);
}

function sendStaticFile(req, res, filePath) {
  fs.stat(filePath, (statErr, stats) => {
    if (statErr || !stats.isFile()) {
      sendPlain(res, 404, `not found: ${req.url}\n`);
      return;
    }
    const headers = {
      'Content-Type': contentTypeFor(filePath),
      'Content-Length': stats.size,
    };
    // Vite emits hashed filenames in /assets, so they're safe to cache hard.
    // index.html must always be re-fetched so a rebuild is picked up.
    if (filePath.includes(`${path.sep}assets${path.sep}`)) {
      headers['Cache-Control'] = 'public, max-age=31536000, immutable';
    } else {
      headers['Cache-Control'] = 'no-cache';
    }
    res.writeHead(200, headers);
    fs.createReadStream(filePath).pipe(res);
  });
}

function handleHttpRequest(req, res) {
  if (req.method !== 'GET' && req.method !== 'HEAD') {
    sendPlain(res, 405, 'method not allowed\n');
    return;
  }

  if (!SERVE_STATIC) {
    // Relay-only mode — just answer with a friendly hint so curl/wget on the
    // port get something readable instead of a hang.
    sendPlain(
      res,
      200,
      'P5 viewer relay (WebSocket only on this port).\n' +
        'Set SERVE_STATIC=1 — or run viewer/scripts/start-viewer.sh — to also serve the dashboard here.\n',
    );
    return;
  }

  // Strip query string + decode the URL-encoded path; default `/` to index.html.
  let urlPath;
  try {
    urlPath = decodeURIComponent((req.url || '/').split('?')[0]);
  } catch {
    sendPlain(res, 400, 'bad request\n');
    return;
  }
  if (urlPath === '/' || urlPath === '') urlPath = '/index.html';

  // Resolve and confirm the result is inside DIST_ROOT — refuse traversal.
  const resolved = path.resolve(DIST_ROOT, '.' + urlPath);
  if (resolved !== DIST_ROOT && !resolved.startsWith(DIST_ROOT + path.sep)) {
    sendPlain(res, 403, 'forbidden\n');
    return;
  }

  // If dist hasn't been built, fail loud rather than silently 404 every asset.
  fs.access(DIST_ROOT, fs.constants.R_OK, (distErr) => {
    if (distErr) {
      sendPlain(
        res,
        503,
        'dashboard build not found (viewer/viewer-app/dist).\n' +
          'Run viewer/scripts/start-viewer.sh to build and serve.\n',
      );
      return;
    }
    sendStaticFile(req, res, resolved);
  });
}

// ---------------------------------------------------------------------------
// HTTP + WebSocket server — share one TCP port. ws attaches via { server }
// and intercepts only Upgrade requests, leaving plain GETs to handleHttpRequest.
// ---------------------------------------------------------------------------
const httpServer = http.createServer(handleHttpRequest);
const wss = new WebSocketServer({ server: httpServer });

httpServer.on('listening', () => {
  log(`WebSocket relay listening on ws://localhost:${PORT}`);
  if (SERVE_STATIC) {
    log(`serving dashboard at http://localhost:${PORT}  (dist: ${DIST_ROOT})`);
  } else {
    log('static dashboard: disabled (set SERVE_STATIC=1 to enable)');
  }
  log(`mock events: ${MOCK_ENABLED ? 'ENABLED (every ' + MOCK_INTERVAL_MS + 'ms)' : 'disabled'}`);
});

httpServer.on('error', (err) => {
  warn('server error:', err.message);
});

wss.on('error', (err) => {
  warn('ws server error:', err.message);
});

wss.on('connection', (ws, req) => {
  const client = {
    id: nextClientId++,
    ws,
    role: null,
    name: null,
    remote: req.socket.remoteAddress,
  };

  log(`connection opened: ${describe(client)} from ${client.remote}`);

  const handshakeTimer = setTimeout(() => defaultToViewer(client), HANDSHAKE_TIMEOUT_MS);

  ws.on('message', (raw) => {
    const text = raw.toString();

    // First message must be the handshake.
    if (!client.role) {
      clearTimeout(handshakeTimer);
      let parsed;
      try {
        parsed = JSON.parse(text);
      } catch (err) {
        warn(`bad handshake JSON from ${describe(client)}: ${err.message} — defaulting to viewer`);
        client.role = 'viewer';
        viewers.add(client);
        return;
      }
      handleHandshake(client, parsed);
      return;
    }

    // After handshake: only senders push events; viewers shouldn't be sending.
    if (client.role === 'viewer') {
      warn(`ignoring message from viewer ${describe(client)} (viewers are read-only)`);
      return;
    }

    // Validate the JSON before relaying so we don't poison the viewer feed.
    let event;
    try {
      event = JSON.parse(text);
    } catch (err) {
      warn(`bad event JSON from ${describe(client)}: ${err.message} — dropping`);
      return;
    }
    if (!event || typeof event !== 'object' || typeof event.type !== 'string') {
      warn(`event from ${describe(client)} missing 'type' field — dropping`);
      return;
    }

    broadcastToViewers(text, client);
  });

  ws.on('close', (code, reason) => {
    clearTimeout(handshakeTimer);
    removeClient(client);
    const reasonStr = reason && reason.length ? ` reason="${reason.toString()}"` : '';
    log(
      `connection closed: ${describe(client)} code=${code}${reasonStr} ` +
        `(${senders.size} sender(s), ${viewers.size} viewer(s) remaining)`
    );
  });

  ws.on('error', (err) => {
    warn(`socket error on ${describe(client)}: ${err.message}`);
  });
});

httpServer.listen(PORT);

// Optional mock event emitter so the pipeline can be tested without P2/P4.
// Acts like an internal sender — fabricates events and broadcasts them to viewers.
function startMockEmitter() {
  const llmSamples = [
    { type: 'stdout', data: { line: 'agent: thinking about the task...' } },
    { type: 'tool_call', data: { tool: 'fetch_url', args: { url: 'https://example.com' } } },
    { type: 'tool_call', data: { tool: 'fetch_url', args: { url: 'https://evil.com/exfil' } } },
    { type: 'stopped', data: { exit_code: 0 } },
  ];
  const kernelSamples = [
    {
      type: 'connect_attempt',
      data: { dst_ip: '93.184.216.34', dst_port: 443, hostname: 'example.com' },
    },
    {
      type: 'connect_allowed',
      data: {
        dst_ip: '93.184.216.34',
        dst_port: 443,
        hostname: 'example.com',
        reason: 'in allowed_hosts',
      },
    },
    {
      type: 'connect_blocked',
      data: {
        dst_ip: '203.0.113.42',
        dst_port: 80,
        hostname: 'evil.com',
        reason: 'no policy match',
      },
    },
  ];

  const fakeClient = { id: 0, role: 'sender', name: 'mock-emitter', ws: null };
  let i = 0;

  setInterval(() => {
    const useLlm = i % 2 === 0;
    const pool = useLlm ? llmSamples : kernelSamples;
    const sample = pool[Math.floor(Math.random() * pool.length)];
    const event = {
      agent: 'demo-agent',
      type: sample.type,
      ts: Date.now() / 1000,
      data: sample.data,
    };
    broadcastToViewers(JSON.stringify(event), fakeClient);
    i += 1;
  }, MOCK_INTERVAL_MS);
}

if (MOCK_ENABLED) startMockEmitter();

const { startAnalyser } = require('./analyser');
startAnalyser(getRecentEvents, (rawJson) => broadcastToViewers(rawJson, null));

function shutdown(signal) {
  log(`received ${signal}, closing server...`);
  wss.close();
  httpServer.close(() => {
    log('server closed, goodbye');
    process.exit(0);
  });
  // Hard exit if clients block close.
  setTimeout(() => process.exit(0), 1500).unref();
}

process.on('SIGINT', () => shutdown('SIGINT'));
process.on('SIGTERM', () => shutdown('SIGTERM'));
