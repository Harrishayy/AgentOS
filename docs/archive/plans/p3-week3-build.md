# P3 Week 3 — V1 Build Plan

Concrete day-by-day execution plan to ship `agentctl` V1 by **Fri 2026-05-01**. Companion to the cross-team plan at `/workspace/planning/p3/week3/`.

## Context

- Today: **Tue 2026-04-28**. Four working days: Tue, Wed, Thu, Fri.
- Cross-team interfaces are locked. Open questions: only P5 Q-009 (VM/CI shape) — affects WS-11 only.
- The cross-team plan assumes Week 2 left a Go module + cobra skeleton + manifest struct + Unix-socket client + working `run` stub + tests. **None of that exists.** Tuesday morning is Week-2-makeup before WS-2/WS-3 proper.
- V1 = the 13 items in `P3_WEEK3_PLAN.md` Exit-criteria checklist. Out-of-scope items in §"Out of scope (Week 4)" stay deferred — no scope creep.

## V1 scope — what ships Friday

The 13 demo-checklist items, in dependency order:

1. `go build ./cmd/agentctl && ./agentctl version` prints semver + git SHA + protocol v1
2. `agentctl run examples/web-fetcher.yaml` against real daemon → exit 0, agent in `list`
3. `agentctl run examples/llm-agent.yaml` runs Claude agent for 30s
4. `agentctl list` (human + `--json`)
5. `agentctl logs --follow llm-agent` streams 60s with `llm.tool_call` + `kernel.connect_allowed` + `kernel.connect_blocked`
6. `agentctl logs --follow` Ctrl-C exits within 100ms with code 130
7. `agentctl run examples/file-reader.yaml` exits 0; sibling shell-runner `/etc/passwd` read denied
8. Bad manifest `allowed_paths: ["/foo/**"]` exits 3 with `invalid_path_pattern` + line/col; `/33` mask exits 3 with `invalid_host_pattern`
9. Typo `allowed_pots` exits 3 with `did you mean 'allowed_paths'?`
10. `agentctl stop llm-agent` exits 0; subsequent `list` shows `exited`
11. `--help` on root + every subcommand renders correctly
12. `agentctl completion {bash,zsh,fish}` outputs syntactically-valid scripts
13. CI green: unit + mock-integration + VM-e2e

**Single biggest cliff:** item 6 (Ctrl-C in 100ms). DEC-007 + a testscript test mitigate.

## Day-by-day

### Tuesday — Bootstrap + manifest validator

**AM (Week-2 makeup).** Get a buildable, runnable scaffold:
- `go mod init github.com/<owner>/agent-sandbox-cli` (or whatever org owns this); pin Go 1.22+
- Vendor: `spf13/cobra@v1.10.2`, `gopkg.in/yaml.v3@v3.0.1` (DEC-002), `rogpeppe/go-internal/testscript@v1.14.1`
- Dir layout: `cmd/agentctl/`, `internal/{manifest,daemon,render,testutil}/`, `e2e/`, `examples/`
- `cmd/agentctl/main.go` — cobra root with `version` baked from `-ldflags`
- `internal/manifest/manifest.go` — typed struct per INTERFACES §1.1 (snake_case yaml tags)
- Round-trip smoke: `go build ./... && ./agentctl --help` prints

**PM (WS-2 manifest validator — INTERFACES §1).** Two-pass YAML + error catalog:
- `internal/manifest/parse.go` — three passes per DEC-003: `yaml.Unmarshal(&node)`; walk `yaml.Node` to emit unknown-field/typo errors with `node.Line`/`node.Column`; `node.Decode(KnownFields(true))`
- `internal/manifest/errors.go` — catalogued errors per DEC-005 (one type per code: `invalid_yaml`, `unknown_field`, `invalid_name`, `invalid_path_pattern`, `invalid_host_pattern`, `invalid_duration`, `invalid_user`, `missing_required_field`, plus reserved `unsupported_field_value`)
- `internal/manifest/validate.go` — name regex, command non-empty, abs-path checks, host pattern (CIDR `/N` accepted; `/33` rejected; non-network host bits rejected), path pattern (literal-prefix only — `*`/`**`/`?` rejected), duration parse, env `${VAR}` resolution
- `internal/manifest/suggest.go` — hand-written Levenshtein, distance ≤ 2, multi-candidate listing
- Golden tests: `testdata/{ok,bad-*}.yaml` + `*.golden`; coverage ≥ 90%

**Acceptance.** `go test ./internal/manifest/...` green. `agentctl run nonexistent.yaml` returns formatted error. Items 8 + 9 testable from CLI today.

### Wednesday — Daemon client + read-side commands

**AM (WS-3 daemon client — INTERFACES §2).** Wire the socket talk:
- `internal/daemon/protocol.go` — request/response Go types matching INTERFACES §2.2–§2.8: `RunAgent` / `StopAgent` / `ListAgents` / `AgentLogs` / `StreamEvents` / `DaemonStatus` / `IngestEvent` (`IngestEvent` is for completeness — CLI doesn't call it; orchestrator does)
- `internal/daemon/stream.go` — `WriteFrame(io.Writer, []byte)` and `ReadFrame(io.Reader) ([]byte, error)` mirroring P2's `WriteFrame`/`ReadFrame` byte-for-byte: `[4-byte BE uint32][body]`, 16 MiB cap, oversize → close. Cancellation via `SetReadDeadline` driven by a context-watcher goroutine
- `internal/daemon/client.go` — `Dial(ctx, socketPath) (*Client, error)` with discovery order per DEC-008: `--socket` flag → `AGENT_SANDBOX_SOCKET` env → `$XDG_RUNTIME_DIR/agent-sandbox.sock` → `/run/agent-sandbox.sock`. Unary methods return single response. `StreamEvents` returns `(<-chan Event, <-chan error)` driven by a goroutine that reads frames until EOF/error
- `internal/daemon/errors.go` — typed errors mapping server `error.code` strings → Go sentinels: `var ErrAgentNotFound = errors.New(...)`, `ErrInvalidManifest`, `ErrPermissionDenied`, `ErrCgroupFailed`, `ErrBPFLoadFailed`, `ErrLaunchFailed`, `ErrDaemonUnreachable`, `ErrInternal`. Subcommands use `errors.Is`

**PM (WS-3 mock + WS-5/WS-6 commands).**
- `internal/testutil/mockdaemon.go` — in-process mock over `net.Listen("unix", tmp)` or `net.Pipe`. Programmable: `mock.OnRunAgent(func)`, `mock.PushEvent(Event)`, etc. Drives integration tests without `agentd`
- `internal/daemon/client_test.go` — round-trip every method, including `StreamEvents` mid-stream cancel asserting < 50ms (DEC-007)
- `cmd/run.go` — parse manifest → resolve defaults/`${VAR}`/durations → `Run` request → render `name agent_id pid started_at` + the `--restart-on-crash` / `--max-restarts` flags through `RunAgent.params` per DEC-012
- `cmd/list.go` — `ListAgents` request → `internal/render/table.go` (human columns: NAME / STATUS / PID / UPTIME / POLICY) and `--json` (raw `result.agents`)
- `cmd/stop.go` — `StopAgent{name, grace_period_ns: 5e9}` → human one-liner

**Acceptance.** `go test ./internal/daemon/...` green. `agentctl run/list/stop` happy path against mock works in `go test` and an interactive shell.

### Thursday — Logs streaming (the long pole)

**WS-7 cmd/logs — INTERFACES §2.5/§2.6, DEC-007.**

Long pole because of two intertwined hard problems: streaming a `<-chan Event` while honouring `cmd.Context()`, and a clean Ctrl-C exit without dangling goroutines. Get the bones in by lunch and spend the afternoon on signal handling and tests.

- `cmd/logs.go` — flags `--follow`, `--tail` (int, default 100, mutually exclusive with `--follow`), `--include` (default `llm,kernel,lifecycle` — `agent` opt-in), `--json`. `--follow` calls `StreamEvents`; `--tail` calls `AgentLogs{tail_n: N}`
- Signal handling per DEC-007: root cmd uses `signal.NotifyContext(ctx, SIGINT, SIGTERM)`. `cmd/logs.go` reads from event channel inside `select { case <-ctx.Done(): close conn; return 130 }`
- `internal/render/logs.go` — `<ts>  <category>  <type>  <one-line summary>`. Type-specific summarisers (per cross-team plan §WS-7):
  - `llm.stdout`: 80-char truncated line
  - `llm.tool_call`: `<tool>(args) [<latency>ms, <in>→<out>]?`
  - `llm.tool_result`: `<tool> -> <ok|err> [<latency>ms]?`
  - `llm.stopped` / `llm.crashed`: `exit <code>`
  - `kernel.connect_allowed`: `<dst>:<port> (<hostname>?)`
  - `kernel.connect_blocked`: `<dst>:<port> reason=<reason>`
  - `agent.stdout` / `agent.stderr`: 80-char truncated; `(truncated)` marker if `data.truncated`; only when `--include=agent`
  - `lifecycle.*`: `<details>`
- JSON mode: `--json` re-emits each event as one NDJSON line (renderer-only — wire stays length-prefixed)
- EOF terminator (no end sentinel — DEC-011): renderer prints `--- agent <name> stream closed` in human mode, suppressed in `--json`
- testscript suite under `e2e/testdata/script/`:
  - `logs-follow.txt` — mock emits 50 events, all rendered
  - `logs-ctrl-c.txt` — start `agentctl logs --follow` background, sleep 200ms, `kill -INT %1`, assert exit 130 within **100ms wall-clock**
  - `logs-tail.txt` — `--tail=20` returns last 20 from mock fixture
  - `logs-include.txt` — `--include=kernel` skips llm events
- Soak gate: `//go:build soak` 60s mock-driven test at 1k ev/s asserting `runtime.NumGoroutine` stable. Nightly only — not blocking demo

**Acceptance.** Items 5 + 6 demo-ready. Round-trip `--json` parity with human renderer for all type summarisers.

### Friday — Polish, completions, demo

**AM.**
- `cmd/completion.go` — cobra builtin (`cmd.GenBashCompletion`, `GenZshCompletion`, `GenFishCompletion`). Validation: `bash -n`, `zsh -n`, `fish -c 'source'`
- `examples/{web-fetcher,file-reader,shell-runner,llm-agent,bad-paths,bad-cidr}.yaml` per Friday checklist + verification recipe
- `README.md` — install + 5-minute quickstart pointing at examples
- `go test ./... -count=1 -race` clean

**PM.**
- VM e2e (WS-11) only if Q-009 closed; otherwise document deferred + run testscript locally as evidence and ship
- Friday demo dry-run: walk the 13 checklist items end-to-end on the VM (or laptop fallback)
- Buffer + bug bash

## Critical path

```
Tue AM bootstrap → Tue PM WS-2 → Wed AM WS-3 client → Wed PM WS-5/WS-6 → Thu WS-7 logs → Fri demo
                                       ↑                                       ↑
                                  cliff #1                                cliff #2
                                  (must finish Wed)                       (long pole)
```

If WS-3 isn't end-of-Wednesday, WS-7 starts Thursday afternoon and the 100ms Ctrl-C test gets shaved. **Cut WS-9 polish or VM e2e before cutting WS-7 testing.**

## Risks (concrete, with mitigation)

| Risk | Likelihood | Mitigation |
|---|---|---|
| Tuesday bootstrap eats more than half a day | medium | Keep makeup minimal — typed struct + cobra root only; defer all validation to PM. No abstractions, no helpers. |
| `agentctl logs --follow` hangs on Ctrl-C | medium | DEC-007 `signal.NotifyContext` + `logs-ctrl-c.txt` 100ms-budget test written *before* the renderer is polished. Fail fast. |
| Length-prefix framing edge cases (oversize, half-frame) | low | Mirror P2's `WriteFrame`/`ReadFrame` byte-for-byte; round-trip tests against mock include 16 MiB+1 oversize and partial-read scenarios. |
| P5 Q-009 still open Wed EOD | medium | Document VM e2e as deferred to Week 4; ship with mock-integration evidence in CI. WS-11 is not on the demo checklist gate. |
| Goroutine leak on stream cancel | low | Soak test gated by build tag; `pprof` available via `--verbose` for ad-hoc inspection. |
| Manifest/daemon contract drift mid-week (P2 ships behavior change) | low | INTERFACES.md §2 is mirrored to P2's `proto.md` and frozen as of Q-005 close. Any drift is P2's PR — request it on the daemon side, not in the CLI. |

## File layout to create

```
/workspace/
├── go.mod
├── go.sum
├── README.md
├── cmd/agentctl/
│   ├── main.go              # cobra root, version, --socket flag
│   ├── run.go               # WS-5
│   ├── list.go              # WS-6
│   ├── stop.go              # WS-6
│   ├── logs.go              # WS-7
│   ├── completion.go        # WS-8
│   └── manifest_validate.go # hidden debug subcommand (WS-2)
├── internal/
│   ├── manifest/
│   │   ├── manifest.go      # typed struct
│   │   ├── parse.go         # WS-2 two-pass
│   │   ├── validate.go      # WS-2 field validators
│   │   ├── errors.go        # WS-2 catalog (DEC-005)
│   │   ├── suggest.go       # WS-2 levenshtein
│   │   ├── parse_test.go
│   │   ├── validate_test.go
│   │   └── testdata/        # *.yaml + *.golden
│   ├── daemon/
│   │   ├── protocol.go      # WS-3 wire types
│   │   ├── client.go        # WS-3 Dial + methods
│   │   ├── stream.go        # WS-3 length-prefixed framing
│   │   ├── errors.go        # WS-4 typed errors
│   │   └── client_test.go   # WS-3 round-trip
│   ├── render/
│   │   ├── table.go         # WS-6 list output
│   │   └── logs.go          # WS-7 type-specific summarisers
│   └── testutil/
│       └── mockdaemon.go    # WS-3 mock server
├── e2e/
│   ├── testscript_test.go   # WS-9 driver
│   └── testdata/script/
│       ├── run-ok.txt
│       ├── run-bad-paths.txt
│       ├── run-bad-cidr.txt
│       ├── run-typo.txt
│       ├── list.txt
│       ├── stop.txt
│       ├── logs-follow.txt
│       ├── logs-ctrl-c.txt
│       ├── logs-tail.txt
│       └── logs-include.txt
└── examples/
    ├── web-fetcher.yaml
    ├── file-reader.yaml
    ├── shell-runner.yaml
    ├── llm-agent.yaml
    ├── bad-paths.yaml
    └── bad-cidr.yaml
```

## Concrete starting point — first 30 minutes

```bash
cd /workspace
go mod init github.com/agent-sandbox/cli   # adjust org name
go get github.com/spf13/cobra@v1.10.2 gopkg.in/yaml.v3@v3.0.1 github.com/rogpeppe/go-internal@v1.14.1
mkdir -p cmd/agentctl internal/{manifest,daemon,render,testutil} e2e/testdata/script examples
```

Then in order:
1. `cmd/agentctl/main.go` — root cmd with `version` (ldflags-baked semver + git SHA + literal `protocol v1`)
2. `internal/manifest/manifest.go` — typed struct per INTERFACES §1.1
3. Verify: `go build ./... && ./agentctl version`
4. Move to WS-2 PM work — start with `errors.go` (catalog), then `parse.go`

## Where each WS maps to files

| WS | What | Files |
|---|---|---|
| WS-1 | Lock-ins (DONE) | `planning/p3/week3/INTERFACES.md` |
| WS-2 | Manifest validator | `internal/manifest/*.go` + `testdata/` |
| WS-3 | Daemon client | `internal/daemon/*.go` + `internal/testutil/mockdaemon.go` |
| WS-4 | Error catalog | `internal/manifest/errors.go` + `internal/daemon/errors.go` |
| WS-5 | `cmd/run` | `cmd/agentctl/run.go` |
| WS-6 | `cmd/list`, `cmd/stop` | `cmd/agentctl/{list,stop}.go` + `internal/render/table.go` |
| WS-7 | `cmd/logs` (long pole) | `cmd/agentctl/logs.go` + `internal/render/logs.go` |
| WS-8 | Completions | `cmd/agentctl/completion.go` |
| WS-9 | testscript suite | `e2e/testscript_test.go` + `e2e/testdata/script/` |
| WS-10 | README | `README.md` |
| WS-11 | VM e2e | `.github/workflows/vm-e2e.yml` (or deferred if Q-009 still open) |

## Out of scope (stays Week 4)

- `--retry` for transient stream errors (Q-003)
- `--wait` readiness signal (Q-004)
- Resumable replay (`--since=<seq>`) — `tail_n` only in v1
- Man pages, install script, goreleaser, deb package
- Dynamic completion (TAB-completing agent names by querying daemon)
- Multi-agent batch (`agentctl run *.yaml`)
- Telemetry beyond `--verbose`

## Definition of done

All 13 Friday-checklist items pass live in front of the team **and** the verification recipe at `P3_WEEK3_PLAN.md` §"Verification recipe" runs clean on a fresh VM clone.
