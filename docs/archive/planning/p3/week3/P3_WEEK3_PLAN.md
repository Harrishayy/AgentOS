# P3 — Week 3 Plan: CLI + Manifest

**Owner:** P3
**Window:** Week 3, Mon–Fri (5 working days)
**Status:** Draft, awaiting team review at Day-1 sync
**Companion documents (read alongside):**
[INTERFACES.md](INTERFACES.md) (canonical schemas) · [DECISIONS.md](DECISIONS.md) (ADRs) · [OPEN_QUESTIONS.md](OPEN_QUESTIONS.md) · [RESEARCH_LOG.md](RESEARCH_LOG.md) · [SOURCES.md](SOURCES.md)

---

## Context

This is the week the CLI stops looking like a demo and becomes the front door for a real product. Through Week 2 the team built a vertical slice: one hardcoded agent, one hardcoded policy, one specific blocked connection, end-to-end. That proved the stack flows. Week 3 replaces the hardcoded values with real ones — the manifest parser drives policy, eBPF enforces real allowlists, the orchestrator runs multiple agents concurrently, the process viewer shows live events, demo scenarios run reliably (project-plan.md §4). For P3 specifically, the bar is: an engineer can sit down at a terminal, write a manifest by hand, and get useful behavior — including a useful error when they mistype a field. The Friday-of-Week-3 demo is the moment this either looks like something a platform engineer would want to use, or stays an artefact. The Day-1 schema lock-ins (with P1, P2, P4, P5) are the gating decisions that determine whether the rest of the week compounds or thrashes.

## Executive summary (≤200 words)

By end of Week 3, P3 ships an `agentctl` CLI that talks to the real daemon over a Unix domain socket and supports the full command surface: `run` (validates a manifest, returns a running agent or a line-and-column error; `--restart-on-crash`/`--max-restarts` flags pass through to the orchestrator), `list` (queries the daemon, prints a human or `--json` table), `stop` (clean termination via SIGTERM with grace), `logs` (live streamed kernel + LLM events, `--follow`/`--tail`/`--include`, clean Ctrl-C). Manifest validation hits the project-plan UX bar with hand-written messages keyed to a published catalogue. Shell completions for bash/zsh/fish are emitted by `agentctl completion <shell>`. CI runs unit and mock-integration tests on every push and end-to-end tests against the real daemon on the shared VM.

The three Day-1 cross-team locks: **(1) daemon protocol** (with P2 — INTERFACES §2), **(2) event schema** (with P4 + P5 — INTERFACES §3), **(3) supported manifest fields** (with P1 — INTERFACES §1.2). Until these close, the rest of the week churns.

The single biggest risk: `agentctl logs` streaming hanging or dropping events under signal. Mitigated by a context-cancellation discipline (DEC-007) and a testscript-based SIGINT test that asserts a sub-100ms exit.

## Assumptions about Week 2 state

The mission specifies the assumed Week 2 state. P3 starts Week 3 with: a Go module skeleton using `spf13/cobra`, a manifest typed-struct definition (no validation yet), a Unix-socket client that can talk to a hardcoded daemon path, a working `run` subcommand, basic error handling, unit tests for the parser, and one mock-daemon integration test.

Two assumptions this plan adds beyond the mission:

1. **`go.mod` exists and pins Go 1.22+.** `signal.NotifyContext` requires 1.16; testscript needs 1.21 for `t.Setenv` and friends. If Week 2 chose 1.21, that's fine.
2. **The Week-2 manifest struct has fields named per `snake_case` YAML tags** (e.g. `AllowedHosts []string \`yaml:"allowed_hosts"\``). If they're `camelCase`, Day 1 includes a 30-minute rename pass.

If either assumption is wrong, the only impact is a half-day Day-1 cleanup item. Flag at the Monday team sync.

## Workstreams

Ten workstreams. Numbering reflects rough sequencing — workstreams that share a number can run in parallel. See [Dependency view](#dependency-view) for the critical path.

### WS-1 — Day-1 schema lock-ins *(blocking)*

**Purpose.** Lock the three interfaces (daemon protocol, event schema, supported-field list) so dependent work can start.

**User-visible outcome.** None directly; all downstream UX depends on this.

**Tasks.**
1. Pre-circulate INTERFACES.md to P1/P2/P4/P5 by Mon 09:00.
2. **14:00 — P1 sync.** Lock supported-field list (OPEN-Q-007). Output: signed-off table replacing INTERFACES §1.2.
3. **15:00 — P2 sync.** Lock daemon protocol shape (OPEN-Q-005). Output: signed-off INTERFACES §2 including error-code list and `tail_n` semantics. (As of Day-1: P2 returned a counter; INTERFACES §2 has been mirrored to `daemon/api/proto.md`; final sign-off slipped to Day 2 10:00.)
4. **16:00 — P4 + P5 sync.** Lock event schema (OPEN-Q-006). Output: signed-off INTERFACES §3.
5. End of Day 1: commit `INTERFACES.md` revision; update DECISIONS.md if any ADR changed.

**Acceptance criteria.**
- INTERFACES.md is checked in with P1/P2/P4/P5 reviewer sign-off in commit message.
- Any deltas captured as new ADRs in DECISIONS.md.
- OPEN-Q-005, -006, -007 closed.

**Files.** `planning/p3/week3/INTERFACES.md`, `planning/p3/week3/DECISIONS.md`.

**Inputs.** P1's enforced-field list. P2's protocol counter-proposal (if any). P4's LLM event types. P5's UI consumption confirmation.

**Outputs.** Signed-off interface contracts that gate WS-2 through WS-7.

**Risks.** Day-1 sync slips → entire week slips. Mitigation: pre-circulate Sunday evening, treat the syncs as 30-minute deadlines, escalate to whole-team sync on Tuesday morning if any miss.

---

### WS-2 — Manifest validation hardening

**Purpose.** Replace Week-2 placeholder validation with the two-pass `yaml.Node` walker that produces line/column-precise hand-written errors.

**User-visible outcome.** Mistyping a field produces:
```
$ agentctl run my-agent.yaml
Error: my-agent.yaml:7:3: unknown field 'allowed_pots' (did you mean 'allowed_paths'?)
```

**Tasks.**
1. Implement the catalogued error codes in `internal/manifest/errors.go` (DEC-005). Each has a template, a code, and a producer.
2. Implement the two-pass parser in `internal/manifest/parse.go` (DEC-003). Pass 1: `yaml.Unmarshal(&node)`. Pass 2: walk and emit errors. Pass 3: `node.Decode` with `KnownFields(true)`.
3. Implement field validators in `internal/manifest/validate.go`: name regex, command non-empty, working-dir absolute path, host patterns (INTERFACES §1.3), path patterns, duration parse, env `${VAR}` resolution.
4. Add a `cmd/agentctl_manifest_validate` debug subcommand (hidden) for round-tripping a manifest through validation without sending it. Useful in tests and ad-hoc.
5. Levenshtein helper in `internal/manifest/suggest.go` (OPEN-Q-002 — hand-write 30 lines).
6. Golden-file unit tests covering every error code with a fixture YAML.

**Acceptance criteria.**
- Every error code in the catalog (DEC-005) is producible by some fixture, asserted byte-equal in golden tests.
- Mistyping `allowed_pots` produces the exact message in the executive-summary example.
- A manifest with a malformed path pattern (e.g., `allowed_paths: ["/foo/**"]` — `**` not in v1) produces the `invalid_path_pattern` error pointing at the right line/column. CIDR `allowed_hosts` (e.g., `10.0.0.0/8`) is accepted; an invalid mask (`/33`) produces `invalid_host_pattern`.
- `go test ./internal/manifest/...` passes; coverage ≥ 90% on `parse.go` and `validate.go`.

**Files.**
- `internal/manifest/manifest.go` — typed struct (modify Week 2's).
- `internal/manifest/parse.go` — new.
- `internal/manifest/validate.go` — new.
- `internal/manifest/errors.go` — new.
- `internal/manifest/suggest.go` — new.
- `internal/manifest/parse_test.go`, `validate_test.go` — new.
- `internal/manifest/testdata/*.yaml` and `testdata/*.golden` — fixtures.

**Inputs.** WS-1's INTERFACES §1.2 (supported-field list).

**Outputs.** A `manifest.Parse(path) (*Manifest, error)` API for WS-3, WS-4 to call.

**Risks.** Levenshtein false matches. Mitigation: cap distance at 2; if more than one candidate, list all.

---

### WS-3 — Daemon client library

**Purpose.** A reusable Go package for talking to `agentd` over the Unix socket. WS-4..WS-7 all consume it.

**User-visible outcome.** Internal — but a clean abstraction here is what keeps the subcommand code thin and testable.

**Tasks.**
1. `internal/daemon/protocol.go` — request/response Go types per INTERFACES §2.
2. `internal/daemon/client.go` — `Dial(ctx, socketPath) (*Client, error)`, with socket-discovery (DEC-008). Methods: `RunAgent`, `ListAgents`, `StopAgent`, `AgentLogs`, `StreamEvents`, `DaemonStatus` (wire names per P2's `protocol.go` constants; CLI's Go method names mirror them). Unary methods return a single response; `StreamEvents` returns a `<-chan Event` plus an error channel.
3. `internal/daemon/stream.go` — length-prefixed framing helpers (DEC-011): `WriteFrame(io.Writer, []byte) error` and `ReadFrame(io.Reader) ([]byte, error)`, mirroring P2's helpers. Cap at 16 MiB. Cancellation via `SetReadDeadline` from a context-watcher goroutine.
4. Error mapping: server `error.code` strings (`INVALID_MANIFEST`, `AGENT_NOT_FOUND`, `CGROUP_FAILED`, `BPF_LOAD_FAILED`, `LAUNCH_FAILED`, `PERMISSION_DENIED`, `INTERNAL`) → typed Go errors (`var ErrAgentNotFound = errors.New(...)`) so subcommands can `errors.Is`.
5. `internal/testutil/mockdaemon.go` — in-process mock server over a `net.Pipe` or a temp socket. Drives integration tests without `agentd`.
6. `internal/daemon/client_test.go` — round-trip tests against the mock for every method including `StreamEvents` mid-stream cancel.

**Acceptance criteria.**
- All six methods have a passing round-trip test against the mock.
- `StreamEvents` cancellation: a test that calls `cancel()` mid-stream returns within 50ms with `context.Canceled`.
- `Dial` falls through the four discovery paths (DEC-008) in order and returns `ErrDaemonUnreachable` with the last attempted path.
- Oversize frame (> 16 MiB) is rejected with a typed error.

**Files.** `internal/daemon/{client,protocol,stream}.go` and tests; `internal/testutil/mockdaemon.go`.

**Inputs.** WS-1's INTERFACES §2.

**Outputs.** A `daemon.Client` consumed by all subcommands.

**Risks.** Half-close semantics need exercise: unary methods use client `shutdown(SHUT_WR)` after writing the request frame; `StreamEvents` does **not** half-close (server keeps writing indefinitely). Get this wrong and `StreamEvents` hangs or unary methods stall. Mitigation: explicit test for both patterns; INTERFACES §2 documents which pattern each method uses.

---

### WS-4 — `agentctl run` against the real daemon

**Purpose.** Replace Week-2's hardcoded-daemon-path `run` with a real client call backed by manifest validation.

**User-visible outcome.**
```
$ agentctl run examples/web-fetcher.yaml
agent 'web-fetcher' started (pid 12345, policy: hosts:1 paths:0 timeout:0)
$ agentctl run examples/web-fetcher.yaml --json
{"agent":{"name":"web-fetcher","pid":12345,"started_at":"2026-04-27T11:42:13Z","policy_summary":"hosts:1 paths:0 timeout:0"}}
```

**Tasks.**
1. `cmd/run.go` — a `*cobra.Command`. `RunE` calls `manifest.Parse`, then `daemon.Client.Run`. Resolves `${VAR}` env interpolation client-side (INTERFACES §1.5).
2. Wire `--json` flag at root with subcommand inheritance.
3. Render success and error via `internal/render`.
4. Exit-code mapping (DEC-009) routed via `cmd/exit.go`.
5. Test: testscript script `e2e/testdata/script/run-success.txt` and `run-bad-manifest.txt`. Real-daemon e2e in WS-11.

**Acceptance criteria.**
- `agentctl run examples/web-fetcher.yaml` against mock daemon prints expected human and JSON output.
- `agentctl run examples/file-reader.yaml` (which has `allowed_paths: ["/srv/agent-input/"]`) starts successfully against the mock; reads outside the allowlist are denied by the BPF LSM hook in real-VM e2e (WS-11).
- Manifest with unset `${ANTHROPIC_API_KEY}` errors at the right line, exits 3.

**Files.** `cmd/run.go`, `cmd/run_test.go`, `internal/render/run.go`.

**Inputs.** WS-2 (`manifest.Parse`), WS-3 (`daemon.Client.Run`).

**Outputs.** None for downstream.

---

### WS-5 — `agentctl list`

**Purpose.** Query the daemon for running agents.

**User-visible outcome.**
```
$ agentctl list
NAME            PID    STATUS   UPTIME    POLICY
web-fetcher     12345  running  00:00:42  hosts:1 paths:0 timeout:0
demo-agent      12346  exited   --        hosts:2 paths:0 timeout:5m

$ agentctl list --json
{"agents":[{"name":"web-fetcher","pid":12345,"status":"running","uptime_ns":42000000000,"policy_summary":"hosts:1 paths:0 timeout:0"}, ...]}
```

**Tasks.**
1. `cmd/list.go` — calls `daemon.Client.List`.
2. Human renderer: tabwriter, fixed columns. Uptime formatted `HH:MM:SS` for under 24h, else `Nd HH:MM`.
3. JSON renderer: pass-through of the protocol response.
4. Empty result: human `(no agents running)`, JSON `{"agents":[]}`.

**Acceptance criteria.**
- Golden tests for human (line-count + regex) and JSON (byte-equal) outputs.
- Empty list produces correct empty-state output.

**Files.** `cmd/list.go`, `internal/render/list.go`, `internal/render/list_test.go`, `internal/render/golden/list-*.txt`.

**Inputs.** WS-3.

---

### WS-6 — `agentctl stop`

**Purpose.** Clean termination of a running agent with configurable grace period.

**User-visible outcome.**
```
$ agentctl stop web-fetcher
agent 'web-fetcher' stopped (signal: SIGTERM, exit 0, took 412ms)

$ agentctl stop --grace 0 web-fetcher
agent 'web-fetcher' stopped (signal: SIGKILL, exit 137, took 12ms)
```

**Tasks.**
1. `cmd/stop.go` — calls `daemon.Client.Stop` with the grace duration.
2. `--grace duration` flag, default 5s.
3. Human + JSON render.
4. Error case: `name_unknown` → exit 6.

**Acceptance criteria.**
- testscript: stop a running mock agent → exit 0 with expected line.
- testscript: stop a nonexistent agent → exit 6 with the `name_unknown` message.

**Files.** `cmd/stop.go`, `internal/render/stop.go`.

**Inputs.** WS-3.

---

### WS-7 — `agentctl logs --follow` *(largest single workstream)*

**Purpose.** Stream interleaved kernel + LLM events for one agent in real time, with clean Ctrl-C.

**User-visible outcome.**
```
$ agentctl logs --follow web-fetcher
2026-04-27T11:42:13.123Z  llm       llm.stdout            user: fetch the OpenAI models page
2026-04-27T11:42:13.456Z  llm       llm.tool_call         fetch_url(url=https://api.openai.com/v1/models) [120ms]
2026-04-27T11:42:13.461Z  kernel    kernel.connect_allowed 203.0.113.5:443 (api.openai.com)
2026-04-27T11:42:13.512Z  kernel    kernel.connect_blocked 198.51.100.7:443 reason=not_in_allowlist
2026-04-27T11:42:13.789Z  llm       llm.tool_result       fetch_url -> ok [340ms]
^C
```

**Tasks.**
1. `cmd/logs.go` — flags `--follow`, `--tail` (int, default 100; replay only), `--include`, `--json`. `--follow` and `--tail` are mutually exclusive in v1: `--tail` calls `AgentLogs`; `--follow` calls `StreamEvents`.
2. Use `cmd.Context()` (signal-aware via DEC-007).
3. Iterate the event channel from `daemon.Client.StreamEvents` (or the response of `AgentLogs` for `--tail`). On `ctx.Done()`, close the connection and exit 130.
4. Renderer (`internal/render/logs.go`):
   - Human format: `<ts>  <category>  <type>  <one-line summary>`. The summary is type-specific (see below).
   - JSON format: one NDJSON line per event (P5-style line orientation; this is renderer-only, the wire is length-prefixed).
5. Type-specific summarisers:
   - `llm.stdout`: `<truncated line, 80 char>`.
   - `llm.tool_call`: `<tool>(<args summarized>) [<latency_ms>ms, <tokens_in>→<tokens_out>]?`.
   - `llm.tool_result`: `<tool> -> <ok|err> [<latency_ms>ms]?`.
   - `llm.stopped` / `llm.crashed`: `exit <exit_code>`.
   - `kernel.connect_allowed`: `<dst>:<port> (<hostname or "?">)`.
   - `kernel.connect_blocked`: `<dst>:<port> reason=<reason>`.
   - `agent.stdout` / `agent.stderr`: `<truncated line, 80 char>` with a `(truncated)` marker if `data.truncated == true`. Only rendered when caller passes `--include=agent`; default excludes them.
   - `lifecycle.*`: `<details>`.
6. End handling: stream terminates on EOF (no `end` sentinel — DEC-011). Renderer prints `--- agent <name> stream closed` on EOF in human mode; suppressed in `--json`.
7. testscript suite:
   - `logs-follow.txt` — start mock, emit 50 events, assert all rendered.
   - `logs-ctrl-c.txt` — start `agentctl logs --follow &name&`, wait, `kill -INT name`, assert exit code 130 within 100ms.
   - `logs-tail.txt` — `--tail=20` returns last 20 events from mock's fixture file.
   - `logs-include.txt` — `--include=kernel` skips llm events.

**Acceptance criteria.**
- 60-second mock-driven soak with 1000 ev/s — zero drops, zero goroutine leaks (`runtime.NumGoroutine` stable). Asserted in a soak test gated behind a build tag, run nightly.
- SIGINT test: `kill -INT mycmd` → exit 130 within 100ms.
- Render parity: same NDJSON in `--json` mode for any event the human renderer touches.

**Files.** `cmd/logs.go`, `internal/render/logs.go`, plus tests under `internal/render/golden/logs-*.json`.

**Inputs.** WS-3 (streaming client), WS-1 (event schema).

**Risks (the biggest in the week):**
- Goroutine leak on signal: a partial read on the socket leaves a reader goroutine blocked. Mitigation: explicit `SetReadDeadline(time.Now())` on cancel; the soak test would catch a leak.
- Backpressure when human renderer is slow: stdout to a slow terminal could backpressure the reader. Mitigation: events buffer in the client (channel of 256), drop-with-warning if the buffer fills (and emit a synthesised `lifecycle.warning` event noting drop count). Locked in DEC if it actually surfaces.
- `--tail` retrieving more events than the daemon's per-agent log file holds (~30 MiB after rotation): daemon truncates silently to what's available; CLI prints a one-line `(<n> events available; older entries rotated out)` notice in human mode.

---

### WS-8 — Output formatting layer

**Purpose.** A single rendering package shared by all subcommands so JSON/human parity is enforced by structure, not discipline.

**Tasks.**
1. `internal/render/render.go` — interface `Renderer` with `Human(io.Writer)`, `JSON(io.Writer) error`. Each subcommand defines a type implementing it.
2. Tabwriter helpers, time-format helpers, byte/duration humanisers.
3. Color: only on TTY (`isatty.IsTerminal(os.Stdout.Fd())`), never in `--json`. One package: `mattn/go-isatty` (vendor minimum).
4. Tests: golden files for every subcommand's output, both modes.

**Acceptance criteria.** A subcommand cannot ship without filing both a human and JSON renderer. CI lint (custom go-vet check or just code review) enforces.

**Files.** `internal/render/*.go`.

**Inputs.** None.

**Outputs.** Used by WS-4..WS-7.

---

### WS-9 — Shell completions

**Purpose.** Tab-completion for bash, zsh, fish.

**Tasks.**
1. `cmd/completion.go` — call `rootCmd.InitDefaultCompletionCmd()` (DEC-006). Cobra wires the four shells automatically.
2. Customise root help footer with one-line install reminder.
3. Smoke tests via testscript:
   - `agentctl completion bash > script` — assert non-empty, contains `_agentctl()` function.
   - Same for zsh and fish.
4. Per-shell verification (manual, documented in `docs/completion.md` Week 4): load and tab-complete `agentctl run <TAB>`, `agentctl logs <TAB>` for an agent name. Week 3 doesn't yet wire dynamic completion for agent names — that's a Week-4 polish item using cobra's `ValidArgsFunction` calling the daemon's `list`.

**Acceptance criteria.** All three completion scripts generate. Bash script passes `bash -n` syntax check.

**Files.** `cmd/completion.go`, `e2e/testdata/script/completion-{bash,zsh,fish}.txt`.

**Inputs.** None.

---

### WS-10 — Integration tests against mock daemon (CI on every push)

**Purpose.** Catch regressions without needing the VM.

**Tasks.**
1. `internal/testutil/mockdaemon.go` — extend WS-3's mock with a fixture loader: load a YAML or JSON file describing the canned response/event sequence.
2. testscript integration suite under `e2e/` with `RequireExplicitExec: true`. Each script boots the mock in-process via a `Cmds` entry that reads a fixture path.
3. Suite covers every subcommand happy path, every error path that's testable without the kernel (manifest errors, daemon unreachable, name unknown).
4. Wire into `.github/workflows/ci.yml`.

**Acceptance criteria.** `go test ./...` and `go test -tags=integration ./...` both green in CI on every push. Total runtime under 60s.

**Files.** `e2e/testscript_test.go`, `e2e/testdata/script/*.txt`, `e2e/testdata/fixtures/*.yaml`, `.github/workflows/ci.yml`.

**Inputs.** WS-3 (mock primitive), all subcommand workstreams.

---

### WS-11 — End-to-end tests against real daemon on shared VM (CI on PR)

**Purpose.** Prove the binary works against the real daemon, not just the mock.

**Tasks.**
1. Coordinate with P5 (OPEN-Q-009) on CI runner shape and VM availability.
2. `.github/workflows/vm-e2e.yml` — bring up the VM, install agentd, run a curated subset of testscript scripts pointed at the real daemon (via `AGENT_SANDBOX_SOCKET=/run/agent-sandbox.sock`).
3. Smoke matrix: each of the four example manifests, plus a `logs --follow` 30s synthetic run.
4. On failure: capture daemon logs and the last 100 lines of the agent process's stderr.

**Acceptance criteria.**
- VM job green on every PR labeled `e2e` or every push to `main`.
- Total runtime under 5 minutes.
- Failure produces enough log context to diagnose without re-running.

**Files.** `.github/workflows/vm-e2e.yml`, `e2e/vm/run.sh`.

**Inputs.** P5 (VM + CI runner), all other workstreams completed.

---

## Dependency view

```
                 ┌─────────────────┐
                 │ WS-1: Day-1     │
                 │ schema lock-ins │  (BLOCKS EVERYTHING)
                 └────────┬────────┘
                          │
            ┌─────────────┼──────────────┐
            ▼             ▼              ▼
     ┌────────────┐ ┌──────────┐  ┌──────────────┐
     │ WS-2:      │ │ WS-3:    │  │ WS-9:        │
     │ Manifest   │ │ Daemon   │  │ Completions  │
     │ validation │ │ client   │  │ (independent)│
     └─────┬──────┘ └────┬─────┘  └──────┬───────┘
           │             │               │
           └──────┬──────┘               │
                  │                      │
          ┌───────┼───────┬───────┐      │
          ▼       ▼       ▼       ▼      │
       ┌─────┐ ┌─────┐ ┌─────┐ ┌──────┐  │
       │WS-4 │ │WS-5 │ │WS-6 │ │ WS-7 │  │  (WS-7 is the long pole)
       │run  │ │list │ │stop │ │ logs │  │
       └──┬──┘ └──┬──┘ └──┬──┘ └──┬───┘  │
          │       │       │       │      │
          └───────┴───┬───┴───────┘      │
                      │                  │
                      ▼                  │
              ┌────────────────┐         │
              │ WS-8: Output   │ ←───────┘ (consumed by all subcommands)
              │ formatting     │
              └───────┬────────┘
                      │
        ┌─────────────┴────────────┐
        ▼                          ▼
 ┌───────────────┐         ┌───────────────┐
 │ WS-10:        │         │ WS-11:        │
 │ Mock-daemon   │         │ VM e2e        │
 │ integration   │         │ (real daemon) │
 │ (every push)  │         │ (PR/main)     │
 └───────────────┘         └───────────────┘
```

**Critical path.** WS-1 → WS-3 → WS-7 → WS-11. WS-7 is the long pole; allocate it the most "buffer day" if anything has to slip on Day 4.

**Day-1 must-close.** WS-1's three sub-locks (P1 supported fields, P2 protocol, P4+P5 event schema). If even one slips past Tuesday morning, the team owes a same-day escalation.

**Parallel-safe.** WS-2 and WS-3 are independent and can run on Day 2. WS-9 is independent of everything; do it any time you need a half-hour change of pace.

## Manifest v1 — schema summary

Full grammar in [INTERFACES.md §1](INTERFACES.md). Quick reference:

| Field | Required | Type | Default | Notes |
|---|---|---|---|---|
| `name` | yes | string | — | `[a-z0-9-]{1,63}` |
| `command` | yes | `[]string` | — | argv, no shell expansion |
| `allowed_hosts` | yes (may be `[]`) | `[]host-pattern` | — | enforced by P1 |
| `allowed_paths` | yes (may be `[]`) | `[]path-pattern` | — | enforced by P1 via BPF LSM `file_open` hook |
| `working_dir` | no | abs-path | `/tmp/agentctl/<name>` | |
| `env` | no | `map[string]string` | `{}` | `${VAR}` is resolved client-side |
| `user` | no | string\|uid | caller's uid | |
| `stdin` | no | enum | `"close"` | `inherit`/`close`/`file:<path>` |
| `timeout` | no | duration | `"0"` | 0 = no timeout |
| `description` | no | string | `""` | metadata only |

Four canonical manifests (full text in [INTERFACES.md §1.5](INTERFACES.md)): `web-fetcher.yaml`, `file-reader.yaml`, `shell-runner.yaml`, `llm-agent.yaml`. The `file-reader.yaml` is the path-policy demo: it exercises the BPF LSM `file_open` enforcement that P1 ships in v1 (per OPEN-Q-007 closure).

## Error message catalog

Exact human-facing messages. Code is the JSON `error.code`; message column is the rendered string.

| Code | Human message |
|---|---|
| `unknown_field` (with suggestion) | `<file>:<line>:<col>: unknown field 'X' (did you mean 'Y'?)` |
| `unknown_field` (no suggestion) | `<file>:<line>:<col>: unknown field 'X'; valid keys: name, command, allowed_hosts, allowed_paths, working_dir, env, user, stdin, timeout, description` |
| `wrong_kind` | `<file>:<line>:<col>: field 'X' must be a <kind>, got <kind>` |
| `missing_required` | `<file>:<line>:<col>: required field 'X' is missing` |
| `invalid_name` | `<file>:<line>:<col>: name '<v>' is invalid: must match [a-z0-9-]{1,63}` |
| `empty_command` | `<file>:<line>:<col>: 'command' must be a non-empty list of argv elements` |
| `non_absolute_path` | `<file>:<line>:<col>: 'working_dir' must be an absolute path; got '<v>'` |
| `invalid_host_pattern` | `<file>:<line>:<col>: 'allowed_hosts[<i>]' is not a valid host pattern; expected hostname (api.example.com), IP, or wildcard (*.example.com), optionally with :port` |
| `invalid_path_pattern` | `<file>:<line>:<col>: 'allowed_paths[<i>]' is not a valid path pattern; expected absolute path, '/dir/' for tree, or single '*' glob` |
| `unsupported_field_value` | *(reserved; no v1 field triggers this — P1 enforces all declared fields. Kept in the catalog for future deferred-enforcement scenarios.)* |
| `bad_duration` | `<file>:<line>:<col>: 'timeout' must be a duration like 30s, 5m, 1h; got '<v>'` |
| `unset_env_var` | `<file>:<line>:<col>: env value '<${VAR}>' references unset variable; export it or remove the entry` |
| `bad_user` | `<file>:<line>:<col>: user '<v>' is not a known account` |
| `bad_stdin` | `<file>:<line>:<col>: 'stdin' must be 'inherit', 'close', or 'file:<path>'; got '<v>'` |
| `yaml_parse` | `<file>:<line>:<col>: yaml: <yaml.v3 message>` |

JSON shape for client-synthesized errors: `{"error":{"code":"<code>","message":"<formatted>","field":"<dotted.path>","manifest_path":"<file>","manifest_line":<n>,"manifest_column":<n>}}`. Server-side errors arrive in P2's envelope shape (`{ok:false, error:{code, message}}`, INTERFACES §2.1) and are translated by `internal/daemon/client.go` into the same client-side shape before rendering, so `--json` output is uniform regardless of source.

## CLI surface

### `agentctl --help`

```
agentctl — sandbox runtime for AI agents

Usage:
  agentctl [command]

Available Commands:
  run         Launch an agent inside the sandbox from a manifest
  list        List running agents
  stop        Terminate a running agent
  logs        Stream events for a running or recent agent
  completion  Generate shell completion scripts (bash, zsh, fish, powershell)
  version     Print the build version
  help        Help about any command

Flags:
      --socket string   Path to the agent-sandbox Unix socket (overrides AGENT_SANDBOX_SOCKET)
      --json            Emit machine-readable JSON instead of human-formatted output
  -v, --verbose         Verbose logging to stderr
  -h, --help            help for agentctl

Use "agentctl [command] --help" for more information about a command.
```

### `agentctl run --help`

```
Launch an agent inside the sandbox from a YAML manifest.

Usage:
  agentctl run <manifest.yaml> [flags]

Examples:
  agentctl run examples/web-fetcher.yaml
  agentctl run my-agent.yaml --json
  agentctl run my-agent.yaml --restart-on-crash --max-restarts=5

Flags:
      --json                Emit machine-readable JSON
      --restart-on-crash    Ask the orchestrator to relaunch on non-zero exit (DEC-012)
      --max-restarts int    Cap automatic restarts (default 3; ignored without --restart-on-crash)
  -h, --help

Exit codes:
  0  agent started
  3  manifest parse or validation failed
  4  daemon unreachable
  5  daemon rejected the manifest (e.g. name in use)
```

### `agentctl list --help`

```
List running agents known to the daemon.

Usage:
  agentctl list [flags]

Examples:
  agentctl list
  agentctl list --json | jq '.agents[] | select(.status=="running")'

Flags:
      --json   Emit machine-readable JSON
  -h, --help

Exit codes:
  0  ok (including empty list)
  4  daemon unreachable
```

### `agentctl stop --help`

```
Terminate a running agent.

Usage:
  agentctl stop <name> [flags]

Examples:
  agentctl stop web-fetcher
  agentctl stop --grace 10s web-fetcher

Flags:
      --grace duration   Grace period before SIGKILL (default 5s)
      --json             Emit machine-readable JSON
  -h, --help

Exit codes:
  0  stopped
  4  daemon unreachable
  6  agent name not found
```

### `agentctl logs --help`

```
Stream events for a running or recent agent. Events are interleaved
LLM-level (prompts, tool calls) and kernel-level (allowed/blocked
connections), in timestamp order.

Usage:
  agentctl logs <name> [flags]

Examples:
  agentctl logs web-fetcher                       # last 100 events (calls AgentLogs)
  agentctl logs --tail=500 web-fetcher            # last 500 events
  agentctl logs --follow web-fetcher              # live stream (calls StreamEvents)
  agentctl logs --json --include=kernel web-fetcher | jq

Flags:
  -f, --follow            Stream events live until interrupted (mutually exclusive with --tail)
      --tail int          Replay last N events from the daemon's per-agent log (default 100)
      --include strings   Categories to include: llm,kernel,lifecycle,agent (default: llm,kernel,lifecycle — raw stdio excluded)
      --json              Emit one NDJSON event per line
  -h, --help

Signals:
  SIGINT (Ctrl-C) and SIGTERM cause clean exit. Returns code 130.

Exit codes:
  0    stream completed (server EOF, or non-follow tail returned)
  4    daemon unreachable
  5    daemon error mid-stream
  6    agent name not found
  130  interrupted by signal
```

### Output conventions

- Human and `--json` outputs are produced from the same in-memory struct (DEC-004).
- Color: TTY-only, never in `--json`.
- Errors: human goes to stderr with `Error: ` prefix; `--json` errors go to stdout for jq pipelining and exit nonzero.
- Timestamps in human output: local time, RFC 3339 with millisecond precision. Timestamps in `--json`: UTC RFC 3339 with nanosecond precision (the protocol value, untouched).

### Signal handling for `logs`

`signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` at root (DEC-007). The cancellation propagates through `daemon.Client.Logs` which:

1. Sets `SetReadDeadline(time.Now())` on the conn → blocked `Read` returns `os.ErrDeadlineExceeded`.
2. Calls `conn.Close()`.
3. Drains any in-flight events from the channel and exits the goroutine.

Asserted by `e2e/testdata/script/logs-ctrl-c.txt`: 100ms wall-clock budget from SIGINT to process exit.

## Daemon protocol — summary

Full spec in [INTERFACES.md §2](INTERFACES.md), which mirrors P2's `daemon/api/proto.md` (the wire-level source of truth). Quick reference:

- Socket: `/run/agent-sandbox.sock`, root, mode 0600. Discovery order from DEC-008; env override `AGENT_SANDBOX_SOCKET`.
- Wire: 4-byte BE length-prefix + UTF-8 JSON, 16 MiB cap (DEC-011). Request body `{method, params}` ↔ response `{ok, result}` or `{ok:false, error:{code, message}}`.
- Methods: `RunAgent`, `StopAgent`, `ListAgents`, `AgentLogs` (request/response, `tail_n` only), `StreamEvents` (persistent push, EOF terminator), `DaemonStatus`, `IngestEvent` (orchestrator → daemon LLM events; SO_PEERCRED authz; `event.type` must be `llm.`-prefixed, validated server-side).
- Errors: `INVALID_MANIFEST`, `AGENT_NOT_FOUND`, `CGROUP_FAILED`, `BPF_LOAD_FAILED`, `LAUNCH_FAILED`, `PERMISSION_DENIED`, `INTERNAL`. Plus client-synthesized `daemon_unreachable`, `manifest_parse_failed`.
- Versioning: queried via `DaemonStatus.result.protocol_version`; method names are append-only by P2's contract.

## Event schema — summary

Full spec in [INTERFACES.md §3](INTERFACES.md). Envelope:
```json
{"schema":"v1","ts":"<rfc3339nano>","agent":"<name>","category":"llm|kernel|lifecycle|agent","type":"<type>","data":{...}}
```
Category-type matrix in INTERFACES §3.2–§3.5. The `agent` category covers daemon-captured raw fd 1/2 lines (`agent.stdout`/`agent.stderr`); distinct from `llm.stdout` (orchestrator's parsed view). P5 consumes the same NDJSON over its WebSocket bridge — no schema fork.

## Testing plan

| Layer | What | Where | When |
|---|---|---|---|
| Unit | manifest parse/validate, render output, exit code map, daemon-client encoding | `internal/**/*_test.go` | every push |
| Integration | every subcommand × happy/error paths against in-process mock daemon | `e2e/testscript_test.go` with mock | every push |
| End-to-end | each example manifest, plus `logs --follow` 30s, against real daemon on shared VM | `.github/workflows/vm-e2e.yml` | PRs labeled `e2e` + every push to `main` |
| Soak | 60s `logs --follow` at 1000 ev/s, no drops, no goroutine leak | nightly job (build tag `soak`) | nightly |

Coverage targets: `internal/manifest` ≥ 90%; `internal/daemon` ≥ 85%; `internal/render` ≥ 80%; `cmd/*` need not exceed 60% (most coverage comes via testscript).

CI gate: a PR cannot land with red unit or mock-integration. VM e2e is allowed to be red on a PR if it's a P5/VM issue but must be green before merge to `main` (escape-hatch label `vm-e2e-skip` for non-blocking infra outages).

## Risks and mitigations

| Risk | Likelihood | Mitigation |
|---|---|---|
| Day-1 lock-ins slip → entire week thrashes | medium | Pre-circulate Sunday; treat the three syncs as 30-minute deadlines; same-day escalation to all-hands sync if any miss |
| Daemon protocol churn mid-week | medium | `protocol: "v1"` versioning; mock daemon decoupled; small protocol surface (4 commands) |
| Event schema disagreement P4↔P5 | medium | Day-1 joint sync; P3 chairs; INTERFACES.md is canonical and only gets edited by sign-off |
| Validation rejects fields P1 supports (or vice versa) | medium | OPEN-Q-007 closes Day 1 with a signed-off table; weekly check-in Wed afternoon |
| `agentctl logs` hangs or drops on signal | high (impact) / low (likelihood) | DEC-007 discipline; mandatory testscript SIGINT test; nightly soak test |
| `gopkg.in/yaml.v3` archive surprise | low | DEC-002 + OPEN-Q-001; one-line `go.mod` swap to `go.yaml.in/yaml/v3` if needed |
| VM CI flake | medium | OPEN-Q-009 closes Day 2 with P5; failure mode captures full daemon logs |
| Goroutine leak in stream client | low | Soak test asserts `runtime.NumGoroutine` stable; `pprof` available via `--verbose` |

## Exit criteria — Friday demo checklist

Every item is testable. The demo is exactly these items run live in front of the team.

- [ ] **Build and version.** `go build ./cmd/agentctl && ./agentctl version` prints `agentctl <semver> (<git-sha>) — protocol v1`.
- [ ] **Run example #1.** `agentctl run examples/web-fetcher.yaml` against the real daemon on the VM exits 0 with the expected human line and the agent appears in `agentctl list`.
- [ ] **Run example #4.** `agentctl run examples/llm-agent.yaml` runs an actual Claude agent for 30 seconds.
- [ ] **List.** `agentctl list` shows both agents; `agentctl list --json | jq '.agents | length'` returns 2.
- [ ] **Logs follow.** `agentctl logs --follow llm-agent` streams a 60-second window with at least one `llm.tool_call`, one `kernel.connect_allowed`, and one `kernel.connect_blocked`. Output is non-empty in both human and `--json` modes.
- [ ] **Logs Ctrl-C.** Ctrl-C in the above terminates the CLI within 100ms with exit code 130.
- [ ] **File-reader path policy.** `agentctl run examples/file-reader.yaml` exits 0; the agent reads `/srv/agent-input/data.txt` successfully and an attempt to read `/etc/passwd` from a sibling shell-runner is denied by the BPF LSM hook (visible in `agentctl logs` once P1 surfaces path events).
- [ ] **Bad manifest.** A manifest with `allowed_paths: ["/foo/**"]` exits 3 with the `invalid_path_pattern` message at the right line/column. A manifest with `allowed_hosts: ["10.0.0.0/33"]` exits 3 with `invalid_host_pattern`.
- [ ] **Typo.** A manifest with `allowed_pots: [...]` exits 3 with `did you mean 'allowed_paths'?`.
- [ ] **Stop.** `agentctl stop llm-agent` exits 0; subsequent `agentctl list` shows it as `exited`.
- [ ] **Help.** `agentctl --help` and every subcommand `--help` render correctly (including the example block).
- [ ] **Completions.** `agentctl completion bash > /tmp/agentctl.bash && bash -n /tmp/agentctl.bash` succeeds; same for zsh and fish (`zsh -n`, `fish -c "source /tmp/agentctl.fish"`).
- [ ] **CI.** Latest `main` shows green unit, mock-integration, and VM-e2e jobs.
- [ ] **Docs.** `INTERFACES.md` reflects the as-shipped contract; OPEN_QUESTIONS.md has zero entries from sections A or B that are still open.

## Verification recipe (for the executing engineer)

To verify this plan executed correctly, on a fresh shared VM clone of the repo:

```bash
# Build
go build ./cmd/agentctl

# Unit + mock integration
go test ./...
go test -tags=integration ./e2e/...

# Start the daemon (P2's binary; assumed installed at /usr/local/bin/agentd)
sudo systemctl start agentd

# Run all four example manifests through their lifecycle
for m in examples/{web-fetcher,file-reader,shell-runner,llm-agent}.yaml; do
  ./agentctl run "$m"
done
./agentctl list
./agentctl logs --follow web-fetcher &
sleep 5; kill -INT %1; wait %1; echo "exit=$?"   # expect 130
./agentctl stop web-fetcher
./agentctl stop file-reader
./agentctl stop shell-runner
./agentctl stop llm-agent

# Bad-manifest check (malformed path glob)
./agentctl run examples/bad-paths.yaml; echo "exit=$?"   # expect 3 (invalid_path_pattern)
./agentctl run examples/bad-cidr.yaml;  echo "exit=$?"   # expect 3 (invalid_host_pattern, /33 mask)

# Completions
./agentctl completion bash | bash -n
./agentctl completion zsh  | zsh -n
./agentctl completion fish > /tmp/c.fish && fish -c 'source /tmp/c.fish'
```

If every line above behaves as commented, Week 3 is done.

---

## Out of scope (Week 4)

For clarity — these are intentionally excluded from Week 3 and tracked for Week 4:

- Man pages (`mango`/`md2man` generation).
- Install script and `goreleaser` config for binary releases.
- Debian package.
- Published tutorial / blog post.
- Dynamic argument completion (e.g. `agentctl logs <TAB>` auto-completing agent names by calling the daemon).
- `agentctl logs --retry` for transient daemon-stream errors (OPEN-Q-003).
- `agentctl run --wait` readiness signal (OPEN-Q-004).
- Multi-agent batch operations (`agentctl run *.yaml`).
- Telemetry / structured logging beyond `--verbose`.
