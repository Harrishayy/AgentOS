// P5 viewer — WebSocket relay server
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
// Run:
//   node server.js                  # relay only
//   MOCK_EVENTS=1 node server.js    # also emit fake events every 2s for testing

const { WebSocketServer, WebSocket } = require('ws');

const PORT = Number(process.env.PORT) || 8765;
const HANDSHAKE_TIMEOUT_MS = 3000;
const MOCK_INTERVAL_MS = 2000;
const MOCK_ENABLED = process.env.MOCK_EVENTS === '1';

const senders = new Set();
const viewers = new Set();

let nextClientId = 1;

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
  if (viewers.size === 0) return;
  let delivered = 0;
  for (const viewer of viewers) {
    if (viewer.ws.readyState === WebSocket.OPEN) {
      viewer.ws.send(rawJson);
      delivered += 1;
    }
  }
  log(`relayed event from ${describe(fromClient)} → ${delivered} viewer(s)`);
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

const wss = new WebSocketServer({ port: PORT });

wss.on('listening', () => {
  log(`WebSocket relay listening on ws://localhost:${PORT}`);
  log(`mock events: ${MOCK_ENABLED ? 'ENABLED (every ' + MOCK_INTERVAL_MS + 'ms)' : 'disabled'}`);
});

wss.on('error', (err) => {
  warn('server error:', err.message);
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

function shutdown(signal) {
  log(`received ${signal}, closing server...`);
  wss.close(() => {
    log('server closed, goodbye');
    process.exit(0);
  });
  // Hard exit if clients block close.
  setTimeout(() => process.exit(0), 1500).unref();
}

process.on('SIGINT', () => shutdown('SIGINT'));
process.on('SIGTERM', () => shutdown('SIGTERM'));
