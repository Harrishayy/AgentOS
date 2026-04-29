# P3 Gap Review — Pre-Orchestration Fixes

## Context

P3 (CLI + manifest) is functionally complete: 8 subcommands, 58 passing tests
under `-race`, 17 e2e scenarios, full README + ASSUMPTIONS.md. Before the
orchestrator merges P3 with P1 (BPF sandbox) and P2 (daemon), this review
identifies concrete gaps the orchestrator would otherwise hit. Three Explore
agents flagged candidates; each item below is verified against the code.

Two distinct goals:
1. Close documented-contract violations and validator-boundary holes inside P3
   (we control these).
2. Surface cross-layer questions for the orchestrator hand-off (we cannot
   resolve these alone).

---

## Group A — CLI behavior diverges from documented contracts

### A1. `logs --tail` default is 50, expected 100
- Evidence: `internal/cli/logs.go:42` — `IntVar(&tail, "tail", 50, ...)`.
- Spec: `ASSUMPTIONS.md:105` says default 100.
- Fix: change literal 50 → 100. Update or add e2e scenario asserting the value
  was forwarded to the daemon (mock currently ignores `tail_n`; see C3).

### A2. `logs --follow` and `--tail` are not mutually exclusive
- Evidence: `internal/cli/logs.go:35-39` — chooses `runLogsTail` if `!follow`,
  silently drops `--tail` when `--follow` is set.
- Spec: `ASSUMPTIONS.md:104` says specifying both must exit code 2.
- Fix: in the `RunE`, after parsing flags, check
  `if follow && cmd.Flags().Changed("tail") { return UsageError(...) }`.
- Add e2e scenario `logs-conflict.txt` covering this.

### A3. Manifest validation errors render to stderr in `--json` mode
- Evidence: `internal/cli/run.go:108` — `_ = render.JSON(rt.Stderr, payload)`.
- Spec: `ASSUMPTIONS.md:108` says `--json` errors go to stdout for jq.
- Fix: change `rt.Stderr` → `rt.Stdout`. Update existing
  `run-bad-paths.txt` / `run-bad-cidr.txt` or add `run-bad-paths-json.txt`
  asserting JSON appears on stdout.

### A4. Daemon-side errors bypass `--json` formatting (architectural)
- Evidence: `internal/cli/run.go:62-64`, plus the same `return err` shape in
  `list.go`, `stop.go`, `logs.go`, `daemon.go`. After the error returns,
  `agentctlcmd.Main` (`cmd/agentctl/agentctlcmd/agentctlcmd.go:21-26`) prints
  `"Error: %s\n"` to stderr regardless of `rt.JSON`.
- Spec: `ASSUMPTIONS.md:108` says `--json` errors must go to stdout.
- Fix shape (chosen for minimal subcommand churn — see Order of Operations):
  Add helper `renderDaemonErr(rt *appRuntime, err error) error` in
  `internal/cli/exit.go`. When `rt.JSON` is true and `err` is a recognised
  daemon-side error (`*daemon.ServerError`, `daemon.ErrDaemonUnreachable`,
  `daemon.ErrAgentNotFound`), emit `render.JSONErr(rt.Stdout, code, message)`
  to **stdout** and return `AlreadyPrinted(err)`. Otherwise return `err`
  unchanged. Wrap each subcommand's `if err != nil { return err }` with this
  helper.
- This reuses the existing `printedErr` plumbing (`exit.go:22-41`) and the
  unused `JSONErr` (`render/render.go:95`) so no new abstractions land.

---

## Group B — Manifest validator gaps that propagate to P1/P2

### B1. Path patterns accept control characters
- Evidence: `internal/manifest/validate.go:158-172` — `validPathPattern`
  rejects `**`, `?`, `[`, `]` but does not filter `\x00`, `\n`, `\r`, `\t`.
  These bytes propagate verbatim into the daemon's `ManifestPayload.AllowedPaths`,
  thence into P1's BPF allowlist key. Behaviour with embedded NUL or newline is
  kernel-implementation-defined and likely creates silent allow/deny mismatches.
- Fix: add `if strings.ContainsAny(p, "\x00\n\r\t") { return false }` early in
  `validPathPattern`.
- Add validator test for each rejected character.

### B2. `stdin: file:<path>` allows control characters in the path
- Evidence: `internal/manifest/validate.go:189-198` — `validStdin` only checks
  `file:` prefix and absolute-path prefix; no content scrub.
- Fix: same character class as B1 applied to `s[5:]`.

### B3. Env keys are not validated
- Evidence: `internal/manifest/parse.go:294-320` — `validateEnv` validates
  *values* (for `${VAR}` resolution) but not *keys*. A manifest with
  `env: { "KEY=VAL": "x" }` or `env: { "K\nL": "x" }` parses cleanly and
  corrupts the envp passed to `execve` in the daemon.
- Fix: at the loop top, validate `k.Value` against POSIX portable name regex
  `^[A-Za-z_][A-Za-z0-9_]*$`. Reject empty keys explicitly (yaml allows them).
- Add a `CodeBadEnvKey` constant in `errors.go` and a validator test.

### B4. Duplicate YAML keys silently overwrite (priority: HIGH)
- Evidence: `internal/manifest/parse.go:85-108` — loop assigns
  `seen[key.Value] = val` with no duplicate check. yaml.v3 permits duplicate
  keys; the last one silently wins.
- Audit-vs-enforcement risk: the audit log shows the manifest source as fed by
  the user, but the daemon enforces only the second occurrence's value. A
  manifest with two `allowed_hosts:` entries would silently merge them
  destructively.
- Fix: detect duplicates inside the parse loop —
  `if _, dup := seen[key.Value]; dup { eb.addf(CodeDuplicateKey, ...); continue }`.
- Add `CodeDuplicateKey` constant and a parser test.

### B5. Negative uids accepted (priority: LOW within Group B)
- Evidence: `internal/manifest/validate.go:177-187` — `strconv.Atoi("-1")`
  succeeds.
- Risk is lower than B1-B4: the daemon's clone3 will hard-fail on `setuid(-1)`
  with a clean kernel error. Worth fixing for early-stage UX, but defensible to
  defer.
- Fix: after `strconv.Atoi`, check `uid >= 0`. Optional upper bound at
  `2147483647` for portability.

---

## Group C — Wire-shape fragility (would silently break post-orchestration)

### C1. `manifestToPayload` is a hand-copied field list
- Evidence: `internal/cli/run.go:82-95` — explicit field-by-field copy from
  `manifest.Manifest` → `daemon.ManifestPayload`. ASSUMPTIONS.md:194 already
  flags this as fragile.
- Risk: if either struct gains a field, the copy silently drops it; both struct
  definitions might compile but the daemon receives a zero value.
- Fix: add a reflection-based test `TestManifestPayload_FieldsMatch` in
  `internal/cli/run_test.go` (new file) that walks the JSON tags of both
  structs and asserts set-equality. Not a runtime fix — a guardrail test.

### C2. `--include` is not honoured server-side for `--tail`
- Evidence: `internal/cli/logs.go:51-62` — `runLogsTail` calls
  `cl.AgentLogs(ctx, name, tail)`. `AgentLogsRequest` (`protocol.go:142-145`)
  has no `include` field. The client-side filter on
  `logs.go:57` drops events after the daemon already sent everything.
  ASSUMPTIONS.md:170-172 says daemon is authoritative for `--include`, which is
  true for `--follow` (StreamEvents has the field) but not for `--tail`.
- Two fixes possible:
  (a) Treat `--include` + `--tail` (no `--follow`) as a usage error.
  (b) Extend `AgentLogsRequest` with `Include []string`. This is a wire change
      requiring P2 confirmation.
- Recommended for P3: pick (a) — minimal wire churn, defers to orchestrator if
  P2 wants the extension. Add usage check in `runLogsTail` and an e2e
  scenario.

### C3. Mock daemon `OnLogs` ignores `TailN`
- Evidence: `e2e/testscript_test.go:229-236` — handler always returns the same
  two events.
- Risk: any test that asserts `--tail` value reaches the daemon passes for the
  wrong reason. If the wire field name diverged from `tail_n`, no test would
  catch it.
- Fix: in `installDefaults`, slice the canned event list down to `req.TailN`
  (when > 0), and expand the canned list to ~5 events so slicing is observable.
  Then add an e2e assertion.

---

## Group D — Cross-layer items for orchestrator hand-off (no P3 code change)

These are not gaps to fix in P3 but items the orchestrator needs to verify
when stitching layers. Add to `ASSUMPTIONS.md` under a new "Open questions for
orchestrator" section:

### D1. Wire-contract verification against the real P2 daemon
- Four assumed-but-unverified contracts (already listed in
  `ASSUMPTIONS.md:37-67`): envelope shape, `events` vs `entries` field name,
  half-close semantics, framing endianness.
- Recommendation: a single round-trip integration test against the compiled
  P2 binary, exercising each of the seven methods and the StreamEvents loop.
  This belongs in the orchestrator's CI, not in P3, but P3 should explicitly
  call it out as a release-gate item.

### D2. `default user = os.Getuid()` semantics
- Evidence: `internal/manifest/parse.go:247` — when manifest omits `user`, the
  CLI fills the *client's* uid into the resolved manifest. The daemon (a
  privileged service) almost certainly runs as a different uid; it must
  reconcile via SO_PEERCRED. The current behaviour is documented in
  ASSUMPTIONS.md:88-90 but worth flagging to P2 as a contract: "if `user` is
  empty in the payload, the daemon must use the client's SO_PEERCRED uid, not
  the value sent by the CLI" — or, alternatively, P3 should send empty and
  let the daemon fill it.
- No code change in P3 yet — orchestrator decides.

---

## Out of scope (deliberately deferred)

- PowerShell completion e2e scenario (works in code; just no test).
- Dead code: `errStreamEnded` in `cli/logs.go:123`. Tag for cleanup pass.
- `AgentLogsRequest.TailN` is platform-dependent `int`; consider explicit
  `int32` after wire verification.
- Bound `--tail` client-side (DoS defense-in-depth) — best deferred until daemon
  cap is known.
- `emitEvent` ignores JSON encode errors; EPIPE on `| head` would silently
  spin. Worth a fix later but not a release blocker.
- Hostname trailing-dot rejection is implicit (works via empty-label check).
- Max path length not enforced (PATH_MAX 4096).

---

## Order of operations

1. **C1 — field-equivalence test** (no behaviour change, sets the safety net
   for later refactors).
2. **A4 — `renderDaemonErr` helper** + wire it through every subcommand. This
   is the architectural prerequisite for A3 to make sense.
3. **A3 — switch manifest validation JSON to stdout** (one-line change; now
   consistent with A4).
4. **Group B in one commit** (B1-B5; all in `validate.go`/`parse.go` with
   shared test scaffolding). Inner order: B4 first (touches parse-loop control
   flow), B3 next (similar), then B1/B2/B5 (predicate-only).
5. **A1 + A2 — logs flag fixes** (independent, single commit).
6. **C2 — `--include` + `--tail` usage error** + scenario.
7. **C3 — fix mock `OnLogs` to honour `TailN`** before adding any new
   `--tail`/`--include` interaction tests, otherwise tests pass for wrong
   reason.
8. **Group D — update `ASSUMPTIONS.md`** with the orchestrator-question
   section.

Reasoning for ordering: doing A3 before A4 leaves a window where manifest errors
go to stdout but daemon errors still go to stderr — inconsistent. Doing Group B
before A4 would force test rewrites once error rendering changes.

---

## Files to modify

- `internal/cli/exit.go` — add `renderDaemonErr` helper.
- `internal/cli/run.go` — fix manifest-error stdout (A3); wire helper (A4).
- `internal/cli/list.go`, `stop.go`, `logs.go`, `daemon.go` — wire helper (A4).
- `internal/cli/logs.go` — flag mutual exclusivity (A2), default value (A1),
  `--include`+`--tail` rejection (C2).
- `internal/cli/run_test.go` — new file: field-equivalence test (C1).
- `internal/manifest/validate.go` — control-char filters (B1, B2),
  uid range check (B5).
- `internal/manifest/parse.go` — duplicate-key detection (B4), env key
  validation (B3).
- `internal/manifest/errors.go` — add `CodeDuplicateKey`, `CodeBadEnvKey`.
- `internal/manifest/parse_test.go` / `validate_test.go` — coverage for B1-B5.
- `e2e/testscript_test.go` — fix `OnLogs` to honour `TailN` (C3); expand canned
  event list.
- `e2e/testdata/script/` — new scenarios:
  - `logs-conflict.txt` (A2)
  - `run-bad-paths-json.txt` (A3)
  - `run-daemon-error-json.txt` (A4)
  - `logs-tail-include-conflict.txt` (C2)
- `ASSUMPTIONS.md` — new "Open questions for orchestrator" section (D1, D2);
  update test inventory totals; update Group A items as resolved.

---

## Verification

After all fixes land:

1. `go vet ./...` — clean.
2. `go build ./...` — clean.
3. `go test ./... -timeout 90s -race -count=1` — all pass; total test count
   should rise by ~10-15.
4. `./agentctl manifest validate examples/bad-paths.yaml` — still rejects.
5. New manual smoke checks:
   - `printf 'name: x\nname: y\ncommand: [/bin/true]\nallowed_hosts: []\nallowed_paths: []\n' | ./agentctl manifest validate /dev/stdin` — should report `duplicate_key` with line/col.
   - `./agentctl --json run -f examples/bad-paths.yaml` — JSON error envelope
     on stdout, exit 3.
   - `./agentctl --json list` against a stopped daemon — JSON
     `{"ok":false,"code":"daemon_unreachable",...}` on stdout, exit 4.
   - `./agentctl logs --follow --tail 5 agent-x` — exits 2 with usage error.
   - `./agentctl logs --tail 5 --include lifecycle agent-x` — exits 2 (per C2).
6. CI workflow re-runs unchanged (`.github/workflows/ci.yml`).
