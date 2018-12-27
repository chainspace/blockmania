package pubsub // import "chainspace.io/blockmania/pubsub"

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"chainspace.io/blockmania/internal/log"
	"chainspace.io/blockmania/internal/log/fld"
	"chainspace.io/blockmania/pubsub/internal"
)

type Config struct {
	Port      uint
	NetworkID string
	NodeID    uint64
}

type Server interface {
	Close()
	Publish(tx []byte)
	RegisterNotifier(n Notifier)
}

type Notifier func(nodeID uint64, tx []byte)

type server struct {
	port      uint
	networkID string
	nodeID    uint64
	conns     map[string]*internal.Conn
	notifiers []Notifier
	mu        sync.Mutex
}

func (s *server) RegisterNotifier(n Notifier) {
	s.notifiers = append(s.notifiers, n)
}

func (s *server) handleConnection(conn net.Conn) {
	// may need to init with block number or sumbthing in the future
	s.conns[conn.RemoteAddr().String()] = internal.NewConn(conn)
}

func (s *server) listen(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Error("pubsub: Could not accept new connections", fld.Err(err))
		}
		go s.handleConnection(conn)
	}
}

func (s *server) Close() {
	for _, v := range s.conns {
		v.Close()
	}
}

func (s *server) Publish(tx []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload := internal.Payload{
		NodeID: s.nodeID,
		Tx:     tx,
	}
	// send to customs notifiers
	for _, notify := range s.notifiers {
		notify(payload.NodeID, payload.Tx)
	}

	b, _ := json.Marshal(&payload)
	badconns := []string{}
	for addr, c := range s.conns {
		if err := c.Write(b, 5*time.Second); err != nil {
			log.Error("unable to publish objectID", fld.Err(err))
			badconns = append(badconns, addr)
		}
	}
	// remove badconns
	for _, addr := range badconns {
		s.conns[addr].Close()
		delete(s.conns, addr)
	}
}

func New(cfg *Config) (*server, error) {
	var err error
	ln, err := net.Listen("tcp", fmt.Sprintf(":%v", cfg.Port))
	if err != nil {
		return nil, err
	}

	srv := &server{
		port:      cfg.Port,
		networkID: cfg.NetworkID,
		nodeID:    cfg.NodeID,
		conns:     map[string]*internal.Conn{},
	}
	go srv.listen(ln)

	return srv, nil
}
