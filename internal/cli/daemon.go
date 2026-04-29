package cli

import (
	"github.com/spf13/cobra"

	"github.com/agent-sandbox/cli/internal/render"
)

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Daemon-side helpers (status, etc.)",
	}
	cmd.AddCommand(newDaemonStatusCmd())
	return cmd
}

func newDaemonStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Probe the daemon for liveness, version, and agent count",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rt := appRuntimeFrom(c.Context())
			cl := rt.newClient()
			res, err := cl.DaemonStatus(c.Context())
			if err != nil {
				return renderDaemonErr(rt, err)
			}
			if rt.JSON {
				return render.JSON(rt.Stdout, res)
			}
			render.HumanDaemonStatus(rt.Stdout, res)
			return nil
		},
	}
}
