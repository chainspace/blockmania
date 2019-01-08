package main

import (
	"fmt"
	"os"
	"strings"

	"chainspace.io/blockmania/internal/exitutil"
)

func main() {
	args := os.Args[1:]
	retVal := 0

	if len(args) <= 0 {
		printHelp()
		retVal = 1
	} else {

		switch args[0] {
		case "help":
			printHelp()
		case "init":
			retVal = initCommand(args[1:])
		case "run":
			retVal = runCommand(args[1:])
		default:
			printHelp()
			retVal = 1
		}
	}

	exitutil.Exit(retVal)
}

func printHelp() {
	fmt.Println(helpStr())
}

func helpStr() string {
	helpStr := `
Usage: blockmania <command> [args]

The available commands for execution are listed below.

Commands:
    init               Initialize a new blockmania network
    run                Start the execution of a blockmania node
    help               Display this help
`
	return strings.TrimSpace(helpStr)
}
