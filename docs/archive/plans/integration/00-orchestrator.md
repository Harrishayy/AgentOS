# Integration Orchestrator Plan

Goal: stitch the five phase branches into a single integrated history that
compiles, smoke-tests cleanly, and lands on `main` without losing any work.

## Branch → phase mapping

| Phase | Owner branch                | Territory                                                 |
|-------|-----------------------------|-----------------------------------------------------------|
| P1    | `origin/Mehul`              | `bpf/`, `policies/`, `iso/`, `systemd/`, top-level `Makefile` |
| P2    | `origin/Harrish/sandbox-daemon` | `daemon/` (real daemon, vendored `bpf/common.h.reference`) |
| P3    | `origin/P3`                 | `cmd/`, `internal/`, `e2e/`, `examples/`, `go.mod`, `go.sum`, `project-plan.md`, `ASSUMPTIONS.md`, `.github/workflows/ci.yml`, `planning/`, `plans/p3-*.md` |
| P4    | `origin/arzaan`             | `arzaan/` (Python orchestrator + demo)                    |
| P5    | `origin/p5/viewer`          | `viewer/`, `scripts/setup-git-workflow.sh`                |

## Stack of integration branches

Each branch is built on top of the previous one and pushed for review.

1. `integration/base` — branched from `origin/main` (the canonical README/Vagrantfile).
2. `integration/p1` — `base` + `Mehul` content scoped to P1 territory.
3. `integration/p1-p2` — adds `Harrish/sandbox-daemon` daemon at `daemon/`.
4. `integration/p1-p2-p3` — adds `P3` CLI / manifest / examples at root.
5. `integration/p1-p2-p3-p4` — adds `arzaan` orchestrator at `arzaan/`.
6. `integration/full` — adds `p5/viewer` UI at `viewer/`.
7. `main` — fast-forwarded (or merged) from `integration/full` once smoke tests pass.

## Shared / colliding files — resolution policy

These files exist on every branch with small diffs. The integration policy:

| File              | Resolution                                                  |
|-------------------|-------------------------------------------------------------|
| `README.md`       | Keep `origin/main` version (it is the project-wide doc).     |
| `Vagrantfile`     | Keep `origin/main`'s; superset.                              |
| `setup-vm.sh`     | Use `Harrish` version (most complete BPF-LSM bootstrap).    |
| `CONTRIBUTING.md` | Keep `origin/main`.                                          |
| `LICENSE`         | Keep `origin/main` (Apache-2.0).                             |
| `decision.md`     | Keep `origin/main` (template).                               |
| `bug_report.md`   | Keep `origin/main` (template).                               |
| `viewer/index.html` | P5 wins (its `viewer-app/` build replaces the stub on main). |
| `daemon/`         | P2 (Harrish) wins; drop Mehul's stub at `daemon/cmd/agentd/`.|
| `cli/agentctl/`   | Drop entirely — superseded by P3's `cmd/agentctl/`.          |
| `bpf/common.h`    | Mehul's authoritative header. P2 already vendors a `.reference` copy.|
| `Makefile`        | Mehul's `bpf` Makefile lives at `bpf/Makefile`; root Makefile, if any, comes from Mehul. |

## Per-merge smoke tests

| Step                | Smoke test                                                              |
|---------------------|-------------------------------------------------------------------------|
| After P1            | `make -n -C bpf` parses; `bpf/*.bpf.c` present; `policies/*.yaml` valid YAML |
| After P2            | `cd daemon && go vet ./...` + `go build ./cmd/...` (Linux-tagged build OK to fail on missing kernel headers, but compile-only paths must pass) |
| After P3            | `go vet ./...` at repo root + `go build ./cmd/agentctl` (it must not pull in P2's Go module from `daemon/` — P3 is its own module) |
| After P4            | `python3 -m py_compile arzaan/**/*.py`                                  |
| After P5            | `node --check viewer/server/server.js`; `python3 -m json.tool viewer/server/package.json viewer/viewer-app/package.json` |
| Full                | All of the above run end-to-end; record results in `plans/integration/SMOKE.md` |

## Constraints / risks

- The repo is a polyglot multi-module workspace. P3 has its own `go.mod` at
  the repo root; P2 has its own `go.mod` at `daemon/go.mod`. They will live
  side-by-side as a Go workspace (`go.work`) is **not** required for this pass,
  but if `go vet ./...` from root recurses into `daemon/`, we add a `go.work`
  with `use ./` and `use ./daemon` to keep both buildable.
- Mehul ships its own `daemon/` and `cli/` stubs; both are removed in favour of
  P2 (`daemon/`) and P3 (`cmd/agentctl/`). This is a destructive choice that
  the integration doc from P2 explicitly anticipated.
- Network installs (`npm install`, `go mod download`) may not be reachable in
  this sandbox. Smoke tests must work without network — if they require it,
  we record the reason and skip that specific test.

## Execution

Each phase has a small plan in this folder (`01-p1.md` … `05-p5.md`).
The orchestrator runs them in order, recording the result of each smoke test
in `plans/integration/SMOKE.md` as it goes. If any merge fails, stop and ask.
