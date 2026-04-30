# P4 — Orchestrator & Demo

What this component is, how to run it, and what each teammate needs to know.

---

## What P4 owns

- **Orchestrator** — manages agent lifecycle (launch, monitor, restart, stop)
- **Demo** — the prompt-injection attack scenario used in the final video
- **Event bridge** — parses LLM-level events and forwards them to P5's process viewer

---

## How to run the demo (right now)

Requires: `OPENAI_API_KEY` set (or in a `.env` file), `ngrok` running on port 8888.

```bash
# Terminal 1
python evil_server.py

# Terminal 2
ngrok http 8888

# Terminal 3
python demo_launcher.py https://<your-ngrok-url>
```

**Success signal:** two `[TOOL] fetch_url called with:` lines. The second URL is the unauthorized call the injection caused. That's the call the eBPF sandbox will block in the "after" demo.

See `before-findings.md` for full baseline results and what injection techniques were tested.

---

## Orchestrator — how it works

```
demo_launcher.py  (or agentctl, or your code)
        │
        ▼
  Orchestrator          ← core.py
  ├── DaemonClient      ← daemon.py  (talks to P2's daemon, or stubs when absent)
  ├── EventStreamer     ← events.py  (sends LLM events to P5's WebSocket)
  └── AgentProcess[]   ← process.py (one per running agent)
```

### Two modes

**Stub mode (now):** daemon socket absent → `AgentProcess` spawns via `Popen`, reads stdout, parses `[TOOL]` lines, emits events to P5 directly over WebSocket.

**Daemon mode (Week 3):** daemon socket present → `AgentProcess` calls `DaemonClient.run_agent()`, gets back an `agent_id`. No `Popen`. Orchestrator wraps the model loop and calls `DaemonClient.ingest_event()` for each LLM event. Daemon fans everything out to P5.

Both modes share the same `Orchestrator` API — the switch is automatic based on whether `/run/agent-sandbox.sock` (or `$AGENT_SANDBOX_SOCKET`) is reachable.

### Launching an agent

```python
from orchestrator import Orchestrator

orch = Orchestrator(ws_url="ws://localhost:8765")   # ws_url optional
orch.launch("demo_agent.yaml")                       # or launch_direct(manifest)
orch.wait_for("demo-agent")
orch.stop_all()
```

Restart policy lives on the `Orchestrator`, not in the manifest:

```python
orch = Orchestrator(restart_on_crash=True, max_restarts=3)
```

---

## Manifest format

```yaml
name: demo-agent
command: ["python3", "demo_agent.py"]
allowed_hosts:                        # required — [] means deny all egress
  - llm-proxy.dev.outshift.ai
allowed_paths: []                     # required — non-empty rejected until P1 ships path enforcement
env:                                  # optional
  SOME_VAR: value
mode: enforce                         # optional — "enforce" | "audit", default enforce
```

`allowed_hosts` accepts hostnames, `*.wildcard` (single label), and IP literals with optional `:port`. No CIDR in v1.

`restart_on_crash` and `max_restarts` are **not** manifest fields — they're orchestrator config (see above).

---

## Events emitted to P5

Orchestrator is a **WebSocket client**. P5 owns the server at `ws://localhost:8765`.

In stub mode, events go directly over that socket. In daemon mode, they go via `IngestEvent` RPC to P2's daemon, which fans them out to P5 — orchestrator does not connect to P5's WebSocket at all.

### Envelope (NDJSON)

```json
{"agent": "demo-agent", "type": "tool_call", "ts": 1714000000.123, "data": {...}}
```

### Event types

| type | when | `data` |
|---|---|---|
| `stdout` | every line of agent output | `{"line": "..."}` |
| `tool_call` | agent calls a tool | `{"raw": "...", "tool": "fetch_url", "args": {"url": "..."}}` |
| `stopped` | agent exits 0 | `{"exit_code": 0}` |
| `crashed` | agent exits non-zero | `{"exit_code": N}` |

In daemon mode, the equivalent `llm.*` types go via `IngestEvent` — see INTERFACES §3.2.

---

## For P2

Socket path: `/run/agent-sandbox.sock` (or `$AGENT_SANDBOX_SOCKET`). We implement:
- `RunAgent` — launch an agent, returns `agent_id`
- `StopAgent` — stop by `agent_id`
- `ListAgents` — snapshot
- `IngestEvent` — push `llm.*` events into the unified pipeline

We do **not** `Popen` in daemon mode. We do **not** track PIDs — only `agent_id`.

Open: waiting on `agent.stdout` / `agent.stderr` stream events and the PR for `IngestEvent` + `agent.stdout/stderr` in `daemon/api/proto.md`.

## For P3

We talk directly to the daemon — we do not shell out to `agentctl`. The orchestrator and `agentctl` are parallel clients of the same socket.

Socket conflict now resolved: `/run/agent-sandbox.sock` is the agreed default (DEC-011).

## For P5

We're a WebSocket **client** in stub mode, daemon-relayed in daemon mode. You own the server.

Structured `tool_call` data: `{"raw": "...", "tool": "<name>", "args": {...}}` — `args` is a parsed object, not a raw string. `call_id` is included for `llm.tool_call` / `llm.tool_result` correlation in daemon mode.

If the WebSocket is unreachable at launch we log and continue — no crash.

---

## Current status

| | |
|---|---|
| Demo attack ("before") | Done — reproducible, documented in `before-findings.md` |
| Orchestrator stub mode | Done |
| Daemon protocol wired | Done — stubs cleanly when P2's daemon isn't up |
| `llm.*` event schema | Drafted, sent to P2 for INTERFACES §3.2 |
| Demo "after" (eBPF blocks call) | Blocked on P1 + P2 |
| Daemon mode model loop wrapper | Week 3 |
