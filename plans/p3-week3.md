# P3 Week 3 Plan — Pointer

The full Week 3 plan for P3 (CLI + manifest) lives at `/workspace/planning/p3/week3/`, as required by the mission brief. This file exists so anyone scanning `/workspace/plans/` (per the gitignored plans-folder convention) can find it.

## Files

- [P3_WEEK3_PLAN.md](/workspace/planning/p3/week3/P3_WEEK3_PLAN.md) — master plan
- [INTERFACES.md](/workspace/planning/p3/week3/INTERFACES.md) — manifest, protocol, event schemas
- [DECISIONS.md](/workspace/planning/p3/week3/DECISIONS.md) — 12 ADRs (DEC-001 superseded by DEC-011 for socket framing)
- [OPEN_QUESTIONS.md](/workspace/planning/p3/week3/OPEN_QUESTIONS.md) — Day-1 closures + 3 open (P1×2, P5×1)
- [RESEARCH_LOG.md](/workspace/planning/p3/week3/RESEARCH_LOG.md) — verified claims
- [SOURCES.md](/workspace/planning/p3/week3/SOURCES.md) — citations

## At-a-glance

- **What ships Friday:** `agentctl` with `run` (incl. `--restart-on-crash`/`--max-restarts`), `list`, `stop`, `logs --follow`/`--tail`, completions, manifest validation with file:line:col errors, CI green.
- **Day-1 closed:** event schema with P4+P5 (Q-006), supported fields with P1 (Q-007 — `allowed_paths` enforced too), unknown event passthrough (Q-008), CIDR support in v1 (Q-010), orchestrator→daemon `IngestEvent` (Q-011), restart in orchestrator (Q-012), required-may-be-empty fields (Q-013), custom Python orchestrator (Q-014, AIOS dropped).
- **Day-2 closed:** daemon protocol with P2 (Q-005) — P2 signed off the seven deltas; full method names (`RunAgent`/`StopAgent`/`ListAgents`) restored on the wire; `agent.stdout`/`agent.stderr` raw stdio events added (INTERFACES §3.5); `event.type` on `IngestEvent` server-validated against `^llm\.`, prefix violation → `INVALID_MANIFEST`.
- **Still open:** P5 VM/CI shape (Q-009).
- **Long pole:** `agentctl logs --follow` (`StreamEvents`) + signal handling. Budget Thursday.
- **Biggest risk:** `logs` hanging on Ctrl-C. Mitigated by DEC-007 + 100ms-budget testscript test.
