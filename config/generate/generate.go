package generate

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/chainspace/blockmania/config"
	"github.com/chainspace/blockmania/internal/crypto/signature"
	"github.com/chainspace/blockmania/internal/crypto/transport"
	"github.com/chainspace/blockmania/internal/fsutil"
	"github.com/chainspace/blockmania/internal/log"
	"github.com/chainspace/blockmania/internal/log/fld"

	yaml "gopkg.in/yaml.v2"
)

var (
	ErrRoundIntervalZero       = errors.New("Round interval cannot be 0s")
	ErrInvalidHTTPOrPubsubPort = errors.New("Invalid HTTP or pubsub port")
	ErrInvalidRegistryURL      = errors.New("Invalid registry URL")
	ErrInvalidNodeCount        = errors.New("Invalid node count (min. 4 node required)")

	b32 = base32.StdEncoding.WithPadding(base32.NoPadding)
)

type Config struct {
	NetworkName   string
	ConfigRoot    string
	Registry      string
	RoundInterval time.Duration
	HTTPPort      uint
	PubSubPort    uint
	NodeCount     uint
	FixedPort     bool
	DisableHTTP   bool
	DisablePubSub bool
}

func (c *Config) Validate() error {
	if c.RoundInterval == 0 {
		return ErrRoundIntervalZero
	}

	if (c.PubSubPort == 0 && !c.DisablePubSub) ||
		(c.HTTPPort == 0 && !c.DisableHTTP) ||
		(c.HTTPPort == c.PubSubPort) {
		return ErrInvalidHTTPOrPubsubPort
	}

	_, err := url.Parse(c.Registry)
	if err != nil {
		return ErrInvalidRegistryURL
	}

	if c.NodeCount < 4 {
		return ErrInvalidNodeCount
	}

	return nil
}

func Generate(cfg *Config) error {
	if err := fsutil.EnsureDir(cfg.ConfigRoot); err != nil {
		return fmt.Errorf("Could not ensure the existence of the root directory ( %v)", err)
	}
	networkDir := filepath.Join(cfg.ConfigRoot, cfg.NetworkName)
	if err := fsutil.CreateUnlessExists(networkDir); err != nil {
		return err
	}

	announce := &config.Announce{}
	bootstrap := &config.Bootstrap{}
	consensus := &config.NetConsensus{
		BlockReferencesSizeLimit:   10 * config.MB,
		BlockTransactionsSizeLimit: 100 * config.MB,
		NonceExpiration:            30 * time.Second,
		RoundInterval:              cfg.RoundInterval,
		ViewTimeout:                15,
	}

	peers := map[uint64]*config.Peer{}
	registries := []config.Registry{}
	if len(cfg.Registry) <= 0 {
		announce.MDNS = true
		bootstrap.MDNS = true
	} else {
		announce.Registry = true
		bootstrap.Registry = true
		token := make([]byte, 36)
		_, err := rand.Read(token)
		if err != nil {
			return fmt.Errorf("Unable to generate random token for use with the network registry (%v)", err)
		}
		registries = append(registries, config.Registry{
			Host:  cfg.Registry,
			Token: hex.EncodeToString(token),
		})
	}

	network := &config.Network{
		Consensus:  consensus,
		MaxPayload: 128 * config.MB,
		SeedNodes:  peers,
	}

	broadcast := &config.Broadcast{
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     2 * time.Second,
	}

	connections := &config.Connections{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	logging := &config.Logging{
		ConsoleLevel: log.DebugLevel,
		FileLevel:    log.ErrorLevel,
		FilePath:     "log/blockmania.log",
	}

	rateLimit := &config.RateLimit{
		InitialRate:  10000,
		RateDecrease: 0.8,
		RateIncrease: 1000,
	}

	storage := &config.Storage{
		Type: "badger",
	}

	httpPort := cfg.HTTPPort
	pubsubPort := cfg.PubSubPort

	for i := 1; i <= int(cfg.NodeCount); i++ {
		fmt.Printf("Generating node %v:node-%v\n", cfg.NetworkName, uint64(i))

		if !cfg.FixedPort && !cfg.DisableHTTP {
			httpPort += 1
		}

		if !cfg.FixedPort && !cfg.DisablePubSub {
			pubsubPort += 1

		}

		nodeID := uint64(i)
		dirName := fmt.Sprintf("node-%d", i)
		nodeDir := filepath.Join(networkDir, dirName)
		if err := fsutil.CreateUnlessExists(nodeDir); err != nil {
			return err
		}

		httpcfg := config.ServiceEnable{
			Enabled: !cfg.DisableHTTP,
			Port:    httpPort,
		}
		pubsubcfg := config.ServiceEnable{
			Enabled: !cfg.DisablePubSub,
			Port:    pubsubPort,
		}

		// Create keys.yaml
		signingKey, cert, err := genKeys(filepath.Join(nodeDir, "keys.yaml"), cfg.NetworkName, nodeID)
		if err != nil {
			return fmt.Errorf("Could not generate keys (%v)", err)
		}

		consensus := &config.NodeConsensus{
			DriftTolerance:      10 * time.Millisecond,
			InitialWorkDuration: 100 * time.Millisecond,
			RateLimit:           rateLimit,
		}

		// Create node.yaml
		cfg := &config.Node{
			Announce:    announce,
			Bootstrap:   bootstrap,
			Broadcast:   broadcast,
			Connections: connections,
			Consensus:   consensus,
			HTTP:        httpcfg,
			Logging:     logging,
			Pubsub:      pubsubcfg,
			Registries:  registries,
			Storage:     storage,
		}

		if err := writeYAML(filepath.Join(nodeDir, "node.yaml"), cfg); err != nil {
			log.Fatal("Could not write to node.yaml", fld.Err(err))
		}

		peers[nodeID] = &config.Peer{
			SigningKey: &config.PeerKey{
				Type:  signingKey.Algorithm().String(),
				Value: b32.EncodeToString(signingKey.PublicKey().Value()),
			},
			TransportCert: &config.PeerKey{
				Type:  cert.Type.String(),
				Value: cert.Public,
			},
		}

	}

	networkID, err := network.Hash()
	if err != nil {
		log.Fatal("Could not generate the Network ID", fld.Err(err))
	}

	network.ID = b32.EncodeToString(networkID)
	if err := writeYAML(filepath.Join(networkDir, "network.yaml"), network); err != nil {
		log.Fatal("Could not write to network.yaml", fld.Err(err))
	}
	return nil
}

func genKeys(path string, networkID string, nodeID uint64) (signature.KeyPair, *transport.Cert, error) {
	signingKey, err := signature.GenKeyPair(signature.Ed25519)
	if err != nil {
		return nil, nil, fmt.Errorf("could not generate signing key: %s", err)
	}
	cert, err := transport.GenCert(transport.ECDSA, networkID, nodeID)
	if err != nil {
		return nil, nil, fmt.Errorf("could not generate transport cert: %s", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	cfg := config.Keys{
		SigningKey: &config.Key{
			Private: b32.EncodeToString(signingKey.PrivateKey().Value()),
			Public:  b32.EncodeToString(signingKey.PublicKey().Value()),
			Type:    signingKey.Algorithm().String(),
		},
		TransportCert: &config.Key{
			Private: cert.Private,
			Public:  cert.Public,
			Type:    cert.Type.String(),
		},
	}
	enc := yaml.NewEncoder(f)
	err = enc.Encode(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("could not write data to %s: %s", path, err)
	}
	return signingKey, cert, nil
}

func writeYAML(path string, v interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := yaml.NewEncoder(f)
	return enc.Encode(v)
}
