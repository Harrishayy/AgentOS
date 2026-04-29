package main

import (
	"os"

	"github.com/agent-sandbox/cli/cmd/agentctl/agentctlcmd"
)

func main() {
	os.Exit(agentctlcmd.Main())
}
