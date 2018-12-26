package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"chainspace.io/blockmania/internal/fsutil"
	"chainspace.io/blockmania/internal/log"
	"chainspace.io/blockmania/node"
	"chainspace.io/blockmania/rest"
)

func runCommand(args []string) int {
	var (
		networkName string
		nodeID      uint64
		configRoot  string
		runtimeRoot string
		consoleLog  string
		fileLog     string
	)

	cmd := flag.NewFlagSet("run", flag.ContinueOnError)
	cmd.StringVar(&networkName, "network-name", "default", "Name of the network to be generated")
	cmd.StringVar(&configRoot, "config-root", fsutil.DefaultRootDir(), "Path to the Blockmania root directory")
	cmd.StringVar(&runtimeRoot, "runtime-root", "", "Path to the runtime root directory")
	cmd.Uint64Var(&nodeID, "node-id", 0, "ID of the blockmania node to be started")
	cmd.StringVar(&consoleLog, "console-log", "", "Level of the log to the console")
	cmd.StringVar(&fileLog, "file-log", "", "Level of the log to write to file")

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

	// load the configuration
	cfg, err := node.LoadConfiguration(networkName, configRoot, runtimeRoot, nodeID)
	if err != nil {
		fmt.Fprintf(cmd.Output(), "%v", err)
		return 1
	}

	// setup loging
	if len(consoleLog) > 0 {
		switch consoleLog {
		case "debug":
			log.ToConsole(log.DebugLevel)
		case "error":
			log.ToConsole(log.ErrorLevel)
		case "fatal":
			log.ToConsole(log.FatalLevel)
		case "info":
			log.ToConsole(log.InfoLevel)
		default:
			fmt.Fprintf(cmd.Output(), "Unknown console-log level `%v`\n", consoleLog)
			return 1
		}
	} else {
		log.ToConsole(cfg.Node.Logging.ConsoleLevel)
	}

	if len(fileLog) > 0 {
		switch fileLog {
		case "debug":
			cfg.Node.Logging.FileLevel = log.DebugLevel
		case "error":
			cfg.Node.Logging.FileLevel = log.ErrorLevel
		case "fatal":
			cfg.Node.Logging.FileLevel = log.FatalLevel
		case "info":
			cfg.Node.Logging.FileLevel = log.InfoLevel
		default:
			fmt.Fprintf(cmd.Output(), "Unknown file-log level `%v`\n", consoleLog)
			return 1
		}
	}

	// init/start the node
	nodesrv, err := node.Run(cfg)
	if err != nil {
		fmt.Fprintf(cmd.Output(), "Could not start node-%v, %v", nodeID, err)
		return 1
	}

	restsrv := rest.New(cfg.Node.HTTP.Port, nodesrv)
	restsrv.Start()

	defer func() {
		if restsrv != nil {
			restsrv.Shutdown()
		}
		if nodesrv != nil {
			nodesrv.Shutdown()
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM)
	<-c

	return 0
}

func helpRun() string {
	helpStr := `
Usage: blockmania run [options]

`
	return strings.TrimSpace(helpStr)
}
