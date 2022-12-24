package api

import (
	"net/http"

	"github.com/chainspace/blockmania/rest/service"
	"github.com/gorilla/websocket"
)

const (
	readBufferSize  = 1024
	writeBufferSize = 1024
)

type WSUpgrader interface {
	Upgrade(w http.ResponseWriter, r *http.Request) (service.WriteMessageCloser, error)
}

type upgrader struct{}

func (u upgrader) Upgrade(w http.ResponseWriter, r *http.Request) (service.WriteMessageCloser, error) {
	var wsupgrader = websocket.Upgrader{
		ReadBufferSize:  readBufferSize,
		WriteBufferSize: writeBufferSize,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}

	conn, err := wsupgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
