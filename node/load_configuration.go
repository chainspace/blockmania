package node

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"chainspace.io/blockmania/config"
)

func LoadConfiguration(networkName, configRoot, runtimeRoot string, nodeID uint64) (*Config, error) {
	_, err := os.Stat(configRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(
				"Could not find the blockmania root directory `%v`", configRoot)
		}
		return nil, fmt.Errorf("Unable to access the blockmania root directory `%v`, %v", configRoot, err)
	}

	netPath := filepath.Join(configRoot, networkName)
	netCfg, err := config.LoadNetwork(filepath.Join(netPath, "network.yaml"))
	if err != nil {
		return nil, fmt.Errorf("Could not load network.yaml, %v", err)
	}

	nodeDir := "node-" + strconv.FormatUint(nodeID, 10)
	nodePath := filepath.Join(netPath, nodeDir)
	nodeCfg, err := config.LoadNode(filepath.Join(nodePath, "node.yaml"))
	if err != nil {
		return nil, fmt.Errorf("Could not load node.yaml, %v", err)
	}

	keys, err := config.LoadKeys(filepath.Join(nodePath, "keys.yaml"))
	if err != nil {
		return nil, fmt.Errorf("Could not load keys.yaml, %v", err)
	}

	root := configRoot
	if len(runtimeRoot) != 0 {
		root = os.ExpandEnv(runtimeRoot)
	}

	cfg := &Config{
		Directory:   filepath.Join(root, networkName, nodeDir),
		Keys:        keys,
		Network:     netCfg,
		NetworkName: networkName,
		NodeID:      nodeID,
		Node:        nodeCfg,
	}

	return cfg, nil
}
