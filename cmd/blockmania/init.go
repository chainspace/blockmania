package main

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"chainspace.io/blockmania/config/generate"
	"chainspace.io/blockmania/internal/fsutil"
)

func initCommand(args []string) int {
	var (
		networkName   string
		configRoot    string
		registry      string
		roundInterval time.Duration
		httpPort      uint
		pubsubPort    uint
		nodeCount     uint
		fixedPort     bool
		force         bool
	)

	cmd := flag.NewFlagSet("init", flag.ContinueOnError)
	cmd.StringVar(&networkName, "network-name", "default", "Name of the network to be generated")
	cmd.StringVar(&configRoot, "config-root", fsutil.DefaultRootDir(), "Path to the Chainspace root directory")
	cmd.StringVar(&registry, "registry", "", "Address of the network registry")
	cmd.DurationVar(&roundInterval, "round-interval", 1*time.Second, "Round interval")
	cmd.UintVar(&httpPort, "http-port", 8000, "Http port used by the node, if not specified will be incremental from 8000")
	cmd.UintVar(&pubsubPort, "pubsub-port", 7000, "Port used for the pubsub socket, if not specified will be incremental from 7000")
	cmd.UintVar(&nodeCount, "node-count", 4, "Number of node part of this blockmanias network")
	cmd.BoolVar(&fixedPort, "fixed-port", true, "The HTTP/pubsub ports are incremented for each node (useful when deploying multiple node on the same host)")
	cmd.BoolVar(&force, "f", false, "Force re-writting the configuration folder")

	cmd.Usage = func() {
		fmt.Fprintf(cmd.Output(), "%v\n\n", helpInit())
		cmd.PrintDefaults()
	}
	if err := cmd.Parse(args); err != nil {
		return 1
	}

	cfg := &generate.Config{
		NetworkName:   networkName,
		ConfigRoot:    configRoot,
		Registry:      registry,
		RoundInterval: roundInterval,
		HTTPPort:      httpPort,
		PubSubPort:    pubsubPort,
		NodeCount:     nodeCount,
		FixedPort:     fixedPort,
		DisableHTTP:   false, // false by default with a standalone blockmania
		DisablePubSub: false, // false by default with a standalone blockmania
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(cmd.Output(), "Configuration error: %v\n", err)
		return 1
	}

	if err := generate.Generate(cfg); err != nil {
		fmt.Fprintf(cmd.Output(), "Unable to generate configuration: %v\n", err)
		return 1
	}

	return 0
}

func helpInit() string {
	helpStr := `
Usage: blockmania init [options]

If the -registry host is specified, then a 36-byte token is randomly
generated and the hex-encoded form is set as a shared secret across
all of the initialised nodes.

`
	return strings.TrimSpace(helpStr)
}
