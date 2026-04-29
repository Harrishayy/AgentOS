package manifest

import (
	"fmt"
	"time"
)

// formatPolicySummary renders the "hosts:N paths:M timeout:T" line used in
// `agentctl run` and `agentctl list` output.
func formatPolicySummary(nHosts, nPaths int, timeoutNS int64) string {
	t := "0"
	if timeoutNS > 0 {
		t = compactDuration(time.Duration(timeoutNS))
	}
	return fmt.Sprintf("hosts:%d paths:%d timeout:%s", nHosts, nPaths, t)
}

// compactDuration formats a duration using the smallest unit that yields an
// integer for whole seconds/minutes/hours.
//   - 5_000_000_000 ns → "5s"
//   - 300_000_000_000 ns → "5m"
//   - 3600_000_000_000 ns → "1h"
//
// Falls back to time.Duration.String() for sub-second or oddball values.
func compactDuration(d time.Duration) string {
	switch {
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	case d%time.Second == 0:
		return fmt.Sprintf("%ds", d/time.Second)
	default:
		return d.String()
	}
}
