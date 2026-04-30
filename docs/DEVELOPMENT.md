# Developer Guide

How to build, test, and contribute to the agent-sandbox runtime.

## Prerequisites

You need a Linux environment with kernel 6.8+. Three supported paths:

| Host                      | Recommended VM tool                                |
|---------------------------|----------------------------------------------------|
| macOS (Apple Silicon)     | `brew install lima` then `limactl start --name=agentsandbox …` |
| macOS (Intel) / Windows   | `brew install vagrant` (Mac) or VirtualBox + `vagrant up`         |
| Linux                     | none — run directly                                |

Once inside the Linux box, run the setup script once:

```bash
bash scripts/setup-vm.sh
# If it prints REBOOT REQUIRED:
sudo reboot
# Then verify:
cat /sys/kernel/security/lsm | grep bpf
```

## Repository layout

```
.
├── bpf/              eBPF C source (P1) + Makefile
├── cmd/              every Go binary
│   ├── agentd/       the daemon
│   ├── agentctl/     the CLI
│   └── test-client/  raw IPC test client
├── internal/         every Go library
│   ├── bpf/          BPF loader + ringbuf reader
│   ├── cgroup/       cgroup v2 lifecycle
│   ├── cli/          cobra command tree
│   ├── client/       CLI-side IPC client
│   ├── events/       event pipeline + WebSocket fanout
│   ├── ipc/          length-prefixed JSON server + types
│   ├── manifest/     YAML parser + validator
│   ├── policy/       manifest → BPF struct compile
│   ├── registry/     in-mem agent registry
│   ├── render/       human + JSON output for the CLI
│   └── testutil/     shared test helpers (mock daemon, fixtures)
├── orchestrator/     LLM-driven launcher (Python, P4)
├── viewer/           dashboard (Node + React, P5)
├── examples/         shipped manifests + test-it.sh smoke
├── policies/         shipped default policies
├── deploy/           install.sh + systemd unit
├── e2e/              testscript-driven CLI scenarios
├── tests/            integration + manual test harnesses
├── scripts/          setup-vm.sh + helpers
├── docs/             this directory
├── go.mod            single Go module
└── Makefile          top-level build
```

One Go module: `github.com/agent-sandbox/runtime`.

## Common workflows

### Build everything

```bash
make all
# produces:
#   bin/agentd
#   bin/agentctl
#   bin/test-client
#   bpf/*.bpf.o
```

### Run unit tests

```bash
make test
```

These are pure Go tests; they don't need root or eBPF.

### Run integration tests (Linux + root)

```bash
make integration
```

These exercise the real cgroup + BPF loader. They need:
- Linux kernel 6.8+
- BPF LSM active (`lsm=…,bpf` on the kernel cmdline)
- `CAP_SYS_ADMIN`, `CAP_BPF`

### Run the e2e CLI suite

```bash
make e2e
```

A `testscript`-driven harness ([`e2e/testscript_test.go`](../e2e/testscript_test.go))
runs the CLI binary against an in-process mock daemon.
Each `*.txt` fixture under `e2e/testdata/script/` is a self-contained
scenario.

### Smoke test the running system

In one terminal, run the daemon:

```bash
sudo ./bin/agentd \
  -bpf-dir=$(pwd)/bpf \
  -socket=/run/agent-sandbox.sock \
  -ws-addr=127.0.0.1:7443
```

In another, run the smoke script:

```bash
sudo bash examples/test-it.sh
```

Expected output: green checkmarks for daemon health, BPF program
count, and verdict assertions on `blocked-net.yaml` /
`allowed-net.yaml`.

To watch events live in a browser:

```bash
bash viewer/scripts/start-viewer.sh
# open http://127.0.0.1:8765 (Lima/Vagrant auto-forwards loopback)
```

The viewer's bridge subscribes to the daemon's `:7443/events` and
mirrors every event into the dashboard.

## Editing the eBPF programs

```bash
make -C bpf clean
make -C bpf
```

`bpf/Makefile` regenerates `vmlinux.h` from the running kernel's BTF
on each build. Targets are platform-specific — `-D__TARGET_ARCH_arm64`
or `__TARGET_ARCH_x86`.

Common pitfall: the verifier rejects unbounded loops. Use
`#pragma unroll` only when necessary; modern verifiers handle bounded
`for` loops natively. Walk through `bpf/common.h` to see the helpers
already in place.

## Adding a new manifest field

A field that the daemon must enforce touches **five** files end-to-end:

1. `bpf/common.h` and the relevant `.bpf.c` — extend `struct policy`,
   add the enforcement logic.
2. `internal/policy/policy.go` — extend the `Compiled` struct and the
   `Compile()` function.
3. `internal/ipc/protocol.go` — extend the wire `Manifest`.
4. `internal/manifest/manifest.go` + `parse.go` + `validate.go` — add
   the field to the YAML schema with a validator.
5. `internal/client/protocol.go` and `internal/cli/run.go` — pass the
   field through the CLI's wire-side `ManifestPayload` and
   `manifestToPayload`.

`internal/cli/run_test.go` has a reflection-based check that catches
field drift between `manifest.Manifest` and `client.ManifestPayload`.
You'll know if you missed step 5.

Update [`docs/INTERFACES.md`](INTERFACES.md) too.

## Adding a new RPC method

1. Define the request/response types in both
   `internal/ipc/protocol.go` (server) and
   `internal/client/protocol.go` (client). Field tags must match.
2. Add the handler to `internal/ipc/server.go`'s dispatch switch.
3. Add the client method to `internal/client/client.go`.
4. If the method is user-facing, wire a cobra command under
   `internal/cli/`.
5. Document it in [`docs/INTERFACES.md`](INTERFACES.md).
6. Add an `e2e/testdata/script/` scenario.

## Style and lint

```bash
make fmt   # gofmt -w
make vet   # go vet
make lint  # golangci-lint (see .golangci.yml)
```

Comments in source code: explain *why*, not what. Reference issue
numbers or design docs only when the linked doc adds context the
reader needs to make a code-level judgement. Avoid comments that
just restate the next line.

## Submitting changes

Per [CONTRIBUTING.md](../CONTRIBUTING.md):

1. Branch from `main`, naming the branch by component:
   `daemon/`, `cli/`, `bpf/`, `orchestrator/`, `viewer/`, `docs/`, or
   `refactor/`.
2. Each commit subject begins with the area and a colon
   (`daemon: …`, `cli: …`).
3. Run `make test`. Optionally `make integration` if your change
   touches kernel-side code.
4. Open a PR. CI runs unit tests on every push; integration tests
   run on demand.
5. One reviewer LGTM is sufficient.

## Useful debugging

- **Daemon not enforcing?** Check
  `cat /sys/kernel/security/lsm` for `bpf`. If absent, re-run
  `scripts/setup-vm.sh` and reboot.
- **Agent crashes with EPERM on a syscall you expected to allow?**
  Tail the per-agent log:
  `sudo tail -f /var/log/agent-sandbox/<agent_id>.log | jq 'select(.details.verdict=="deny")'`
- **BPF programs loaded but not firing?**
  `sudo bpftool prog show | grep asb_` lists each program with its
  ID and tag. The expected count is 8.
- **Verifier error at daemon startup?** The daemon prints the
  verifier's diagnosis verbatim. Common cause: a new `for` loop
  without an obvious bound. Add an explicit `if (i >= MAX) break;`.
