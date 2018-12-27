package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"chainspace.io/blockmania/pubsub"
	"github.com/gorilla/websocket"
)

type WriteMessageCloser interface {
	io.Closer
	WriteMessage(int, []byte) error
}

type wsservice struct {
	ctx       context.Context
	pbService pubsub.Server
	chans     map[string]*chanCloser
	mu        sync.Mutex // locks chans
}

type WSService interface {
	Websocket(string, WriteMessageCloser) (int, error)
	Callback(nodeID uint64, tx []byte)
}

type payload struct {
	NodeID uint64 `json:"nodeId"`
	Tx     []byte `json:"tx"`
}

type chanCloser struct {
	C     chan payload
	close bool
	mu    sync.Mutex
}

func (c *chanCloser) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.close = true
}

func NewWS(ctx context.Context, pbService pubsub.Server) WSService {
	srv := &wsservice{
		ctx:       ctx,
		pbService: pbService,
		chans:     map[string]*chanCloser{},
	}
	pbService.RegisterNotifier(srv.Callback)
	return srv
}

func (s *wsservice) Callback(nodeID uint64, tx []byte) {
	go func(nodeID uint64, tx []byte) {
		s.mu.Lock()
		defer s.mu.Unlock()
		doneConn := []string{}
		for k, ch := range s.chans {
			ch.mu.Lock()
			if ch.close == true {
				close(ch.C)
				doneConn = append(doneConn, k)
			} else {
				ch.C <- payload{nodeID, tx}
			}
			ch.mu.Unlock()
		}
		for _, v := range doneConn {
			delete(s.chans, v)
		}
	}(nodeID, tx)
}

func (s *wsservice) addChan(cltID string, c *chanCloser) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chans[cltID] = c
}

func (s *wsservice) Websocket(cltID string, conn WriteMessageCloser) (int, error) {
	ch := &chanCloser{
		C: make(chan payload, 100),
	}
	s.addChan(cltID, ch)
	for {
		select {
		case <-s.ctx.Done():
			ch.Close()
			if err := conn.Close(); err != nil {
				return http.StatusInternalServerError, err
			}
			return http.StatusOK, s.ctx.Err()
		case p := <-ch.C:
			msg, err := json.Marshal(p)
			if err != nil {
				ch.Close()
				return http.StatusInternalServerError, err
			}
			err = conn.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				ch.Close()
				return http.StatusInternalServerError, err
			}
		}
	}
}
