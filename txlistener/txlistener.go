package txlistener

import (
	"sync"

	"chainspace.io/blockmania/broadcast"
	"chainspace.io/blockmania/internal/log"
	"chainspace.io/blockmania/internal/log/fld"
	"chainspace.io/blockmania/node"
	"chainspace.io/blockmania/pubsub"
)

type Listener struct {
	node   *node.Server
	pubsub pubsub.Server
	mu     sync.Mutex
}

func New(node *node.Server, pubsub pubsub.Server) *Listener {
	lst := &Listener{node: node, pubsub: pubsub}
	node.Broadcast.Register(lst.handleDeliver)
	return lst
}

func (l *Listener) handleDeliver(round uint64, blocks []*broadcast.SignedData) {
	l.mu.Lock()
	for _, signed := range blocks {
		block, err := signed.Block()
		if err != nil {
			log.Error("Unable to decode delivered block", fld.Err(err))
		}
		it := block.Iter()
		for it.Valid() {
			it.Next()
			l.pubsub.Publish(it.TxData)
		}
	}
	l.mu.Unlock()
	l.node.Broadcast.Acknowledge(round)
}
