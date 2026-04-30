package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/agent-sandbox/runtime/internal/manifest"
	"github.com/agent-sandbox/runtime/internal/render"
)

func newManifestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Manifest-handling subcommands",
	}
	cmd.AddCommand(newManifestValidateCmd())
	return cmd
}

func newManifestValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <manifest.yaml>",
		Short: "Parse and validate a manifest without contacting the daemon",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			rt := appRuntimeFrom(c.Context())
			abs, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolve manifest path: %w", err)
			}
			data, err := os.ReadFile(abs)
			if err != nil {
				return fmt.Errorf("read manifest: %w", err)
			}
			m, err := manifest.ParseBytes(abs, data)
			if err != nil {
				return printManifestError(rt, err)
			}
			if rt.JSON {
				return render.JSON(rt.Stdout, m)
			}
			fmt.Fprintf(rt.Stdout, "OK %s\n  policy: %s\n", m.Name, m.PolicySummary())
			return nil
		},
	}
}
