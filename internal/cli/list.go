package cli

import (
	"github.com/spf13/cobra"

	"github.com/agent-sandbox/runtime/internal/render"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sandboxed agents currently tracked by the daemon",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rt := appRuntimeFrom(c.Context())
			cl := rt.newClient()
			res, err := cl.ListAgents(c.Context())
			if err != nil {
				return renderDaemonErr(rt, err)
			}
			if rt.JSON {
				return render.JSON(rt.Stdout, res)
			}
			render.HumanList(rt.Stdout, res.Agents)
			return nil
		},
	}
	return cmd
}
