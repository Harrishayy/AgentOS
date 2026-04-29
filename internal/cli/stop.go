package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/agent-sandbox/cli/internal/render"
)

func newStopCmd() *cobra.Command {
	var grace time.Duration
	cmd := &cobra.Command{
		Use:   "stop <name>",
		Short: "SIGTERM-then-SIGKILL a sandboxed agent",
		Long: "Sends SIGTERM to the agent and waits up to --grace before escalating " +
			"to SIGKILL. Default grace is 5s. Returns once the agent has reaped.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			rt := appRuntimeFrom(c.Context())
			name := args[0]
			if name == "" {
				return UsageError(fmt.Errorf("agent name required"))
			}
			cl := rt.newClient()
			res, err := cl.StopAgent(c.Context(), name, int64(grace))
			if err != nil {
				return renderDaemonErr(rt, err)
			}
			if rt.JSON {
				return render.JSON(rt.Stdout, res)
			}
			render.HumanStopResult(rt.Stdout, res)
			return nil
		},
	}
	cmd.Flags().DurationVar(&grace, "grace", 5*time.Second, "grace period before SIGKILL escalation")
	return cmd
}
