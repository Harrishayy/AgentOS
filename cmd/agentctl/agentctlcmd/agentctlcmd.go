// Package agentctlcmd exposes the binary's main loop as a callable function.
// Both cmd/agentctl/main.go and the testscript-driven e2e suite invoke Main
// to get identical entry-point behaviour (signal trapping, exit-code mapping).
package agentctlcmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/agent-sandbox/cli/internal/cli"
)

// Main runs the CLI. Returns the appropriate exit code; never calls os.Exit.
func Main() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	root := cli.NewRoot()
	if err := root.ExecuteContext(ctx); err != nil {
		if !cli.ErrorAlreadyPrinted(err) {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		}
		return cli.MapExitCode(err)
	}
	return cli.ExitOK
}
