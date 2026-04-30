# P3 Week 3 — Interfaces (Canonical)

This is the canonical reference for the interfaces P3 owns or co-owns at end of Week 3. Other roles read this. Anything contradicting it in another doc is wrong; fix this file or fix the other doc.

Three interfaces in scope:

1. **Manifest v1** — the YAML format users write. P3 owns.
2. **Daemon protocol v1** — Unix socket request/response shapes. **Co-owned with P2**; P2 implements server, P3 implements client. P2's `daemon/api/proto.md` is the wire-level source of truth; this section mirrors it for P3's planning view.
3. **Event schema v1** — unified format for LLM and kernel events flowing through `agentctl logs`. **Co-owned with P4 (LLM events) and P5 (kernel-event renderer for the web UI)**.

---

## 1. Manifest v1

### 1.1 File shape

```yaml
# Required
name: <string>                  # DNS-label compatible, [a-z0-9-]{1,63}
command: [<string>, ...]        # argv; first element is the binary

# Required-but-may-be-empty
allowed_hosts: [<host-pattern>]
allowed_paths: [<path-pattern>]

# Optional
working_dir: <abs-path>         # default: /tmp/agentctl/<name>
env: {<KEY>: <VALUE>}           # default: {}
user: <string-or-uid>           # default: current user (numeric uid recommended)
stdin: <"inherit"|"close"|"file:<path>">  # default: "close"
timeout: <duration>             # default: "0" (no timeout)
description: <string>           # human-readable, ignored by enforcement
```

No other top-level keys are accepted. Unknown keys are an error (DEC-003). `restart_on_crash` and `max_restarts` are **not** manifest fields — they are CLI flags on `agentctl run` (DEC-012, OPEN-Q-012 resolution).

### 1.2 Field-to-enforcement-layer mapping

This table is the contract with P1 and P2. **Lock with P1 by Day 1 of Week 3.**

| Field | Layer | Enforcement mechanism | Notes |
|---|---|---|---|
| `name` | P3 | manifest validator + daemon registry | Must be unique among running agents |
| `command` | P2 | `clone3` + `execve` inside cgroup | argv directly; no shell expansion |
| `allowed_hosts` | P1 | eBPF `cgroup/connect4` + `connect6` policy map | Kernel does prefix/CIDR-range match against `cHostRule{AddrV4, PrefixLen, Port}`; hostnames are resolved to IPs in user space **at policy-load time** (one-shot inside `ParseHost`), not per-connection |
| `allowed_paths` | P1 | BPF LSM `file_open` hook against per-cgroup `cPathRule` map | Enforced in v1 (`buildC` writes `Paths[NPaths]` into the BPF map; `asb_file_open` enforces) |
| `working_dir` | P2 | `chdir` before `execve` | |
| `env` | P2 | `execve` envp | |
| `user` | P2 | `setresuid` + `setresgid` | |
| `stdin` | P2 | dup2 / open before `execve` | |
| `timeout` | P2 | daemon timer, sends SIGTERM then SIGKILL | |
| `description` | none | metadata only | Echoed by `agentctl list` |

**Note on hostname freshness.** Because hostnames resolve once at policy-load time, an agent's allowlist is pinned to the IPs that resolved when the agent started. Subsequent DNS changes do not affect the in-kernel allowlist. For long-running agents whose target endpoints rotate IPs (e.g., behind a CDN), prefer CIDR or wildcard hostnames (`*.example.com`) over single FQDNs.

### 1.3 Pattern grammars

**Host pattern** (v1):
- Literal hostname: `api.openai.com` (resolved once at policy-load time)
- Wildcard left-most label: `*.openai.com` matches one label only (`api.openai.com` ✓, `foo.bar.openai.com` ✗ in v1)
- IP literal: `203.0.113.5` (v4) or `2001:db8::1` (v6)
- **CIDR range:** `10.0.0.0/8`, `203.0.113.0/24`, `2001:db8::/32`. Kernel does prefix-range match via `cHostRule.PrefixLen`.
- Port suffix optional on any of the above: `api.openai.com:443`, `10.0.0.0/8:443`. If absent, all ports allowed (note this in `agentctl list` policy summary).

Validation: each pattern must parse as one of the above. Invalid CIDR mask widths (`10.0.0.0/33`) or non-network host bits (e.g., `10.0.0.5/8` — host bits set on a network address) are rejected with `invalid_host_pattern`.

**Path pattern** (v1, enforced via BPF LSM `file_open` hook):
- Absolute path: `/srv/data/input.json`
- Trailing-slash directory: `/srv/data/` (matches that dir and its contents)
- Single-`*` glob: `/srv/data/*.json`
- No `**`, no `?`, no character classes in v1.

### 1.4 Defaults

| Field | Default if absent |
|---|---|
| `working_dir` | `/tmp/agentctl/<name>` (daemon creates with mode 0700, owned by `user`) |
| `env` | `{}` (note: `PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin` is **not** auto-injected; agents that need PATH must set it) |
| `user` | the uid of the daemon caller, validated via SO_PEERCRED on the Unix socket |
| `stdin` | `"close"` |
| `timeout` | `"0"` (no timeout) |
| `description` | `""` |

### 1.5 Canonical example manifests

#### web-fetcher.yaml
```yaml
name: web-fetcher
command: ["/usr/bin/curl", "-fsSL", "https://api.openai.com/v1/models"]
allowed_hosts:
  - api.openai.com:443
allowed_paths: []
description: "Smoke test: one outbound request, blocked elsewhere."
```

#### file-reader.yaml
```yaml
name: file-reader
command: ["/usr/bin/cat", "/srv/agent-input/data.txt"]
allowed_hosts: []
allowed_paths:
  - /srv/agent-input/
working_dir: /srv/agent-input
description: "Reads a file, makes no network calls. Hosts list empty = deny-all egress."
```
Note: paths are enforced by P1's BPF LSM `file_open` hook against the per-cgroup `cPathRule` map. Reads outside `allowed_paths` fail with EACCES at the LSM layer. Whether path-deny events also surface as `kernel.*` log events in v1 is a P1 follow-up; v1's documented kernel-event types remain `kernel.connect_allowed`/`kernel.connect_blocked` only (§3.3).

#### shell-runner.yaml
```yaml
name: shell-runner
command: ["/bin/sh", "-c", "echo hello && date"]
allowed_hosts: []
allowed_paths: []
timeout: "30s"
description: "No network, no path policy, 30-second wall clock. Demonstrates timeout."
```

#### llm-agent.yaml
```yaml
name: llm-agent
command: ["/usr/bin/python3", "/opt/agents/demo.py"]
allowed_hosts:
  - api.anthropic.com:443
  - api.openai.com:443
allowed_paths: []
working_dir: /opt/agents
env:
  ANTHROPIC_API_KEY: "${ANTHROPIC_API_KEY}"
  PYTHONUNBUFFERED: "1"
user: "65534"
timeout: "5m"
description: "Demo LLM agent with two-LLM-provider egress allowlist; runs as nobody."
```

Env interpolation rule: `${VAR}` is resolved by `agentctl run` from the caller's environment **before** sending to the daemon. The daemon never sees `${...}` literals. If the variable is unset, `agentctl run` errors out at the manifest's line/column with:
```
llm-agent.yaml:11:24: env value '${ANTHROPIC_API_KEY}' references unset variable; export it or remove the entry
```

---

## 2. Daemon Protocol v1

**Source of truth.** P2's `daemon/api/proto.md` is the canonical wire-level spec. This section mirrors it for P3's planning view. If the two diverge, `proto.md` wins; raise an issue and update this file.

**Transport.** Unix domain socket at `/run/agent-sandbox.sock` (production, root-owned, mode 0600). Path is configurable via `AGENT_SANDBOX_SOCKET` env var or `--socket` flag on every subcommand. Discovery order in DEC-008.

**Wire format.** Length-prefixed JSON (DEC-011). Each frame is `[4-byte BE uint32: body_len][body_len bytes: UTF-8 JSON]`. `maxFrameBytes = 16 MiB`; receivers reject and close on oversize.

**Connection model.**
- One connection per CLI invocation.
- Unary methods (`RunAgent`, `StopAgent`, `ListAgents`, `AgentLogs`, `DaemonStatus`, `IngestEvent`): client writes one request frame, optionally `shutdown(SHUT_WR)` to signal end-of-request, server writes one response frame, server closes.
- Streaming method (`StreamEvents`): client writes one request frame, server writes zero-or-more response frames as events occur, until either side closes. **EOF is the stream terminator — no `{"type":"end"}` sentinel.** Per-event server write has a 1-second timeout; a stalled client gets dropped, not held.

**Authentication.** SO_PEERCRED on the Unix socket. The daemon trusts the kernel's peer-cred and uses the caller's uid as the agent's default `user`. `IngestEvent` adds an authz check (§2.8). No tokens.

### 2.1 Envelope

**Request frame body:**
```json
{"method": "RunAgent|StopAgent|ListAgents|AgentLogs|StreamEvents|DaemonStatus|IngestEvent",
 "params": { ... }}
```

**Success response frame body:**
```json
{"ok": true, "result": { ... }}
```

**Error response frame body:**
```json
{"ok": false, "error": {"code": "INVALID_MANIFEST|...", "message": "human-readable"}}
```

There is no `protocol` field, no `request_id`, no `command`/`payload` fields on the wire. The CLI's Go types may alias `method`↔`command` and `params`↔`payload` internally for readability — the wire stays as above.

**Versioning.** The daemon advertises its protocol version via `DaemonStatus.result.protocol_version`. The CLI does not send a version on the wire; it queries `DaemonStatus` once at startup if it needs to branch on version. P2's contract: wire-level method names are append-only, so a forward-compatible client tolerates new methods it doesn't call.

### 2.2 `RunAgent`

**Request params:**
```json
{
  "manifest": {
    "name": "web-fetcher",
    "command": ["/usr/bin/curl", "-fsSL", "https://api.openai.com/v1/models"],
    "allowed_hosts": ["api.openai.com:443"],
    "allowed_paths": [],
    "working_dir": "/tmp/agentctl/web-fetcher",
    "env": {},
    "user": "1000",
    "stdin": "close",
    "timeout_ns": 0,
    "description": ""
  },
  "manifest_source": {
    "path": "/home/aryan/web-fetcher.yaml",
    "sha256": "..."
  },
  "restart_on_crash": false,
  "max_restarts": 3
}
```

The CLI sends the **fully-resolved** manifest — defaults filled in, `${VAR}` substitutions done, `timeout` parsed to nanoseconds. The daemon does not re-parse YAML. `manifest_source` is metadata only, used for daemon-side audit logs. `restart_on_crash` and `max_restarts` (DEC-012) are operator-supplied flags; the daemon stores them on the agent record for the orchestrator to consult.

**Success result:**
```json
{
  "name": "web-fetcher",
  "agent_id": "agt_xxxx",
  "pid": 12345,
  "cgroup_path": "/sys/fs/cgroup/agent-sandbox/web-fetcher.scope",
  "started_at": "2026-04-27T11:42:13.123456Z",
  "policy_summary": "hosts:1 paths:0 timeout:0"
}
```

**Errors:** `INVALID_MANIFEST`, `LAUNCH_FAILED`, `CGROUP_FAILED`, `BPF_LOAD_FAILED`, `INTERNAL`.

### 2.3 `ListAgents`

**Request params:** `{}`

**Success result:**
```json
{
  "agents": [
    {
      "name": "web-fetcher",
      "agent_id": "agt_xxxx",
      "pid": 12345,
      "status": "running|exited|killed",
      "exit_code": null,
      "started_at": "2026-04-27T11:42:13.123456Z",
      "uptime_ns": 1234567890,
      "policy_summary": "hosts:1 paths:0 timeout:0"
    }
  ]
}
```

**Errors:** `INTERNAL`.

### 2.4 `StopAgent`

**Request params:**
```json
{"name": "web-fetcher", "grace_period_ns": 5000000000}
```

`grace_period_ns` defaults to 5 seconds; daemon sends SIGTERM, waits, then SIGKILL.

**Success result:**
```json
{
  "name": "web-fetcher",
  "exit_code": 0,
  "signal": "SIGTERM",
  "duration_ns": 412000000
}
```

**Errors:** `AGENT_NOT_FOUND`, `INTERNAL`.

### 2.5 `AgentLogs`

Request/response. Returns the last `tail_n` events from the per-agent log file at `/var/log/agent-sandbox/<agent-id>.log` (DEC-010).

**Request params:**
```json
{"name": "web-fetcher", "tail_n": 100}
```

`tail_n` is required; CLI default 100 if `--tail` is omitted. Maximum is bounded by file size (~30 MiB per agent due to rotation; older events lost beyond that).

**Success result:**
```json
{"events": [<Event>, <Event>, ...]}
```

`<Event>` shape per §3.

**Errors:** `AGENT_NOT_FOUND`, `INTERNAL`. There is no `since` parameter in v1; resumable replay (server-side `seq` numbering, client-side `--since=<seq>`) is v0.2.

### 2.6 `StreamEvents`

Persistent push. Server writes frames as events occur until either side closes. **No terminator sentinel — EOF terminates the stream.**

**Request params:**
```json
{"name": "web-fetcher", "include": ["llm", "kernel", "lifecycle"]}
```

`name` filters to one agent; if omitted, the stream covers all agents the caller can see. `include` filters event categories; if omitted, all categories pass.

**Response frames:** zero or more frames, each containing one event:
```json
{"ok": true, "result": {"event": <Event>}}
```

The 1-second per-frame server write timeout protects the daemon from a stalled client; stall → daemon closes the connection (drop-then-warn semantics for backpressure).

The CLI does **not** half-close after the request frame — the server may write replies indefinitely. Cancellation is by `Close()` from the client side (DEC-007 wires it through `cmd.Context()`).

**Errors:** `AGENT_NOT_FOUND` (only at subscribe time when `name` is set and unknown; mid-stream agent-not-found is not signalled), `INTERNAL`.

### 2.7 `DaemonStatus`

Request/response. Used for health check, version probe, and (later) capability discovery.

**Request params:** `{}`

**Success result:**
```json
{
  "protocol_version": "v1",
  "build": "agentd 0.1.0 (abc1234)",
  "uptime_ns": 9876543210,
  "agents_running": 3
}
```

**Errors:** `INTERNAL`.

### 2.8 `IngestEvent`

Write-side method used by the orchestrator (P4) to push LLM-level events into the daemon's pipeline. Events flow through `pipeline.Submit` → per-agent log file + `StreamEvents` subscribers + P5's WebSocket fan-out, so `agentctl logs` and the web UI see orchestrator events on the same channel as kernel events.

**Authorization.** SO_PEERCRED peer-uid must equal the daemon's own uid (orchestrator co-located default) **or** the uid named in `AGENT_SANDBOX_INGEST_UID` env (split-uid deployments). Anything else returns `PERMISSION_DENIED` (P3 vote per OPEN-Q-011 resolution).

**Request params:**
```json
{
  "agent_id": "agt_xxxx",
  "event": {
    "type": "llm.<subtype>",
    "ts": "2026-04-27T11:42:13.123456789Z",
    "details": { ... }
  }
}
```

- `agent_id` required; must exist in the registry. Unknown id → `AGENT_NOT_FOUND`.
- `event.type` MUST match `^llm\.` — server validates this. The `llm.*` prefix is a reserved namespace on `IngestEvent` so the orchestrator-emitted stream cannot collide with daemon-internal categories (`kernel.*`, `lifecycle.*`, `agent.*`). Type-prefix violation → `INVALID_MANIFEST`.
- `event.ts` is RFC 3339 nano; daemon trusts it by default but may overwrite if skewed beyond a sanity bound.
- `event.details` is P4-owned free-form per §3.2.

The daemon stamps `agent_id` and `category: "llm"` on the outgoing envelope before fan-out.

**Success result:** `{}`

**Errors:** `AGENT_NOT_FOUND`, `PERMISSION_DENIED`, `INVALID_MANIFEST` (malformed event payload OR `event.type` not `llm.`-prefixed — overloaded code per P2's existing set), `INTERNAL`. Backpressure: `pipeline.Submit` is non-blocking; on full buffer the daemon drops the event and emits a synthesised `lifecycle.dropped` warning to subscribers — the request itself still returns `{"ok": true, "result": {}}` (drop is observability, not an error).

### 2.9 Error codes

| Code | Meaning | Emitted by |
|---|---|---|
| `INVALID_MANIFEST` | Server-side manifest validation, or malformed `IngestEvent` payload | server |
| `AGENT_NOT_FOUND` | `StopAgent`/`AgentLogs`/`StreamEvents`/`IngestEvent` for an unknown agent | server |
| `CGROUP_FAILED` | cgroup creation or attachment failed | server |
| `BPF_LOAD_FAILED` | eBPF program load or map setup failed | server |
| `LAUNCH_FAILED` | clone3/execve failed | server |
| `PERMISSION_DENIED` | `IngestEvent` peer-uid mismatch | server |
| `INTERNAL` | unspecified server error | server |
| `daemon_unreachable` | Unix socket dial failed (client-synthesized; lowercase to mark client-side) | client |
| `manifest_parse_failed` | YAML parse / validation failed before send (client-synthesized) | client |

Client-synthesized errors use the same envelope shape so `--json` output is uniform.

---

## 3. Event Schema v1

Unified across LLM-level events (from P4's orchestrator, ingested via §2.8 `IngestEvent`) and kernel-level events (from P1's eBPF ring buffer, surfaced by P2). **Co-owned with P4 and P5 — locked Day 1.**

### 3.1 Common envelope

Every event:
```json
{
  "schema": "v1",
  "ts": "2026-04-27T11:42:13.123456789Z",
  "agent": "web-fetcher",
  "agent_id": "agt_xxxx",
  "category": "llm|kernel|lifecycle|agent",
  "type": "<see below>",
  "data": { ... }
}
```

`ts` is RFC 3339 with nanosecond precision. The daemon stamps `category` based on the source:
- `IngestEvent` → `category: "llm"`
- daemon's eBPF-event reader → `category: "kernel"`
- daemon's spawn / kill / wait4 bookkeeping → `category: "lifecycle"`
- daemon's subprocess fd 1 / fd 2 capture → `category: "agent"` (§3.5)

Event-stream consumers (CLI, P5 UI) route on `category`; no separate `source` field is needed on the wire.

### 3.2 LLM-level events (`category: "llm"`)

Source: P4's orchestrator via `IngestEvent`. All `type` values are `llm.*`. P4 owns the subtype set; this table reflects the locked Day-1 view.

| `type` | `data` shape | Meaning |
|---|---|---|
| `llm.stdout` | `{"line":"..."}` | One line of agent stdout (everything that doesn't match a tool prefix) |
| `llm.tool_call` | `{"tool":"<name>","args":{...},"raw":"[TOOL] ...","latency_ms":<n>?,"tokens_in":<n>?,"tokens_out":<n>?}` | Detected when stdout line starts with `[TOOL]`. `latency_ms`/`tokens_in`/`tokens_out` are emitted when the orchestrator can extract them from the line; null otherwise. |
| `llm.tool_result` | `{"tool":"<name>","ok":true|false,"raw":"[RESULT] ...","latency_ms":<n>?,"tokens_in":<n>?,"tokens_out":<n>?}` | Detected when stdout line starts with `[RESULT]`. |
| `llm.stopped` | `{"exit_code":0}` | Orchestrator observed the agent exit successfully |
| `llm.crashed` | `{"exit_code":<n>}` | Orchestrator observed the agent exit non-zero |

Note: `llm.stopped`/`llm.crashed` are the orchestrator's narrative-level exit signals. The daemon also emits `lifecycle.exited` (§3.4) on the same exit observed via `wait4`. Both fire; the CLI displays both at their respective abstraction levels. P5's UI may suppress the duplicate by `agent_id`+`ts` proximity if desired. (Open follow-up: confirm `[RESULT]` is actually emitted by `demo_agent.py`; if not, drop `llm.tool_result`.)

### 3.3 Kernel-level events (`category: "kernel"`)

P1 emits these via eBPF ring buffer; P2 reads and forwards. All `type` values are `kernel.*`.

| `type` | `data` shape | Meaning |
|---|---|---|
| `kernel.connect_allowed` | `{"family":"ipv4","dst":"203.0.113.5","port":443,"hostname":"api.openai.com"}` | Outbound connection permitted |
| `kernel.connect_blocked` | `{"family":"ipv4","dst":"203.0.113.5","port":443,"reason":"not_in_allowlist"}` | Outbound connection blocked |

Kernel events do not always carry hostnames — DNS resolution happens before connect. The daemon attempts a reverse lookup from its in-memory cache (populated when it programs the eBPF map); if unknown, `hostname` is omitted.

### 3.4 Lifecycle events (`category: "lifecycle"`)

Daemon-emitted, observing process state directly via `wait4` and timer-driven kills.

| `type` | `data` shape | Meaning |
|---|---|---|
| `lifecycle.started` | `{"pid":12345}` | Daemon clone3+cgroup+execve completed |
| `lifecycle.exited` | `{"exit_code":<n>}` | Daemon's `wait4` returned (any exit code) |
| `lifecycle.killed` | `{"signal":"SIGKILL","reason":"timeout|stop_request"}` | Daemon SIGTERM'd or SIGKILL'd the agent |
| `lifecycle.dropped` | `{"count":<n>,"window_ms":<n>}` | Synthesised: daemon dropped events due to subscriber backpressure |

There is no `end` sentinel — `StreamEvents` terminates by EOF (DEC-011, §2.6).

### 3.5 Agent stdio events (`category: "agent"`)

Daemon-emitted, captured directly from the agent subprocess's fd 1 / fd 2. These are the **raw** bytes — distinct from `llm.stdout` (§3.2), which is the orchestrator's parsed view after tool-prefix detection. Both fire for the same byte stream; consumers pick whichever abstraction they want.

| `type` | `data` shape | Meaning |
|---|---|---|
| `agent.stdout` | `{"line":"...","truncated":<bool>}` | One line from the agent's fd 1 |
| `agent.stderr` | `{"line":"...","truncated":<bool>}` | One line from the agent's fd 2 |

- Each line is capped at **8 KiB**; longer lines are split with `truncated: true` on the chunk that was clipped.
- Backpressure is **drop-on-full** through `pipeline.Submit`; drops surface as a synthesised `lifecycle.dropped` event, not as an `agent.*` gap.
- **Orchestrator contract.** P4 must NOT parse `agent.stdout` for tool-call detection. Tool-call detection happens orchestrator-side and is published structurally via `IngestEvent` → `llm.tool_call` / `llm.tool_result`. Treating `agent.stdout` as a parseable channel would race with the orchestrator and produce duplicate tool events.

CLI rendering: `agentctl logs --include=agent` shows the raw stream; default `--include` excludes `agent` (the LLM-level view is usually what users want). P5 UI: optional "raw stdio" panel.

### 3.6 P5 contract

P5's web UI consumes the same events from the daemon over a WebSocket bridge that P5/P2 jointly define. The bridge re-encodes the length-prefixed frames as NDJSON over WebSocket (browser WebSocket APIs are line-oriented). The schema above is the only schema; P5 must not require a different shape. P5 routes events into UI panels by `category` (`llm` → LLM column, `kernel` → kernel column, `lifecycle` → status banner). If P5 needs richer kernel data (e.g. process tree), that's a v2 conversation, not Week 3.
