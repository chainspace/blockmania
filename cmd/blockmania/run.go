package main

import (
	"flag"
	"fmt"
	"strings"

	"chainspace.io/blockmania/internal/fsutil"
)

func runCommand(args []string) int {
	var (
		networkName string
		nodeID      uint64
		configRoot  string
		runtimeRoot string
	)

	cmd := flag.NewFlagSet("run", flag.ContinueOnError)
	cmd.StringVar(&networkName, "network-name", "default", "Name of the network to be generated")
	cmd.StringVar(&configRoot, "config-root", fsutil.DefaultRootDir(), "Path to the Blockmania root directory")
	cmd.StringVar(&runtimeRoot, "runtime-root", fsutil.DefaultRootDir(), "Path to the runtime root directory")
	cmd.Uint64Var(&nodeID, "node-id", 0, "ID of the blockmania node to be started")

	cmd.Usage = func() {
		fmt.Fprintf(cmd.Output(), "%v\n\n", helpRun())
		cmd.PrintDefaults()
	}
	if err := cmd.Parse(args); err != nil {
		return 1
	}

	if nodeID == 0 {
		fmt.Fprintf(cmd.Output(), "Invalid or missing node ID\n")
		return 1
	}

	return 0
}

func helpRun() string {
	helpStr := `
Usage: blockmania run [options]

`
	return strings.TrimSpace(helpStr)
}
