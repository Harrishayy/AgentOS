# Contributing

Thanks for considering a contribution. This project is built and
maintained as one open-source codebase, not five branches glued
together.

## Repo layout (TL;DR)

The repo is a **single Go module** rooted at the top level
(`github.com/agent-sandbox/runtime`). Binaries under `cmd/`,
shared libraries under `internal/`. The eBPF source, Python
orchestrator, and Node viewer have their own subtrees but build
through the same top-level `Makefile`.

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the full tour
and [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) for the build
instructions.

## Where to file changes

Use a topic-prefixed branch name and matching commit subject prefix:

| Touches                                  | Branch / commit prefix |
|------------------------------------------|------------------------|
| eBPF C source (`bpf/`)                   | `bpf:`                 |
| Daemon (`cmd/agentd`, `internal/...`)    | `daemon:`              |
| CLI (`cmd/agentctl`, `internal/cli|manifest|client|render`) | `cli:` |
| Python orchestrator (`orchestrator/`)    | `orch:`                |
| Viewer (`viewer/`)                       | `viewer:`              |
| Wire-protocol or schema changes          | `proto:`               |
| Documentation (`docs/`, README)          | `docs:`                |
| Build / CI                               | `build:`               |
| Cross-cutting refactor                   | `refactor:`            |

Example commit subjects:

```
daemon: pre-create working_dir before exec
cli: accept mode/allowed_bins/forbidden_caps in manifest schema
bpf: harden has_prefix against zero-length prefix
docs: document the BPF LSM cmdline gotcha
```

Keep subjects ≤ 70 characters. Body wraps at 72.

## PR checklist

- [ ] `make test` passes locally.
- [ ] If you touched kernel-side code, `make integration` passes on a
      Linux box with the BPF LSM enabled.
- [ ] If you added a manifest field or RPC method, the
      [`docs/INTERFACES.md`](docs/INTERFACES.md) reference is updated.
- [ ] One reviewer approval before merge.

CI runs unit tests on every push; integration tests run on demand
via the `integration` workflow_dispatch trigger.

## Code style

- Comments explain *why* the code is the way it is, not what it
  does. If the *why* is obvious, no comment.
- Don't refactor adjacent code "while you're there." Land minimal,
  reviewable diffs; cleanup goes in its own PR.
- Match the surrounding style. Run `make fmt` before committing.

## Reporting bugs

- Functional bug: open an issue with steps, expected, actual, and
  the relevant per-agent log line if any.
- Security issue: see [`docs/THREAT_MODEL.md`](docs/THREAT_MODEL.md)
  for the disclosure process — please don't open a public issue.
