# Integration smoke-test log

This file is appended as each integration branch is created.

| Step | Branch | Result | Notes |
|------|--------|--------|-------|
| P1   | `integration/p1` | PASS | `make -n -C bpf` parses; 4 `.bpf.c` files (network/file/creds/exec) listed for compilation; `policies/*.yaml` + `examples/*.yaml` are valid YAML. `MAX_POLICIES=32` (will bump to 64 in P2 step per Harrish ask). |
| P2   | `integration/p1-p2` | PASS | `bpf/common.h` matches `daemon/bpf/common.h.reference` byte-for-byte; both bumped `MAX_POLICIES 32→64` per Harrish integration ask #1. `cd daemon && go vet ./...` clean. `go build ./cmd/test-client` and `GOOS=linux go build ./cmd/daemon` both succeed. `go test ./internal/...` passes (bpf, events, ipc, policy, registry). `go.sum` regenerated via `go mod tidy`. |
