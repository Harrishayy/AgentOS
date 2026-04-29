package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Print shell completion script",
		Long: "Print a completion script for the requested shell. To enable for the " +
			"current session, evaluate the output, e.g.:\n\n  source <(agentctl completion bash)\n\n" +
			"Persistent installation is shell-specific; consult your shell's docs.",
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(c *cobra.Command, args []string) error {
			rt := appRuntimeFrom(c.Context())
			switch args[0] {
			case "bash":
				return c.Root().GenBashCompletion(rt.Stdout)
			case "zsh":
				return c.Root().GenZshCompletion(rt.Stdout)
			case "fish":
				return c.Root().GenFishCompletion(rt.Stdout, true)
			case "powershell":
				return c.Root().GenPowerShellCompletionWithDesc(rt.Stdout)
			default:
				return UsageError(fmt.Errorf("unsupported shell %q", args[0]))
			}
		},
	}
	return cmd
}
