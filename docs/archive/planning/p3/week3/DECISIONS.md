# P3 Week 3 — Decisions (ADRs)

Week-3-scoped architectural decisions. Each entry: Context / Decision / Consequences / Alternatives. New decisions append at the bottom; superseded ones are marked, not deleted.

---

## DEC-001 — Streaming wire format: NDJSON over Unix socket *(superseded by DEC-011 for the daemon socket)*

**Status.** Superseded by **DEC-011** for the CLI↔daemon Unix socket: P2's existing daemon ships length-prefixed framing (4-byte BE length + UTF-8 JSON, 16 MiB cap) and we are mirroring it rather than asking P2 to flip. NDJSON is retained downstream only for P5's WebSocket fan-out — browser WebSocket clients are line-oriented, so the bridge re-encodes each frame as one NDJSON line. This entry is preserved for context; do not implement against it.

**Context.** `agentctl logs` streams events from the daemon. The transport is a Unix domain socket [#10]. We need a framing scheme that is (a) trivial to write on both sides, (b) fully streamable line-by-line, (c) self-synchronising on reconnect, (d) easy to inspect with `cat` during debugging.

**Decision.** Newline-delimited JSON. One JSON object per line, each terminated by `\n`. Server flushes after each event. Client uses `json.Decoder` over the socket — it is itself a streaming decoder and tolerates whitespace between objects [#8 stdlib `encoding/json`].

**Consequences.**
- Reading: `dec := json.NewDecoder(conn); for { var ev Event; if err := dec.Decode(&ev); err != nil { break } ... }`.
- Cancellation: client cancels by closing the connection (half-close after writing nothing more, or full close on Ctrl-C). Server detects via `Read` returning `io.EOF` or `net.ErrClosed`.
- Trade-off accepted: NDJSON has no length prefix, so a malformed line corrupts the stream. We accept that — the only producers are our own daemon and our own orchestrator.

**Alternatives rejected.**
- **Length-prefixed JSON (RFC 7464 / `\x1e` record separator)**: more robust to corruption, harder to inspect with `cat`. Not justified here.
- **gRPC streaming**: heavyweight for a 4-week project; pulls in protobuf toolchain. Documented as a v2 option.
- **Plain text lines**: kills `--json` output and the P5 web UI's typed-event consumption.

---

## DEC-002 — Manifest YAML library: `gopkg.in/yaml.v3 v3.0.1`

**Context.** Manifest UX requires line/column-aware error messages [mission §"Week 3 scope" bullet 5]. Two real candidates:
- `gopkg.in/yaml.v3` v3.0.1 [#2]
- `sigs.k8s.io/yaml` v1.6.0 [#3]

`sigs.k8s.io/yaml` documentation explicitly states "no compatibility guarantees for returned error values" [#3]. That kills our line/column UX commitment.

**Decision.** Use `gopkg.in/yaml.v3 v3.0.1`. The library exposes `yaml.Node.Line` and `yaml.Node.Column` directly, which is the foundation of DEC-003 (two-pass parse).

**Consequences.**
- Pin `gopkg.in/yaml.v3 v3.0.1` in `go.mod`.
- Caveat: upstream `gopkg.in/yaml.v3` is archived as of 2026; the maintained successor is `go.yaml.in/yaml/v3` [#9] (drop-in API). Tracked as **OPEN-Q-001**: confirm with team Day 1, switch later if desired (one-line `go.mod` change).

**Alternatives rejected.** `sigs.k8s.io/yaml` (error opacity), `goccy/go-yaml` (less battle-tested in our reference projects, third-party error-position API).

---

## DEC-003 — Manifest validation: two-pass parse via `yaml.Node`

**Context.** `yaml.TypeError` only exposes `Errors []string`; line/column must be parsed from strings. That is brittle and we have a hand-written-error-message commitment [mission §"Error message catalog"].

**Decision.** Two passes:
1. `yaml.Unmarshal(data, &root)` where `root` is `*yaml.Node`. Always succeeds for syntactically valid YAML; errors here are pure-YAML syntax errors and we extract Line/Column directly from the YAML library's error string with a documented regex (one place, one regex, tested).
2. Walk the document mapping. For each top-level key:
   - If unknown, emit `manifest.yaml:<line>:<col>: unknown field 'X' (did you mean 'Y'?)` using the **node's own Line/Column** — no string parsing.
   - If known, validate the value's kind (scalar/sequence/mapping) and content per the schema.
3. Once the walk passes, call `node.Decode(&manifest)` with `Decoder.KnownFields(true)` set on a `yaml.NewDecoder` against a `bytes.Reader` of the original YAML — this catches nested unknown fields (e.g. typo inside `env:`).

**Consequences.**
- All hand-written errors carry exact line/column.
- Adding a manifest field is two changes: schema struct + the walker's known-keys list. The walker is the one place that learns about new fields.
- Did-you-mean suggestions use Levenshtein distance ≤ 2 against the known-keys set; standard `agnivade/levenshtein` or a 30-line stdlib implementation.

**Alternatives rejected.**
- Single-pass `Decode` with regex on `yaml.TypeError.Error()`: brittle, breaks when yaml.v3 changes its error format.
- `Decoder.KnownFields(true)` alone: catches unknown keys but its error message format is generic and lacks did-you-mean.

---

## DEC-004 — `--json` is the canonical machine output; human output is best-effort

**Context.** Every `agentctl` subcommand has a human and a `--json` mode. We need a rule for what to optimise.

**Decision.** `--json` output is the normative wire format and what tests assert against. Human output is rendered from the same in-memory data structure but is best-effort tabular. This means:
- Schema changes are made in `--json` first; human renderer follows.
- Tests pin `--json` output exactly (golden files); human output is asserted with regex / line-count only.
- Errors are JSON-shaped in `--json` mode, with code/message/field/path/line/column. Human errors are formatted from the same struct.

**Consequences.** Adding a field to `agentctl list` output is a one-place change to the JSON struct. The human-table renderer adapts. Scripts that consume `--json` are stable; scripts that scrape human output are not, and we do not promise stability there.

**Alternatives rejected.** "Human-first, JSON is a side feature": drives schema decisions by aesthetics, breaks scripts. "Both equally canonical": doubles the test surface for no real-world benefit — nobody scrapes human tables in 2026.

---

## DEC-005 — Error message catalog: hand-written, table-driven

**Context.** Mission requires hand-written messages, not generic schema errors, for common manifest mistakes [mission §"Error message catalog"].

**Decision.** A `cmd/manifest/errors.go` file holds an enum of validation failure codes and a table mapping `code -> message template`. The walker (DEC-003) emits `(code, field, line, column)`; the renderer formats. Templates use `{{.Field}}`, `{{.Suggestion}}`, etc.

**Consequences.** Adding an error message is one PR with one new template. The catalog is a fixed list maintained in `cmd/manifest/errors.go` and mirrored in this repo as a doc fixture for review by P1/P2/P4 (they will hit these in their own integration tests).

**Alternatives rejected.** Inline `fmt.Errorf` strings: drift, no review surface, no `--json` machine code. Generic schema errors: fails the mission UX bar.

---

## DEC-006 — Completion install paths: emit, do not install

**Context.** Mission requires shell completions for bash/zsh/fish [mission §"Week 3 scope" bullet 6].

**Decision.** Use `cobra.Command.InitDefaultCompletionCmd()` [#4]. Users run `agentctl completion bash > /etc/bash_completion.d/agentctl` etc. We do **not** ship an installer that writes to those paths in Week 3 — that's Week 4 packaging. The `completion` subcommand's `--help` text (cobra-generated) lists the install commands per shell. We override the default help footer to add a one-line "see `agentctl completion <shell> --help` for install instructions" to the root.

**Consequences.** Zero filesystem writes from `agentctl completion`. Easy to test (golden output of the generated completion script). Defers OS-specific install logic to packaging (Week 4).

**Alternatives rejected.** Shipping our own installer that writes to system paths: needs sudo, varies by distro, brittle, out of Week 3 scope.

---

## DEC-007 — Signal handling: `signal.NotifyContext` at root, `cmd.Context()` everywhere else

**Context.** `agentctl logs --follow` must exit cleanly on Ctrl-C. Other subcommands should also be cancel-safe.

**Decision.**
```go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()
    if err := cmd.NewRoot().ExecuteContext(ctx); err != nil { os.Exit(mapExitCode(err)) }
}
```
Every subcommand's `RunE` reads `cmd.Context()` and threads it into the daemon client. The daemon client uses the context to (a) abort `Dial` via `net.Dialer{Timeout, ...}.DialContext`, (b) abort streaming reads by setting a past `SetReadDeadline` when ctx fires.

**Consequences.** One signal handler, no goroutine leaks. Test pattern: under testscript, `kill -INT mycmd` triggers the same path [#7].

**Alternatives rejected.** Per-subcommand signal channels (duplicates code, easy to forget). Letting cobra handle signals (cobra ships no signal helpers per [#4]).

---

## DEC-008 — Daemon socket discovery order

**Context.** Where does `agentctl` find the daemon socket?

**Decision.** Resolution order, first hit wins:
1. `--socket <path>` flag on the subcommand.
2. `AGENT_SANDBOX_SOCKET` environment variable.
3. `$XDG_RUNTIME_DIR/agent-sandbox.sock` if `XDG_RUNTIME_DIR` is set (development).
4. `/run/agent-sandbox.sock` (production, root-owned, mode 0600).

If no socket is found, error code `daemon_unreachable` with the resolved candidate path the client tried last. Path and env-var name align with P2's daemon (`daemon/api/proto.md`).

**Consequences.** Single env var to override in CI / VM tests. Co-developer environments don't conflict (each dev's `XDG_RUNTIME_DIR` is per-session).

**Alternatives rejected.** Hardcoded `/var/run`, no override (kills dev/CI). Config file discovery (overkill for Week 3).

---

## DEC-009 — Exit code table

**Context.** Scripts need stable exit codes.

**Decision.**

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Generic error (last-resort fallback) |
| 2 | Usage error (cobra-default for bad flags) |
| 3 | Manifest parse / validation failed |
| 4 | Daemon unreachable |
| 5 | Daemon returned an error (server-side validation, name in use, etc.) |
| 6 | Agent named in `stop`/`logs` does not exist |
| 130 | Interrupted by SIGINT (canonical 128 + SIGINT=2; matches shell convention) |

The `mapExitCode` function in `cmd/exit.go` translates error types to these codes. Tests assert via `testscript.Params` exit-code checks.

**Alternatives rejected.** Single nonzero code for all errors: scripts can't branch. Custom 100+ codes for every subcase: nobody remembers them.

---

## DEC-010 — Logs replay: file-backed tail, scope is the daemon's

**Context.** Where do historical events live for `agentctl logs --tail=N`?

**Decision.** **In the daemon's per-agent log file**, not in the CLI and not in a ring buffer. P2's daemon writes every event to `/var/log/agent-sandbox/<agent-id>.log` via a `rotatingWriter` (default 10 MiB × 3 retained per agent — ~30 MiB ceiling). The CLI's `agentctl logs --tail=N` invokes `AgentLogs{tail_n: N}` and the daemon scans the log file from the tail. Resumable follow (`--since=<ts>` or `--since=<seq>`) is **not** in v1; CLI surfaces only `--tail` for replay.

**Consequences.**
- `agentctl logs --tail=N` is the only replay surface in Week 3.
- `agentctl logs --follow` calls `StreamEvents` which delivers events from the moment of subscription forward; no replay.
- A follower that disconnects and reconnects misses events that fired while disconnected. Acceptable for v1 demo. Resumable follow (server-side `seq` numbering) is a v0.2 follow-up; P2 has agreed to add a `seq` field to the event header if we want it (cheap).
- Eviction is governed by the rotating writer — older events are lost when the file rotates past the third generation.

**Alternatives rejected.** In-memory ring buffer in the daemon (P2 already has the file-backed writer; no point parallelising). Client-side buffer (defeats the purpose; the CLI is short-lived). Full-retention disk persistence in v1 (Week 4 problem at earliest).

---

## DEC-011 — Daemon socket framing: 4-byte big-endian length prefix

**Context.** Replaces DEC-001 for the CLI↔daemon Unix socket. P2's existing daemon already implements length-prefixed framing in `WriteFrame`/`ReadFrame` per `daemon/api/proto.md` and `architecture.md`: `[4-byte BE uint32: body_len][body_len bytes: UTF-8 JSON]`, with a hard `maxFrameBytes = 16 MiB` cap. NDJSON would force a re-implementation of code that already exists, lose the size cap, and complicate partial-read handling on streaming methods.

**Decision.** Length-prefixed framing on the Unix socket. The CLI's `internal/daemon/stream.go` provides `WriteFrame(io.Writer, []byte) error` and `ReadFrame(io.Reader) ([]byte, error)` that mirror P2's helpers byte-for-byte. Streaming methods (`StreamEvents`) deliver one frame per event; the server closes the connection on agent exit or daemon shutdown — there is no `{"type":"end"}` sentinel. The client treats EOF as the stream terminator.

**Consequences.**
- The CLI consumes `daemon/api/proto.md` as the source of truth; INTERFACES §2 mirrors it for the P3 planning view.
- 16 MiB is enforced on both sides; receivers reject and close on oversize. Per-event server write has a 1-second timeout — a stalled client gets dropped, not held (P2's contract).
- NDJSON survives in exactly one place: P5's WebSocket bridge re-encodes the frames into `\n`-separated JSON for browser clients (browser WebSocket APIs are line-oriented). That is P5's transport, not the daemon's.
- Cancellation discipline (DEC-007) is unchanged: a cancelled context still calls `SetReadDeadline(time.Now())` to unblock `ReadFrame`.

**Alternatives rejected.** NDJSON on the Unix socket (DEC-001 original): rejected after P2's counter on OPEN-Q-005 — P2 already ships length-prefixed; aligning the CLI is one-time, flipping the daemon would propagate. Plain text or RFC 7464: not a serious option once length-prefixed is shipping.

---

## DEC-012 — Restart policy: CLI flags, not manifest fields

**Context.** P4's interface document originally proposed `restart_on_crash` and `max_restarts` as manifest fields. After OPEN-Q-012 closed Day 1, restart policy lives in P4's orchestrator state machine (`_monitor_loop` in `core.py`), not in the manifest schema. P3's CLI exposes the policy as `agentctl run` flags so an operator can override per-invocation without editing the YAML.

**Decision.** Two new flags on `agentctl run`:
- `--restart-on-crash` (boolean, default false): instructs the orchestrator to relaunch the agent if it exits non-zero.
- `--max-restarts <int>` (default 3): caps automatic restarts per agent name. Ignored unless `--restart-on-crash` is set.

The CLI passes these to the daemon in the `RunAgent.params` envelope as fields adjacent to the manifest body (`restart_on_crash: bool`, `max_restarts: int`). The daemon stores them on the agent record; the orchestrator (which subscribes via `StreamEvents`) consults them on `lifecycle.exited` / `llm.crashed`.

**Consequences.**
- Manifest schema stays minimal — no restart fields, no schema bump for a policy decision.
- Operator can flip restart policy without editing the manifest; CI scripts pin via flags.
- Default-off matches the safety intuition (Week-3 demo agents are short-lived; no implicit restart).

**Alternatives rejected.** Manifest fields (rejected by OPEN-Q-012 — orchestrator state, not policy declaration). Per-agent config file (overkill for two flags).
