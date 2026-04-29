# P3 Week 3 — Sources

All external sources cited by the Week 3 plan. Numbered for inline reference (`[#N]`). Access date is the date the page or proxy was hit.

| # | Title | URL | Access Date | Notes |
|---|-------|-----|-------------|-------|
| 1 | spf13/cobra @latest (Go module proxy) | https://proxy.golang.org/github.com/spf13/cobra/@latest | 2026-04-27 | Authoritative version pin: v1.10.2 (2025-12-03) |
| 2 | gopkg.in/yaml.v3 @latest (Go module proxy) | https://proxy.golang.org/gopkg.in/yaml.v3/@latest | 2026-04-27 | v3.0.1 (2022-05-27); upstream archived |
| 3 | sigs.k8s.io/yaml package docs | https://pkg.go.dev/sigs.k8s.io/yaml | 2026-04-27 | v1.6.0 (2025-07-24); rejected — no error stability guarantee |
| 4 | github.com/spf13/cobra package docs | https://pkg.go.dev/github.com/spf13/cobra | 2026-04-27 | Completion API + ExecuteContext signatures |
| 5 | rogpeppe/go-internal @latest (Go module proxy) | https://proxy.golang.org/github.com/rogpeppe/go-internal/@latest | 2026-04-27 | v1.14.1 (2025-02-25) |
| 6 | os/signal package docs | https://pkg.go.dev/os/signal | 2026-04-27 | NotifyContext signature + canonical SIGINT pattern; Go 1.16+ |
| 7 | testscript package docs | https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript | 2026-04-27 | Background/wait/kill primitives for streaming tests |
| 8 | gopkg.in/yaml.v3 package docs | https://pkg.go.dev/gopkg.in/yaml.v3 | 2026-04-27 | yaml.Node Line/Column fields; KnownFields(true) |
| 9 | go.yaml.in/yaml/v3 (maintained fork) | https://github.com/go-yaml/yaml | 2026-04-27 | Reference for OPEN-Q-001 (yaml.v3 archive successor) |
| 10 | project-plan.md | /workspace/project-plan.md | 2026-04-27 | The product brief; §2, §3, §4 referenced throughout |

## Citation conventions

- Inline citations in other artifacts use `[#N]` referring to this table.
- Library versions are pinned to the exact tag confirmed via the Go module proxy. Do not bump silently.
- If a source URL is replaced in a later week, append a new row rather than editing — preserve provenance.
