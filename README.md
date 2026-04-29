# agentctl

`agentctl` is the user-facing CLI for the **agent-sandbox** runtime. It validates
declarative agent manifests, talks to the `agentd` daemon over a Unix socket,
and renders agent state and event streams.

This repository contains the **P3** workstream: the manifest layer, the daemon
client, and the cobra-based command tree. The daemon (`agentd`) ships in the
P2 repository.

## What ships in this build

| Subcommand            | Purpose                                                     |
|-----------------------|-------------------------------------------------------------|
| `run -f manifest.yaml`| Validate manifest, send to daemon, print run summary        |
| `list`                | Tab-aligned table of agents the daemon is tracking          |
| `stop <name>`         | SIGTERM → grace → SIGKILL through the daemon                |
| `logs <name>`         | Print the last N events; `--follow` opens a live subscription |
| `daemon status`       | Probe the daemon for liveness, version, agent count         |
| `manifest validate`   | Parse & validate a manifest without contacting the daemon   |
| `completion <shell>`  | Emit a shell completion script (bash, zsh, fish, powershell)|
| `version`             | Print build, Go version, target platform, protocol version  |

## Quick start

```sh
go build -o ./agentctl ./cmd/agentctl

./agentctl manifest validate examples/web-fetcher.yaml
./agentctl --json list
./agentctl run -f examples/web-fetcher.yaml
./agentctl logs --follow agent-x
```

`--json` is a persistent flag and works on every subcommand. `--socket`
overrides the default discovery order.

## Manifest schema (v1)

```yaml
name: web-fetcher                       # required, [a-z0-9-]{1,63}
command: ["/usr/bin/curl", "https://x"] # required, non-empty argv
allowed_hosts:                          # required (may be empty list)
  - example.com:443
  - "*.openai.com:443"
  - 10.0.0.0/8
allowed_paths:                          # required (may be empty list)
  - /etc/hostname
  - /tmp/work/
  - /var/log/*.log
working_dir: /tmp/work                  # optional, absolute
env:                                    # optional, ${VAR} interpolated
  API_KEY: "${ANTHROPIC_API_KEY}"
user: "65534"                           # optional, uid or username
stdin: close                            # optional: inherit | close | file:/abs
timeout: "5m"                           # optional duration
description: "..."                      # optional
```

Every field is validated with **line/column-precise errors**. Examples:

```
$ agentctl manifest validate examples/bad-paths.yaml
examples/bad-paths.yaml:8:5: "/srv/**" is not a valid path pattern;
  expected absolute path, '/dir/' for tree, or single '*' glob
```

```
$ agentctl run -f bad.yaml
bad.yaml:4:1: unknown field "allowed_pots" (did you mean "allowed_paths"?)
```

See the [manifest error catalogue](internal/manifest/errors.go) for all codes.

## Socket discovery

In order (DEC-008):

1. `--socket /path/to/agentd.sock`
2. `$AGENT_SANDBOX_SOCKET`
3. `$XDG_RUNTIME_DIR/agent-sandbox.sock`
4. `/run/agent-sandbox.sock`

The first existing path wins. If none exist, the CLI falls through to the
production path and surfaces a clean `daemon unreachable` error.

## Exit codes

| Code | Meaning                                         |
|------|-------------------------------------------------|
| 0    | success                                         |
| 1    | generic failure                                 |
| 2    | usage error (cobra arg/flag mismatch)           |
| 3    | manifest invalid                                |
| 4    | daemon unreachable                              |
| 5    | daemon-side error                               |
| 6    | agent not found                                 |
| 130  | interrupted (SIGINT / SIGTERM)                  |

## Wire protocol (talking to `agentd`)

Length-prefixed JSON over a Unix domain socket: `[4-byte BE length][body]`,
16 MiB per-frame cap. Seven methods (RunAgent, StopAgent, ListAgents,
AgentLogs, StreamEvents, DaemonStatus, IngestEvent). All shapes live in
[`internal/daemon/protocol.go`](internal/daemon/protocol.go).

Streaming method (`StreamEvents`): the client writes one request frame and
then reads frames until the daemon closes (or until `Ctrl-C`). Cancellation
is observed in well under 100 ms via a `SetReadDeadline(past)` trick — see
[`internal/daemon/client.go`](internal/daemon/client.go).

## Development

```sh
go vet ./...
go build ./...
go test ./... -race    # 58 tests across 8 packages
```

End-to-end coverage lives in [`e2e/`](e2e). Each `.txt` file under
`e2e/testdata/script/` is a [testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript)
scenario that drives the compiled binary against an in-process mock daemon.

For assumptions made by this implementation that need orchestrator review,
see [`ASSUMPTIONS.md`](ASSUMPTIONS.md).
