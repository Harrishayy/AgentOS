# P3 Week 3 — Open Questions

Two sections: **Closed** (Day-1 resolutions, kept for audit) and **Open** (still pending, sorted by recipient so each block can be copy-pasted and sent).

---

## Closed (Day 1)

Cross-team:

- **OPEN-Q-006** — *event schema (P4 + P5)*. Closed. P4 confirmed five LLM types: `llm.stdout`, `llm.tool_call`, `llm.tool_result`, `llm.stopped`, `llm.crashed`, with optional `latency_ms`/`tokens_in`/`tokens_out` on tool events. INTERFACES §3.2 updated. P5 routes by `category` — no `source` field needed on the wire.
- **OPEN-Q-007** — *supported manifest fields (P1)*. Closed. P1 confirmed both fields enforced in v1, with two amendments: (a) kernel does prefix/CIDR-range match against `cHostRule{AddrV4, PrefixLen, Port}`, not exact-match; hostnames are resolved at **policy-load** time, not per-connection. (b) `allowed_paths` is **fully enforced** via BPF LSM `file_open` against a per-cgroup `cPathRule` map — **not deferred**. INTERFACES §1.2/§1.3 updated; the `unsupported_field_value` rejection rule is removed (no v1 field triggers it).
- **OPEN-Q-008** — *unknown event types (P2)*. Closed. Pass-through with original `type`; CLI renders unknown types as a generic line. P2 will annotate `proto.md` so future-them doesn't tighten this into a closed enum.
- **OPEN-Q-010** — *CIDR support for `allowed_hosts` (P1)*. Closed. CIDR is **in v1**, not backlog. `ParseHost` already parses `/N` notation and the kernel does prefix-range matching. Validator now accepts CIDR (`10.0.0.0/8`, `2001:db8::/32`); rejects only invalid masks (`/33`) or non-network host bits. P4's demo manifests can use CIDR directly.
- **OPEN-Q-011** — *orchestrator → daemon LLM-event ingestion (P2 + P4)*. Closed. P2 will add `IngestEvent` on the existing socket (SO_PEERCRED authz: peer uid = daemon uid OR `AGENT_SANDBOX_INGEST_UID`; `event.type` must be `llm.`-prefixed and is server-validated, type-prefix violation → `INVALID_MANIFEST`; authz fail → `PERMISSION_DENIED`). INTERFACES §2.8 captures the full shape. P4's WebSocket-direct alternative declined (would bypass the unified `agentctl logs` view).
- **OPEN-Q-012** — *restart policy (P4)*. Closed. Stays in P4's orchestrator state machine (`_monitor_loop`); not a manifest field. CLI surfaces `--restart-on-crash` and `--max-restarts` flags on `agentctl run`, passed through `RunAgent.params` as `restart_on_crash`/`max_restarts` (DEC-012).
- **OPEN-Q-013** — *required-vs-optional manifest fields (P4)*. Closed. Keep `allowed_hosts`/`allowed_paths` as required-may-be-empty. P4 agreed.
- **OPEN-Q-014** — *AIOS integration (P4)*. Closed. Path (b) — custom Python orchestrator (~250 lines, already in tree). AIOS rejected because its agents share an address space (single PID), incompatible with per-agent cgroup isolation.
- **OPEN-Q-005** — *daemon protocol (P2)*. Closed. P2 confirmed parity on all seven deltas (length-prefixed framing, `{method,params}` envelope, six methods + `IngestEvent`, `tail_n`-only replay, unary half-close, error codes incl. `PERMISSION_DENIED`, `/run/agent-sandbox.sock` 0600). P2 endorsed `PERMISSION_DENIED` over overloading `INVALID_MANIFEST` for `IngestEvent` authz (no manifest in scope; reusable for future authz checks; matches POSIX `EPERM` intuition). Two flags raised by P2 and resolved: (a) wire method names use the full `RunAgent`/`StopAgent`/`ListAgents` to match P2's `protocol.go` constants — INTERFACES §2 updated; (b) per-agent log path was a typo in the chat summary, the planning docs were already correct (`/var/log/agent-sandbox/<agent-id>.log`). Two additions from P2 captured: `agent.stdout`/`agent.stderr` daemon-emitted events for raw fd 1/2 (8 KiB cap, drop-on-full, `truncated` flag — INTERFACES §3.5); `event.type` on `IngestEvent` is server-validated against `^llm\.` regex, prefix violation → `INVALID_MANIFEST` (INTERFACES §2.8).

P3-owned:

- **OPEN-Q-001** — `gopkg.in/yaml.v3 v3.0.1` archive: stay on it for Week 3 (DEC-002). One-line migration in Week 4 if desired.
- **OPEN-Q-002** — Levenshtein: hand-write 30 lines, no dep.
- **OPEN-Q-003** — `agentctl logs --follow` reconnect: exit 5 in Week 3; `--retry` is Week 4 polish.
- **OPEN-Q-004** — `agentctl run` block-until-ready: no, return as soon as daemon confirms `clone3`.

---

## Open

### → To P5 — Process viewer + packaging

> One item still open for the Friday-of-Week-3 demo. (Q-006 is signed off — see closed list.)

#### OPEN-Q-009 — VM availability + CI runner shape

- **Target:** Day 2
- **Context:** Mission requires "end-to-end tests run against the real daemon on the shared VM in CI" (P3_WEEK3_PLAN.md WS-11). I'm writing the testscript-based suite; you own the VM and the CI runner.
- **What we need from you:** by Day 2 EOD —
  - Is the VM reachable from CI?
  - What's the CI invocation shape? (`vagrant up && ssh && go test ./e2e/vm/...`?)
  - Per-PR daemon, or a shared one (and how do we isolate test runs)?
  - Where do daemon logs go on failure so post-mortems are useful?

---

## Items P3 needs the user (P3 engineer) to verify with P4

These came back from P4 as "double-check before sending" / "confirm with the relevant party." Not blocking any document, but should round-trip before Day 2 so INTERFACES doesn't ship something P4 won't actually emit.

- **AIOS finding** — P4 inferred we went with the custom orchestrator from absence of AIOS code in the repo. Confirm this matches your spike notes.
- **`[RESULT]` line** — confirm `demo_agent.py` actually emits a `[RESULT]`-prefixed line. If not, soften / remove the `llm.tool_result` row in INTERFACES §3.2 (keep `llm.tool_call`).
- **`source` field** — P4 originally proposed a `source: "orchestrator|daemon"` field on the envelope to help P5's UI route. P3's call: not needed because `category` (llm/kernel/lifecycle) already routes (orchestrator-emitted = llm; daemon-emitted = kernel/lifecycle). Confirm with P5 that category-based routing is sufficient before locking.

---

## Issue-tracker mapping

These open questions correspond 1:1 to issues in the project tracker. Naming convention: `P3-W3-Q###` matching the IDs above. Keep them in sync; close the OPEN_QUESTIONS row when the tracker issue closes.
