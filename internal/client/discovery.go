package client

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// EnvSocket is the environment variable that overrides the daemon socket path
// (DEC-008 step 2).
const EnvSocket = "AGENT_SANDBOX_SOCKET"

// DefaultProductionSocket is the production fallback path (DEC-008 step 4).
const DefaultProductionSocket = "/run/agent-sandbox.sock"

// DiscoveryError is returned when no socket can be discovered. It carries the
// list of paths that were tried so the user can debug.
type DiscoveryError struct {
	Tried []string
}

func (e *DiscoveryError) Error() string {
	return fmt.Sprintf("daemon socket not found; tried: %v", e.Tried)
}

// Is wires the discovery failure to the user-facing ErrDaemonUnreachable
// sentinel.
func (e *DiscoveryError) Is(target error) bool {
	return target == ErrDaemonUnreachable
}

// ResolveSocketPath applies the discovery order from DEC-008:
//
//  1. explicit (CLI flag)
//  2. AGENT_SANDBOX_SOCKET env
//  3. $XDG_RUNTIME_DIR/agent-sandbox.sock
//  4. /run/agent-sandbox.sock
//
// Returns the first existing path. If none exist, returns the production path
// (so a clean error surfaces from Dial against `/run/agent-sandbox.sock`).
//
// `explicit` may be empty.
func ResolveSocketPath(explicit string) string {
	for _, candidate := range candidates(explicit) {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// Fallback to production path so the eventual Dial yields a clean
	// "no such file or directory" error against the canonical location.
	return DefaultProductionSocket
}

// ResolveSocketPathStrict is ResolveSocketPath but returns DiscoveryError if
// none of the candidate paths exist. Intended for diagnostic / CLI flag
// validation flows.
func ResolveSocketPathStrict(explicit string) (string, error) {
	cs := candidates(explicit)
	tried := []string{}
	for _, c := range cs {
		if c == "" {
			continue
		}
		tried = append(tried, c)
		_, err := os.Stat(c)
		if err == nil {
			return c, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			// Some other error (permission, etc.) — surface it.
			return "", fmt.Errorf("stat %s: %w", c, err)
		}
	}
	return "", &DiscoveryError{Tried: tried}
}

func candidates(explicit string) []string {
	out := make([]string, 0, 4)
	out = append(out, explicit)
	out = append(out, os.Getenv(EnvSocket))
	if x := os.Getenv("XDG_RUNTIME_DIR"); x != "" {
		out = append(out, filepath.Join(x, "agent-sandbox.sock"))
	}
	out = append(out, DefaultProductionSocket)
	return out
}
