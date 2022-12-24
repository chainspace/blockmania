package node // import "github.com/chainspace/blockmania/node"

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base32"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chainspace/blockmania/broadcast"
	"github.com/chainspace/blockmania/config"
	"github.com/chainspace/blockmania/internal/crypto/signature"
	"github.com/chainspace/blockmania/internal/freeport"
	"github.com/chainspace/blockmania/internal/log"
	"github.com/chainspace/blockmania/internal/log/fld"
	"github.com/chainspace/blockmania/network"
	"github.com/chainspace/blockmania/service"
	"github.com/gogo/protobuf/proto"
)

const (
	dirPerms = 0700
)

// Error values.
var (
	errDirectoryMissing = errors.New("node: config is missing a value for Directory")
	errInvalidSignature = errors.New("node: invalid signature on hello")
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

type usedNonce struct {
	nonce []byte
	ts    time.Time
}

// Config represents the configuration of an individual node within a Chainspace network.
type Config struct {
	Directory   string
	Keys        *config.Keys
	Network     *config.Network
	NetworkName string
	NodeID      uint64
	Node        *config.Node
}

// Server represents a running Chainspace node.
type Server struct {
	Broadcast       *broadcast.Service
	cancel          context.CancelFunc
	ctx             context.Context
	id              uint64
	key             signature.KeyPair
	keys            map[uint64]signature.PublicKey
	readTimeout     time.Duration
	maxPayload      int
	mu              sync.RWMutex // protects nonceMap
	nonceExpiration time.Duration
	nonceMap        map[uint64][]usedNonce
	top             *network.Topology
	writeTimeout    time.Duration
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	c := network.NewConn(conn)
	hello, err := c.ReadHello(s.maxPayload, s.readTimeout)
	if err != nil {
		if network.AbnormalError(err) {
			log.Error("Unable to read hello message", fld.Err(err))
		}
		return
	}
	var (
		svc    service.Handler
		peerID uint64
	)
	switch hello.Type {
	case service.CONNECTION_BROADCAST:
		svc = s.Broadcast
		peerID, err = s.verifyPeerID(hello)
		if err != nil {
			log.Error("Unable to verify peer ID from the hello message", fld.Err(err))
			return
		}
	default:
		log.Error("Unknown connection type", fld.ConnectionType(int32(hello.Type)))
		return
	}
	for {
		msg, err := c.ReadMessage(s.maxPayload, s.readTimeout)

		if err != nil {
			if network.AbnormalError(err) {
				log.Error("Could not decode message from an incoming stream", fld.Err(err))
			}
			return
		}

		resp, err := svc.Handle(peerID, msg)
		if err != nil {
			// log.Error("Received error response", fld.Service(svc.Name()), fld.Err(err))
			conn.Close()
			return
		}
		if resp != nil {
			if err = c.WritePayload(resp, s.maxPayload, s.writeTimeout); err != nil {
				if network.AbnormalError(err) {
					log.Error("Unable to write response to peer", fld.PeerID(peerID), fld.Err(err))
				}
				return
			}
		}
	}
}

func (s *Server) listen(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			s.cancel()
			log.Fatal("Could not accept new connections", fld.Err(err))
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) pruneNonceMap() {
	exp := s.nonceExpiration
	tick := time.NewTicker(exp)
	var xs []usedNonce
	for {
		select {
		case <-tick.C:
			now := time.Now()
			nmap := map[uint64][]usedNonce{}
			s.mu.Lock()
			for nodeID, nonces := range s.nonceMap {
				xs = nil
				for _, used := range nonces {
					diff := now.Sub(used.ts)
					if diff < 0 {
						diff = -diff
					}
					if diff < exp {
						xs = append(xs, used)
					}
				}
				if xs != nil {
					nmap[nodeID] = xs
				}
			}
			s.nonceMap = nmap
			s.mu.Unlock()
		case <-s.ctx.Done():
			tick.Stop()
			return
		}
	}
}

func (s *Server) verifyPeerID(hello *service.Hello) (uint64, error) {
	if len(hello.Payload) == 0 {
		return 0, nil
	}
	info := &service.HelloInfo{}
	if err := proto.Unmarshal(hello.Payload, info); err != nil {
		return 0, err
	}
	if info.Server != s.id {
		return 0, fmt.Errorf("node: mismatched server field in hello payload: expected %d, got %d", s.id, info.Server)
	}
	key, exists := s.keys[info.Client]
	if !exists {
		return 0, fmt.Errorf("node: could not find public signing key for node %d", info.Client)
	}
	if !key.Verify(hello.Payload, hello.Signature) {
		return 0, fmt.Errorf("node: could not validate signature claiming to be from node %d", info.Client)
	}
	diff := time.Now().Sub(info.Timestamp)
	if diff < 0 {
		diff = -diff
	}
	if diff > s.nonceExpiration {
		return 0, fmt.Errorf("node: timestamp in client hello is outside of the max clock skew range: %d", diff)
	}
	s.mu.RLock()
	nonces, exists := s.nonceMap[info.Client]
	if exists {
		for _, used := range nonces {
			if bytes.Equal(used.nonce, info.Nonce) {
				s.mu.RUnlock()
				return 0, fmt.Errorf("node: received duplicate nonce from %d", info.Client)
			}
		}
	}
	s.mu.RUnlock()
	return info.Client, nil
}

// Shutdown closes all underlying resources associated with the node.
func (s *Server) Shutdown() {
	s.cancel()
}

func createDir(path string) error {
	_, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		log.Info("Creating directory", fld.Path(path))
		if err = os.MkdirAll(path, dirPerms); err != nil {
			return err
		}
	}
	return nil
}

// Init initialize the the fs for the node runtime
func EnsureRuntimeDirs(cfg *Config) (string, error) {
	var err error
	// Initialise the runtime directory for the node.
	dir := cfg.Directory
	if dir == "" {
		return "", errDirectoryMissing
	}

	if !filepath.IsAbs(dir) {
		if dir, err = filepath.Abs(dir); err != nil {
			return "", err
		}
	}

	if err := createDir(dir); err != nil {
		return "", err
	}

	if cfg.Node.Logging.FileLevel >= log.DebugLevel && cfg.Node.Logging.FilePath != "" {
		logfile := filepath.Join(dir, cfg.Node.Logging.FilePath)
		if err := createDir(filepath.Dir(logfile)); err != nil {
			return "", err
		}
		if err := log.ToFile(logfile, cfg.Node.Logging.FileLevel); err != nil {
			log.Fatal("Could not initialise the file logger", fld.Err(err))
		}
	}

	return dir, nil
}

// Run initialises a node with the given config.
func Run(dir string, cfg *Config) (*Server, error) {
	var err error

	// Initialise the topology.
	top, err := network.New(cfg.NetworkName, cfg.Network)
	if err != nil {
		return nil, err
	}

	// Bootstrap using mDNS.
	if cfg.Node.Bootstrap.MDNS {
		top.BootstrapMDNS()
	}

	// Bootstrap via network registries.
	if cfg.Node.Bootstrap.Registry {
		top.BootstrapRegistries(cfg.Node.Registries)
	}

	// Bootstrap using a static map of addresses.
	if cfg.Node.Bootstrap.Static != nil {
		if err = top.BootstrapStatic(cfg.Node.Bootstrap.Static); err != nil {
			return nil, err
		}
	}

	// Get a port to listen on.
	port, err := freeport.TCP("")
	if err != nil {
		return nil, err
	}

	// Initialise the TLS cert.
	cert, err := tls.X509KeyPair([]byte(cfg.Keys.TransportCert.Public), []byte(cfg.Keys.TransportCert.Private))
	if err != nil {
		return nil, fmt.Errorf("node: could not load the X.509 transport.cert from config: %s", err)
	}
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	// Initialise signing keys.
	var key signature.KeyPair
	switch cfg.Keys.SigningKey.Type {
	case "ed25519":
		pubkey, err := b32.DecodeString(cfg.Keys.SigningKey.Public)
		if err != nil {
			return nil, fmt.Errorf("node: could not decode the base32-encoded public signing.key from config: %s", err)
		}
		privkey, err := b32.DecodeString(cfg.Keys.SigningKey.Private)
		if err != nil {
			return nil, fmt.Errorf("node: could not decode the base32-encoded private signing.key from config: %s", err)
		}
		key, err = signature.LoadKeyPair(signature.Ed25519, append(pubkey, privkey...))
		if err != nil {
			return nil, fmt.Errorf("node: unable to load the signing.key from config: %s", err)
		}
	default:
		return nil, fmt.Errorf("node: unknown type of signing.key found in config: %q", cfg.Keys.SigningKey.Type)
	}

	keys := top.SeedPublicKeys()
	nodes := top.Nodes()
	if len(nodes) == 0 {
		return nil, fmt.Errorf("node: got no peers for the node %d", cfg.NodeID)
	}

	log.SetGlobal(fld.SelfNodeID(cfg.NodeID))

	idx := 0
	peers := make([]uint64, len(nodes)-1)
	for _, peer := range nodes {
		if peer != cfg.NodeID {
			peers[idx] = peer
			idx++
		}
	}

	maxPayload, err := cfg.Network.MaxPayload.Int()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	bcfg := &broadcast.Config{
		Broadcast:     cfg.Node.Broadcast,
		Connections:   cfg.Node.Connections,
		Directory:     dir,
		Key:           key,
		Keys:          keys,
		MaxPayload:    maxPayload,
		NetConsensus:  cfg.Network.Consensus,
		NodeConsensus: cfg.Node.Consensus,
		NodeID:        cfg.NodeID,
		Peers:         peers,
	}

	broadcaster, err := broadcast.New(ctx, bcfg, top)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("node: unable to instantiate the broadcast service: %s", err)
	}

	node := &Server{
		Broadcast:       broadcaster,
		cancel:          cancel,
		ctx:             ctx,
		id:              cfg.NodeID,
		key:             key,
		keys:            keys,
		maxPayload:      maxPayload,
		nonceExpiration: cfg.Network.Consensus.NonceExpiration,
		nonceMap:        map[uint64][]usedNonce{},
		readTimeout:     cfg.Node.Connections.ReadTimeout,
		top:             top,
		writeTimeout:    cfg.Node.Connections.WriteTimeout,
	}

	// Start listening on the given port.
	l, err := tls.Listen("tcp", fmt.Sprintf(":%d", port), tlsConf)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("node: unable to listen on port %d: %s", port, err)
	}

	go node.pruneNonceMap()
	go node.listen(l)

	// Announce our location to the world.
	if cfg.Node.Announce.MDNS {
		if err = announceMDNS(cfg.Network.ID, cfg.NodeID, port); err != nil {
			return nil, err
		}
	}
	if cfg.Node.Announce.Registry {
		if err := announceRegistry(cfg.Node.Registries, cfg.Network.ID, cfg.NodeID, port); err != nil {
			return nil, err
		}
	}

	log.Info("Node is running", fld.NetworkName(cfg.NetworkName), fld.Port(port))
	log.Info("Runtime directory", fld.Path(dir))
	return node, nil
}
