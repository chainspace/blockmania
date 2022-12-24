package txlistener

import (
	"sync"

	"github.com/chainspace/blockmania/broadcast"
	"github.com/chainspace/blockmania/internal/log"
	"github.com/chainspace/blockmania/internal/log/fld"
	"github.com/chainspace/blockmania/node"
	"github.com/chainspace/blockmania/pubsub"
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
