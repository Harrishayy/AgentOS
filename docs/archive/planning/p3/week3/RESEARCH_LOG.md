# P3 Week 3 — Research Log

Append-only. UTC timestamps. Each entry: source, version/SHA, one-line conclusion.

---

## 2026-04-27

### 11:00 — Cobra current version
- Source: `https://proxy.golang.org/github.com/spf13/cobra/@latest` (Go module proxy, authoritative).
- Version: **v1.10.2**, published 2025-12-03.
- Conclusion: pin `github.com/spf13/cobra v1.10.2` in `go.mod`. Cross-checked against pkg.go.dev. (See SOURCES.md #1, #4.)

### 11:00 — Cobra completion API surface
- Source: pkg.go.dev `github.com/spf13/cobra` (v1.10.2 godoc).
- Confirmed signatures:
  - `(c *Command) GenBashCompletionV2(w io.Writer, includeDesc bool) error` — `bash_completionsV2.go`, added v1.2.0.
  - `(c *Command) GenZshCompletion(w io.Writer) error` — `zsh_completions.go`.
  - `(c *Command) GenFishCompletion(w io.Writer, includeDesc bool) error` — `fish_completions.go`.
  - `(c *Command) GenPowerShellCompletionWithDesc(w io.Writer) error` — `powershell_completions.go`.
- `(c *Command) InitDefaultCompletionCmd(args ...string)` auto-adds a `completion` subcommand that dispatches to the right generator and emits install instructions for each shell.
- Conclusion: use `InitDefaultCompletionCmd` in the root command rather than rolling our own. Users run `agentctl completion bash|zsh|fish` and pipe to the install path. (See SOURCES.md #4.)

### 11:01 — Cobra context propagation
- Source: pkg.go.dev `github.com/spf13/cobra` (v1.10.2 godoc).
- `(c *Command) ExecuteContext(ctx context.Context) error` and `cmd.Context()` inside `RunE` are the idiomatic plumb-through. Cobra ships **no** signal helpers — use `os/signal` directly.
- Conclusion: `main()` builds a `signal.NotifyContext` and passes it to `rootCmd.ExecuteContext`. All long-running subcommands (today: `logs --follow`) read `cmd.Context()` and exit on `Done`.

### 11:05 — `os/signal.NotifyContext` confirmed
- Source: pkg.go.dev `os/signal`.
- Signature: `func NotifyContext(parent context.Context, signals ...os.Signal) (ctx context.Context, stop context.CancelFunc)`. Added Go 1.16.
- Cancel cause is retrievable via `context.Cause(ctx)` — useful for distinguishing user-Ctrl-C from EOF.
- Conclusion: canonical pattern for `agentctl logs --follow` is `ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM); defer stop()` then propagate `ctx` to the daemon-stream reader. (See SOURCES.md #6.)

### 11:10 — gopkg.in/yaml.v3 version + maintenance status
- Source: `https://proxy.golang.org/gopkg.in/yaml.v3/@latest`.
- Version: **v3.0.1**, published 2022-05-27. No new tags since.
- Cross-check (WebSearch, 2026-04-27): `gopkg.in/yaml.v3` is archived; the maintained fork is `go.yaml.in/yaml/v3` (same API surface).
- Conclusion: default to `gopkg.in/yaml.v3 v3.0.1` per mission instruction. Flagged as **OPEN-Q-001**: confirm with team whether to migrate to `go.yaml.in/yaml/v3` Week 3 or defer to Week 4 polish. Note: `sigs.k8s.io/yaml` is **not** suitable here — its docs explicitly state "no compatibility guarantees for returned error values," which kills our line/column UX. (See SOURCES.md #2, #3.)

### 11:15 — yaml.v3 line/column error extraction
- Source: pkg.go.dev `gopkg.in/yaml.v3`.
- `yaml.TypeError` exposes only `Errors []string` — line/column must be parsed from strings. Brittle.
- `yaml.Node` exposes `Line int` and `Column int` directly. After `yaml.Unmarshal(data, &node)`, every node carries its position.
- `Decoder.KnownFields(true)` rejects unknown mapping keys against the destination struct.
- Conclusion: **two-pass parse strategy**:
  1. Unmarshal into `yaml.Node`. Walk the document mapping; for every key that isn't in the manifest schema, emit a hand-written `<file>:<line>:<col>: unknown field 'X' (did you mean 'Y'?)` error using the node's own Line/Column.
  2. `node.Decode(&manifest)` with `KnownFields(true)` on a `Decoder` to catch nested mismatches.
- This is the **only** reliable way to hit the mission's error-UX bar. Documented as **DEC-003**.

### 11:20 — sigs.k8s.io/yaml comparison
- Source: pkg.go.dev `sigs.k8s.io/yaml` v1.6.0 (2025-07-24).
- Wraps go-yaml v2 primarily; supports JSON struct tags. Explicit "no compatibility guarantees for returned error values" disqualifies it for our manifest UX.
- Conclusion: rejected for the manifest parser. Documented in DEC-002. Could be reconsidered if we later need JSON struct tag reuse for shared types, but that's a v2 problem.

### 11:25 — testscript (rogpeppe/go-internal)
- Source: `https://proxy.golang.org/github.com/rogpeppe/go-internal/@latest` and pkg.go.dev `github.com/rogpeppe/go-internal/testscript`.
- Version: **v1.14.1**, published 2025-02-25.
- Pattern:
  ```go
  func TestMain(m *testing.M) {
      testscript.Main(m, map[string]func(){"agentctl": cmd.Main})
  }
  func TestCLI(t *testing.T) {
      testscript.Run(t, testscript.Params{Dir: "testdata/script", RequireExplicitExec: true})
  }
  ```
- Built-ins relevant to streaming: `exec ... &name&` (background), `wait name`, `kill -INT name`, `stdout/stderr` regex match.
- Conclusion: testscript is the right harness for end-to-end CLI tests including signal/streaming. Mock-daemon tests live alongside. (See SOURCES.md #5.)

### 11:30 — NDJSON streaming over Unix socket
- Sources: stdlib `bufio` (Scanner default 64 KiB token, `Buffer` to grow), `encoding/json` (`json.Decoder.Decode` reads a single JSON value at a time and is itself stream-safe), `net` (`UnixConn`, `SetReadDeadline`).
- Convention chosen: newline-delimited JSON. Each event is one line, one JSON object, terminated by `\n`. Server flushes per event.
- Read side: `dec := json.NewDecoder(conn)` then `for { var ev Event; if err := dec.Decode(&ev); ... }`. `json.Decoder` tolerates trailing whitespace/newlines between objects, so NDJSON is read directly.
- Cancellation: writer goroutine watches `ctx.Done()`; on cancel, sets a past `SetReadDeadline` to unblock the decoder, returns `io.EOF` cleanly.
- Conclusion: NDJSON is the wire format for `logs` streaming and the protocol-default unless P2 chooses otherwise (DEC-001). The exact framing must be locked Day 1 with P2.

### 11:35 — Real-world reference clients
- kubectl `logs --follow`, nerdctl `logs --follow`: GitHub blob fetches timed out; both use `signal.NotifyContext` and a streaming HTTP body / containerd stream. Pattern is well-documented in Go community examples; `os/signal.NotifyContext` documentation already cites the canonical pattern (RESEARCH 11:05).
- Conclusion: do not block on direct source citation; the mental model (`Ctrl-C cancels context, context cancels read, server side closes stream cleanly`) is settled. Logged as **OPEN-Q-007** if anyone disputes this during code review.
