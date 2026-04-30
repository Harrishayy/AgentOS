package cli

import (
	"runtime"

	"github.com/spf13/cobra"

	"github.com/agent-sandbox/runtime/internal/render"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print agentctl build and runtime info",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rt := appRuntimeFrom(c.Context())
			payload := struct {
				Build    string `json:"build"`
				Go       string `json:"go"`
				OS       string `json:"os"`
				Arch     string `json:"arch"`
				Protocol string `json:"protocol"`
			}{
				Build:    Build,
				Go:       runtime.Version(),
				OS:       runtime.GOOS,
				Arch:     runtime.GOARCH,
				Protocol: "v1",
			}
			if rt.JSON {
				return render.JSON(rt.Stdout, payload)
			}
			cmd := c
			cmd.Printf("agentctl %s\n  go: %s\n  platform: %s/%s\n  protocol: %s\n",
				payload.Build, payload.Go, payload.OS, payload.Arch, payload.Protocol)
			return nil
		},
	}
}
