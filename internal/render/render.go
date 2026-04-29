// Package render formats CLI output. Two modes:
//
//   - Human (default): tab-aligned tables, colourless, designed to be diff-able.
//   - JSON (--json): newline-delimited JSON objects, one per logical row.
//
// All public functions take an io.Writer so tests can capture output cleanly.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/agent-sandbox/cli/internal/daemon"
)

// JSON marshals v as one line of JSON to w followed by a newline. Used by all
// subcommands when --json is set.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// HumanRunResult prints the post-run summary used by `agentctl run`.
//
//	Started agent-x  pid=4242  cgroup=/sys/fs/cgroup/agent/agent-x
//	policy: hosts:1 paths:0 timeout:0
func HumanRunResult(w io.Writer, r *daemon.RunAgentResult) {
	fmt.Fprintf(w, "Started %s  pid=%d  cgroup=%s\n", r.Name, r.PID, r.CgroupPath)
	if r.PolicySummary != "" {
		fmt.Fprintf(w, "policy: %s\n", r.PolicySummary)
	}
}

// HumanStopResult prints the summary used by `agentctl stop`.
func HumanStopResult(w io.Writer, r *daemon.StopAgentResult) {
	dur := time.Duration(r.DurationNS).Round(time.Millisecond)
	fmt.Fprintf(w, "Stopped %s  signal=%s  exit=%d  in=%s\n", r.Name, r.Signal, r.ExitCode, dur)
}

// HumanList prints `agentctl list` rows in a tab-aligned table.
//
//	NAME      ID      STATUS   PID    UPTIME    POLICY
//	agent-x   01H8X0  running  4242   3m12s     hosts:1 paths:0 timeout:0
//	gone      01F00B  exited   -      -         hosts:0 paths:1 timeout:30s   exit=0
func HumanList(w io.Writer, agents []daemon.AgentInfo) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	defer tw.Flush()
	fmt.Fprintln(tw, "NAME\tID\tSTATUS\tPID\tUPTIME\tPOLICY")
	for _, a := range agents {
		pid := "-"
		if a.PID > 0 {
			pid = fmt.Sprintf("%d", a.PID)
		}
		uptime := "-"
		if a.UptimeNS > 0 {
			uptime = compactDuration(time.Duration(a.UptimeNS))
		}
		policy := a.PolicySummary
		if a.Status != "running" && a.ExitCode != nil {
			policy = fmt.Sprintf("%s\texit=%d", policy, *a.ExitCode)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			a.Name, a.AgentID, a.Status, pid, uptime, policy)
	}
}

// HumanDaemonStatus prints `agentctl daemon status`.
func HumanDaemonStatus(w io.Writer, s *daemon.DaemonStatusResult) {
	uptime := compactDuration(time.Duration(s.UptimeNS))
	fmt.Fprintf(w, "protocol: %s\nbuild: %s\nuptime: %s\nagents: %d\n",
		s.ProtocolVersion, s.Build, uptime, s.AgentsRunning)
}

func compactDuration(d time.Duration) string {
	if d < time.Second {
		return d.String()
	}
	d = d.Round(time.Second)
	switch {
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	default:
		return d.String()
	}
}

// JSONErr renders a wire-shaped error envelope (for `--json` failure output).
func JSONErr(w io.Writer, code, message string) error {
	return JSON(w, struct {
		Ok    bool   `json:"ok"`
		Code  string `json:"code"`
		Error string `json:"error"`
	}{Ok: false, Code: code, Error: message})
}

// quote wraps s in double quotes if it contains whitespace; bare otherwise.
// Used by event renderers below.
func quote(s string) string {
	if strings.ContainsAny(s, " \t\n\"") {
		b, _ := json.Marshal(s)
		return string(b)
	}
	return s
}
