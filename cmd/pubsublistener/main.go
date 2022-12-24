package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/chainspace/blockmania/pubsub/client"
)

var (
	addrs       string
	nodeCount   int
	networkName string
)

func init() {
	flag.StringVar(&addrs, "addrs", "", "list of addresses to listen to")
	flag.IntVar(&nodeCount, "node-count", 4, "number of node to connect to")
	flag.StringVar(&networkName, "network-name", "", "name of the network to use")
}

func splitAddresses(s string) map[uint64]string {
	split := strings.Split(s, ",")
	out := map[uint64]string{}
	for _, v := range split {
		sp := strings.Split(v, "=")
		i, err := strconv.Atoi(sp[0])
		if err != nil {
			fmt.Printf("error, unable to strconv nodeid: %v", err)
			os.Exit(1)
		}
		out[uint64(i)] = strings.TrimSpace(sp[1])
	}
	return out
}

func pubsubCallback(nodeID uint64, tx []byte) {
	tx64 := base64.StdEncoding.EncodeToString(tx)
	fmt.Printf("node-%v => tx=%v\n", nodeID, tx64)
}

func main() {
	flag.Parse()
	if len(addrs) <= 0 {
		fmt.Printf("error: at least 1 address required\n")
		os.Exit(1)
	}

	nodesAddr := map[uint64]string{}

	nodesAddr = splitAddresses(addrs)
	fmt.Printf("subscribing to:\n")
	for k, v := range nodesAddr {
		fmt.Printf("  node-%v => %v\n", k, v)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cfg := client.Config{
		NetworkName: networkName,
		NodeAddrs:   nodesAddr,
		CB:          pubsubCallback,
		Ctx:         ctx,
	}

	clt := client.New(&cfg)
	_ = clt
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	<-sigc
	cancel()
	fmt.Printf("exiting...\n")
}
