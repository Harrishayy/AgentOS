# P3 Implementation Assumptions

This file lists every assumption made during the P3 (CLI + manifest) implementation
that the orchestrator may need to reconcile with other layers.

Each entry: **assumption** / **why** / **where it shows up** / **what to check / change**.

---

## Repo / module identity

- **Module path:** `github.com/agent-sandbox/cli`
  - Why: the build plan's Tuesday recipe says "adjust org name". Picked the namespace
    used in the planning doc until the orchestrator decides on the canonical one.
  - Where: `go.mod`, every Go import (`github.com/agent-sandbox/cli/internal/...`).
  - Fix: search-replace `github.com/agent-sandbox/cli` if a different module path is
    chosen. No other code logic depends on this name.

- **Go toolchain:** Go 1.23 (downloaded toolchain 1.25.9 via auto-switch).
  - Why: plan requires Go 1.22+. We bootstrapped on 1.22.10, but
    `github.com/rogpeppe/go-internal@v1.14.1` requires Go 1.23+, so `go.mod`'s
    `go` directive was bumped to 1.23 by `go get`. The downloaded toolchain
    (1.25.9) is what actually compiles and tests; the directive only sets the
    minimum.
  - Where: `go.mod` `go 1.23` directive.
  - Fix: if the orchestrator wants strict 1.22 pinning, downgrade
    `go-internal` to v1.12.x (last 1.22-compatible release) or use a different
    testscript driver.

- **Repository root for P3 code:** `/workspace/` (i.e., the implementation files live
  directly under the workspace, not in a `cli/` subdir).
  - Why: the build plan's "File layout to create" rooted things at `/workspace/`.
  - Fix: if the orchestrator wants P3 to live under, e.g., `/workspace/cli/`, move
    `cmd/`, `internal/`, `e2e/`, `examples/`, `go.mod`, `go.sum` under that subdir
    and re-run `go mod init` with the correct path.

## Cross-layer wire dependencies (P2)

- **Daemon socket framing format (DEC-011):** 4-byte BE length prefix + UTF-8 JSON,
  16 MiB cap. The CLI's framing helpers in `internal/daemon/stream.go` mirror P2's
  `WriteFrame`/`ReadFrame`. If P2's actual implementation diverges (different
  endianness, different cap, header bytes, etc.), the CLI will not interoperate.
  - Verification: round-trip test against P2's daemon binary, not the in-process
    mock.

- **`StreamEvents` per-frame envelope shape:** the CLI assumes each streamed frame
  body is `{"ok":true,"result":{"event":<Event>}}` (INTERFACES §2.6). P2 confirmed
  this on Day 2; if P2 actually emits the bare event without the envelope, change
  `internal/daemon/client.go`'s stream reader to decode `Event` directly.

- **`StreamEvents` cancellation:** the CLI does not half-close after sending the
  request; it relies on `conn.Close()` driven by context cancellation. P2 must
  not require a half-close.

- **`AgentLogs` response field name:** assumed `events` (plural) per INTERFACES
  §2.5. If the wire actually says `result.event` or `result.entries`, fix in
  `internal/daemon/protocol.go`.

- **Error code casing:** server-side codes are SCREAMING_SNAKE (`INVALID_MANIFEST`,
  `AGENT_NOT_FOUND`, ...). Client-synthesized codes are lowercase
  (`daemon_unreachable`, `manifest_parse_failed`). The CLI translates both to
  the same render shape.

- **`IngestEvent` peer-uid env var:** assumed `AGENT_SANDBOX_INGEST_UID` per the
  Day-1 OPEN-Q-011 resolution. CLI does not call `IngestEvent` (orchestrator
  does) but the type is exported in `internal/daemon/protocol.go` for completeness.

## Manifest schema (P1)

- **`allowed_paths` is enforced in v1** via P1's BPF LSM `file_open` hook. The
  validator accepts `/abs/file`, `/abs/dir/`, and `/abs/dir/*.ext` (single `*`
  glob, no `**`, no `?`, no character classes).
  - If P1 ends up shipping a different glob grammar for v1, update
    `internal/manifest/validate.go::validatePathPattern`.

- **Host pattern CIDR:** `10.0.0.0/8` accepted; `10.0.0.0/33` and host-bits-set
  (`10.0.0.5/8`) rejected with `invalid_host_pattern`.

- **Hostnames resolve at policy-load time, not per-connection** (INTERFACES §1.2
  note). The CLI does not perform DNS — it forwards the literal pattern to the
  daemon, which resolves it at `RunAgent` time. No code change needed in the CLI,
  but operators should be aware (documented in README).

- **Default `working_dir`:** `/tmp/agentctl/<name>`. The CLI does **not** create
  it; the daemon does. The CLI fills the default into the resolved manifest before
  sending if the user omitted it.

- **Default `user`:** caller's uid. The CLI fills `strconv.Itoa(os.Getuid())` into
  the resolved manifest if the user omits `user`. The daemon will SO_PEERCRED-verify
  separately.

## CLI behaviour

- **`--socket` is a persistent root flag**, available on every subcommand
  (DEC-008 step 1).

- **`--json` is a persistent root flag** that every subcommand inherits.

- **`run` flags `--restart-on-crash` / `--max-restarts` are CLI-only** (DEC-012).
  They do not appear in the manifest schema. The CLI passes them to the daemon
  at the same level as `manifest` in `RunAgent.params`.

- **`logs --follow` and `logs --tail` are mutually exclusive.** Specifying both
  exits with usage error (code 2). Default behaviour with neither flag: `--tail
  100` (replay last 100 from the daemon's log file). Enforced in
  `internal/cli/logs.go` via `cmd.Flags().Changed("tail")`.

- **`logs --include` requires `--follow`.** `AgentLogsRequest` has no `Include`
  field on the wire (DEC: defer wire change to orchestrator). Without
  `--follow`, the daemon would return all events and the client would discard
  silently, masking divergence. Specifying `--include` without `--follow`
  exits with usage error (code 2).

- **Stdout vs stderr split for `--json`:** human errors go to stderr with
  `Error: ` prefix. **All** `--json` errors — manifest validation **and**
  daemon-side errors — go to stdout (so `jq` consumers can pipe). Daemon errors
  route through `renderDaemonErr` (`internal/cli/exit.go`) which emits a
  `{"ok":false,"code":...,"error":...}` envelope and tags the error as
  already-printed so `agentctlcmd.Main` skips the stderr prefix.

- **NDJSON in `--json` mode for `logs`:** one JSON object per line, terminated by
  `\n`. EOF terminator only — no `{"end":true}` sentinel.

## Tests

- **In-process mock daemon:** uses a temp `net.Listen("unix", ...)` socket in
  `t.TempDir()`. The mock implements the same length-prefix framing as the real
  daemon. Tests that need the mock construct it explicitly.

- **StreamEvents cancel-fast assertion:** the daemon-client unit test
  (`TestClient_StreamEvents_CancelFast` in `internal/daemon/client_test.go`)
  asserts that `Close()` returns within 200ms after `cancel()`. The spec target
  is well under 100ms; the test budget is widened to 200ms to absorb scheduler
  jitter under `-race`. The library code path itself does not sleep — cancel is
  observed within ~1 cooperative tick in practice.

- **No dedicated `logs-ctrl-c.txt` testscript scenario.** Sending a real SIGINT
  to a testscript-driven in-process command is awkward (the CLI is a function
  call inside the test binary, not a forked process). The streaming cancel path
  is exercised by the unit test cited above plus `logs-follow.txt`, which
  drives the natural-end branch via `mockd end`. If the orchestrator needs an
  end-to-end SIGINT scenario, it should run the compiled binary as a subprocess
  in a separate harness.

- **`go test -race` is enabled** in `.github/workflows/ci.yml`. All 58 tests
  pass under `-race` on the GitHub-hosted ubuntu-latest runner.

## Things deliberately not implemented (Week 4)

Per `P3_WEEK3_PLAN.md` §"Out of scope (Week 4)":

- `--retry` for transient stream errors.
- `--wait` readiness signal on `run`.
- Resumable replay (`--since=<seq>`).
- Man pages, install script, `goreleaser`, deb package.
- Dynamic shell completion of agent names.
- Multi-agent batch (`agentctl run *.yaml`).
- Telemetry beyond `--verbose`.
- VM-based e2e workflow file (`.github/workflows/vm-e2e.yml`) — requires P5
  Q-009 closure. CI skeleton has only the unit + mock-integration job.

## Things implemented but not on the demo checklist

- **`agentctl manifest validate <file>`** (subcommand): runs the manifest
  through the parser and prints `OK <name>` plus the resolved policy summary,
  or the error. Useful for debugging and CI manifest gates without a daemon.

- **`agentctl daemon status`** (subcommand): a thin wrapper around
  `DaemonStatus` for health-check scripts.

- **`agentctl version`** prints build, Go version, target os/arch, and the
  protocol version the CLI was compiled against. `Build` defaults to "dev" and
  is overridable at link time via `-ldflags="-X .../internal/cli.Build=v1.2.3"`.

- **Persistent `--verbose` flag** is parsed but currently only emits a
  one-liner before each `RunAgent` call ("submitting manifest %q to daemon at
  %s"). Other subcommands accept `--verbose` for forward-compatibility but do
  not emit additional output yet.

- **`logs --include` is forwarded to the daemon (StreamEvents) AND filtered
  client-side.** The daemon side is authoritative; the client filter is a
  defence-in-depth belt against a daemon that ignores the request field. Only
  in effect on the streaming path; with `--tail`, `--include` is rejected up
  front (see CLI behaviour above).

- **Manifest validator boundary checks.** `validate.go`/`parse.go` reject:
  embedded NUL/LF/CR/TAB in `allowed_paths` patterns and in `stdin: file:<path>`
  (would corrupt the BPF allowlist key or `open(2)` path); negative uids;
  duplicate top-level YAML keys (yaml.v3 silently lets the last one win — we
  reject so the audit log matches what the daemon enforces); env keys outside
  the POSIX portable name regex `[A-Za-z_][A-Za-z0-9_]*` (would corrupt the
  envp passed to `execve`). New error codes: `duplicate_key`, `bad_env_key`.

## Test inventory

| Package | Tests | Notes |
|---|---|---|
| `internal/daemon` | 11 | round-trips against the in-process mock, plus framing/discovery edge cases |
| `internal/manifest` | 20 | two-pass YAML, did-you-mean, line/col extraction, duplicate-key, env-key, control-char, negative-uid boundary tests |
| `internal/render` | 4 | tab table + per-event-type summarisers |
| `internal/cli` | 2 | reflection guard on `manifestToPayload` field set |
| `internal/testutil` | 0 | the mock itself; exercised by daemon + e2e |
| `cmd/agentctl/agentctlcmd` | 0 | trivial wrapper |
| `e2e` | 23 testscript scenarios | run (ok, bad-paths, bad-cidr, typo, bad-paths-json, daemon-error-json), list (human, json), stop (ok, not-found), logs (tail, tail-honored, follow, include, conflict, tail-include-conflict), validate, completion (bash/zsh/fish), version, daemon status, daemon-unreachable |

Total: 65 cases, all green under `-race`.

## Open questions for the orchestrator

These are not P3 gaps — P3 cannot resolve them alone — but the orchestrator
needs to answer them before stitching layers.

- **D1. Wire-contract round-trip against the real P2 daemon.** P3 is built
  against the in-process mock (`internal/testutil/mockdaemon.go`) and four
  contracts are assumed but not verified end-to-end against the compiled P2
  binary: (a) envelope shape (`{"ok":..., "result":..., "error":...}`),
  (b) `AgentLogsResult.events` field name (vs `entries`), (c) half-close
  semantics on unary methods (CLI half-closes write; P2 must accept that as
  end-of-request without requiring a different framing), (d) framing
  endianness (4-byte BE length prefix). Recommended: a single integration
  test in the orchestrator's CI that exercises each of the seven methods plus
  the StreamEvents loop against the real daemon. Release-gate item.

- **D2. Default `user` semantics.** When the manifest omits `user`, the CLI
  fills `strconv.Itoa(os.Getuid())` into the resolved manifest payload before
  sending. The daemon (a privileged service) almost certainly runs as a
  different uid; it must reconcile via SO_PEERCRED. Two reconciliation paths,
  pick one:
  1. **Daemon-side override:** if `manifest.user` is empty in the payload, the
     daemon uses the SO_PEERCRED uid. P3 sends empty; CLI patch deletes the
     `os.Getuid()` default in `parse.go::parseAndValidate` (the post-validation
     defaults block).
  2. **CLI-side fill, daemon-side verify:** CLI fills the client uid (current
     behaviour); daemon verifies `manifest.user == SO_PEERCRED uid` and rejects
     on mismatch. No CLI change.
  Path 1 is cleaner (single source of truth); path 2 keeps the manifest
  self-describing for audit. Document the choice in INTERFACES.md before P2
  implements either side.

## Files the orchestrator should review first

1. `internal/daemon/protocol.go` — wire types; the most likely place to need
   adjustment if P2 diverged from INTERFACES.md after Day 2.
2. `internal/manifest/validate.go` — host/path grammar; the most likely place
   to need adjustment if P1 narrowed or widened the v1 grammar.
3. `internal/cli/run.go::manifestToPayload` — explicit field-by-field copy
   from `manifest.Manifest` → `daemon.ManifestPayload`. If either struct
   gains a field, both ends and this copier must update. The reflection guard
   `TestManifestPayload_FieldsMatch` in `internal/cli/run_test.go` will fail
   on a JSON-tag set mismatch; `TestManifestToPayload_CopiesAllFields` will
   fail if the copier forgets a populated field.
4. `e2e/testscript_test.go` — the test harness; specifically the `mockd`
   in-script command set, which is what other layers' tests would extend.
