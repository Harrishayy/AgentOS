# Integration smoke-test log

This file is appended as each integration branch is created.

| Step | Branch | Result | Notes |
|------|--------|--------|-------|
| P1   | `integration/p1` | PASS | `make -n -C bpf` parses; 4 `.bpf.c` files (network/file/creds/exec) listed for compilation; `policies/*.yaml` + `examples/*.yaml` are valid YAML. `MAX_POLICIES=32` (will bump to 64 in P2 step per Harrish ask). |
| P2   | `integration/p1-p2` | PASS | `bpf/common.h` matches `daemon/bpf/common.h.reference` byte-for-byte; both bumped `MAX_POLICIES 32→64` per Harrish integration ask #1. `cd daemon && go vet ./...` clean. `go build ./cmd/test-client` and `GOOS=linux go build ./cmd/daemon` both succeed. `go test ./internal/...` passes (bpf, events, ipc, policy, registry). `go.sum` regenerated via `go mod tidy`. |
| P3   | `integration/p1-p2-p3` | PASS | Combined `.github/workflows/ci.yml` into two jobs (`cli` + `daemon`). Created `go.work` (`./` + `./daemon`) so both modules cohabit cleanly. `go vet ./cmd/... ./internal/...` clean. `go build ./cmd/agentctl` succeeds. `go test ./cmd/... ./internal/... ./e2e/...` all pass. `agentctl manifest validate examples/web-fetcher.yaml` → OK; `examples/bad-paths.yaml` → exit 3 with line/col error. `go build ./...` (root) and `cd daemon && go vet ./...` both clean — no module collision. |
