package blockmania

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/chainspace/blockmania/blockmania/messages"
	"github.com/chainspace/blockmania/blockmania/states"
	"github.com/chainspace/blockmania/internal/log"
	"github.com/chainspace/blockmania/internal/log/fld"
)

type entry struct {
	block BlockID
	deps  []BlockID
	prev  BlockID
}

type blockInfo struct {
	data *BlockGraph
	max  uint64
}

// Config represents the configuration of a blockmania Graph.
type Config struct {
	LastInterpreted uint64
	Peers           []uint64
	SelfID          uint64
	TotalNodes      uint64
}

// Graph represents the graph that is generated from the nodes in a shard
// broadcasting to each other.
type Graph struct {
	blocks     []*blockInfo
	callback   func(*Interpreted)
	ctx        context.Context
	entries    chan *BlockGraph
	max        map[BlockID]uint64
	mutex      sync.Mutex // protects blocks, max
	peerCount  int
	peers      []uint64
	quorumf1   int
	quorum2f   int
	quorum2f1  int
	resolved   map[uint64]map[uint64]string
	round      uint64
	self       uint64
	statess    map[BlockID]*state
	totalNodes uint64
}

func (graph *Graph) deliver(node uint64, round uint64, hash string) {
	if round < graph.round {
		return
	}
	hashes, exists := graph.resolved[round]
	if exists {
		curhash, exists := hashes[node]
		if exists {
			if curhash != hash {
				log.Fatal("Mismatching block hash for delivery", fld.NodeID(node), fld.Round(round))
			}
		} else {
			if log.AtDebug() {
				log.Debug("Consensus achieved", fld.BlockID(BlockID{
					Hash:  hash,
					Node:  node,
					Round: round,
				}))
			}
			hashes[node] = hash
		}
	} else {
		hashes = map[uint64]string{
			node: hash,
		}
		graph.resolved[round] = hashes
	}
	if round != graph.round {
		return
	}
	if len(hashes) == graph.peerCount {
		graph.deliverRound(round, hashes)
	}
}

func (graph *Graph) deliverRound(round uint64, hashes map[uint64]string) {
	var blocks []BlockID
	for node, hash := range hashes {
		if hash == "" {
			continue
		}
		blocks = append(blocks, BlockID{Hash: hash, Node: node, Round: round})
	}
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Hash < blocks[j].Hash
	})
	delete(graph.resolved, round)
	graph.mutex.Lock()
	consumed := graph.blocks[0].data.Block.Round - 1
	idx := 0
	for i, info := range graph.blocks {
		if info.max > round {
			break
		}
		delete(graph.max, info.data.Block)
		delete(graph.statess, info.data.Block)
		for _, dep := range info.data.Deps {
			delete(graph.max, dep.Block)
			delete(graph.statess, dep.Block)
		}
		consumed++
		idx = i + 1
	}
	if idx > 0 {
		graph.blocks = graph.blocks[idx:]
	}
	graph.round++
	if log.AtDebug() {
		log.Debug("Mem usage:", log.Int("g.max", len(graph.max)), log.Int("g.statess", len(graph.statess)),
			log.Int("g.blocks", len(graph.blocks)))
	}
	graph.mutex.Unlock()
	graph.callback(&Interpreted{
		Blocks:   blocks,
		Consumed: consumed,
		Round:    round,
	})
	hashes, exists := graph.resolved[round+1]
	if exists && len(hashes) == graph.peerCount {
		graph.deliverRound(round+1, hashes)
	}
}

func (graph *Graph) findOrCreateState(e *entry) *state {
	var stat *state
	if e.prev.Valid() {
		stat = graph.statess[e.prev].clone(graph.round)
	} else {
		stat = &state{
			timeouts: map[uint64][]timeout{},
		}
	}
	return stat
}

// This should be pulled out into a separate module, it's actually the consensus interpreter
// (where we interpret the block graph to see if consensus has been achieved)
func (graph *Graph) process(ntry *entry) {

	// find or create the state for this entry
	var state = graph.findOrCreateState(ntry)

	if log.AtDebug() {
		log.Debug("Interpreting block", fld.BlockID(ntry.block))
	}

	node, round, hash := ntry.block.Node, ntry.block.Round, ntry.block.Hash

	// set up a vec of out messages as per the interpretation protocol
	// we get a vec with the first message being a pre-prepare marked with the node, round, and hash of the block_id
	out := []messages.Message{messages.PrePrepare{
		Hash:  hash,
		Node:  node,
		Round: round,
		View:  0,
	}}

	if len(ntry.deps) > 0 {
		if state.delay == nil {
			state.delay = map[uint64]uint64{}
		}
		for _, dep := range ntry.deps {
			state.delay[dep.Node] = diff(round, dep.Round) * 10
		}
	}

	// timeout, a logical clock value
	tval := uint64(10)

	// if we have more delays than commit quorum
	if len(state.delay) > graph.quorum2f1 {
		vals := make([]uint64, len(state.delay))
		i := 0

		// grab all the delays
		for _, val := range state.delay {
			vals[i] = val
			i++
		}

		// sort the delays by value
		sort.Slice(vals, func(i, j int) bool {
			return vals[i] < vals[j]
		})

		// take the delay from 1 less than commit quorum
		xval := vals[graph.quorum2f]

		// if that delay is greater than the timeout
		if xval > tval {
			// set the timeout to that delay
			tval = xval
		}
	} else {
		// we have less delays than commit quorum
		// so we take the max delay as the timeout value
		for _, val := range state.delay {
			if val > tval {
				tval = val
			}
		}
	}

	// set the timeout value we got from all the above
	state.timeout = tval

	// set the timeouts for the current round based on the timeout value and current entry's round number
	tround := round + tval
	for _, xnode := range graph.peers {
		state.timeouts[tround] = append(state.timeouts[tround], timeout{
			node:  xnode,
			round: round,
			view:  0,
		})
	}

	// for the timeout values in this block's state timeouts for this round
	for _, tmout := range state.timeouts[round] {
		if _, exists := state.data[states.Final{Node: tmout.node, Round: tmout.round}]; exists {
			// If we've already reached finality, we don't need to do anything else, ditch out of the loop.
			// Seems like there's maybe a way to not do all that above work if we've already reached finality? Shouldn't we check this first?
			// We've already reached finality
			continue
		}

		// if we haven't reached finality, we need to do some more work

		// maybe some kind of local view number?
		var v uint32

		// create a view key from the timeout node and round
		skey := states.View{Node: tmout.node, Round: tmout.round}

		// if the state data has a value for the view key from the timeout node and round
		if sval, exists := state.data[skey]; exists {
			// set the view number variable to that value
			v = sval.(uint32)
		}

		// if the view number is greater than the timeout view
		if v > tmout.view {
			// We've already moved view
			// so just skip the rest of the loop
			continue
		}

		// some kind of hash value?
		hval := ""

		// if the state data has a value for the prepared state key
		if pval, exists := state.data[states.Prepared{Node: tmout.node, Round: tmout.round, View: tmout.view}]; exists {

			// set whatever the fuck that hash value is to that state.Prepared value
			hval = pval.(string)
		}

		// if the state data is nil (bit of a go-ism here)
		if state.data == nil {
			// set the state data to a new stateKV map, initialized with the view key and the local view number + 1
			state.data = stateKV{skey: v + 1}
		} else { // otherwise, we have some state data
			// set the state data for the view key to the local view number + 1
			state.data[skey] = v + 1
		}

		// put a view change message in the local output messages variable, I suspect this means we've timed out and new to change views.
		out = append(out, messages.ViewChange{
			Hash:   hval,
			Node:   tmout.node,
			Round:  tmout.round,
			Sender: node,
			View:   tmout.view + 1,
		})
	}
	// an index variable set to the length of the output messages
	idx := len(out)

	// a map of processed messages
	processed := map[messages.Message]bool{}

	// process the messages for our current state, and append the output messages from that to the local output messages variable
	out = append(out, graph.processMessages(state, processed, node, node, ntry.block, out[:idx])...)

	// for each of the dependencies of the current entry
	for _, dep := range ntry.deps {
		if log.AtDebug() {
			log.Debug("Processing block dep", fld.BlockID(dep))
		}
		// process the messages for the dependency's state, and append the output messages from that to the local output messages variable
		out = append(out, graph.processMessages(state, processed, dep.Node, node, ntry.block, graph.statess[dep].getOutput())...)
	}

	// save the local output messages variable to the current entry's state
	state.out = out
	graph.statess[ntry.block] = state

}

// interpret a message to see if consensus is reached for a given node, round, and view
func (graph *Graph) processMessage(s *state, sender uint64, receiver uint64, origin BlockID, msg messages.Message) messages.Message {

	// figure out the node and round from the message
	node, round := msg.NodeRound()

	// if there is already a final state for this node and round, return and do nothing
	if _, exists := s.data[states.Final{Node: node, Round: round}]; exists {
		return nil
	}

	// get the view for this node and round
	v := s.getView(node, round)
	if log.AtDebug() {
		log.Debug("Processing message from block", fld.BlockID(origin),
			log.String("message", msg.String()))
	}

	// switch on the type of message
	switch m := msg.(type) {

	// if it's a pre-prepare message
	case messages.PrePrepare:
		// TODO: and valid view!
		// if the view for this node and round is not the same as the message view
		if v != m.View {
			// jump out
			return nil
		}
		// if there is a pre_prepared state for this node and round and view in the state store
		pp := states.PrePrepared{Node: node, Round: round, View: m.View}
		if _, exists := s.data[pp]; exists {
			// jump out. Not sure why we're doing this.
			return nil
		}
		// assert m.view == 0 || (nid, xround, xv, "HNV") in state
		// get the bitset for graph.peerCount (?) and the message. Very unsure what is up here.
		b := s.getBitset(graph.peerCount, m)

		// set the prepare bit for the sender
		b.setPrepare(sender)

		// set the prepare bit for the sender
		b.setPrepare(receiver)

		// if the current state's data is nil set the state data for the pre-prepared state key to the message
		if s.data == nil {
			s.data = stateKV{pp: m}
		} else {
			s.data[pp] = m
		}
		// return a prepare message with the message hash, node, round, receiver, and view
		return messages.Prepare{Hash: m.Hash, Node: node, Round: round, Sender: receiver, View: m.View}

	// if it's a prepare message
	case messages.Prepare:

		// if the local view number is greater than the message view, jump out without doing anything
		if v > m.View {
			return nil
		}

		// if the local view number is less than the message view, jump out without doing anything
		if v < m.View {
			// TODO: should we remember future messages?

			// but make sure to set prepare on the bitset for the message (TODO: check that peerCount, it looks crazy (maybe a bad refactoring))
			b := s.getBitset(graph.peerCount, m.Pre())
			b.setPrepare(m.Sender)
			return nil
		}
		// assert m.view == 0 || (nid, xround, xv, "HNV") in state

		// set prepare on the bitset for the message
		b := s.getBitset(graph.peerCount, m.Pre())
		b.setPrepare(m.Sender)
		if log.AtDebug() {
			log.Debugf("Prepare count == %d", b.prepareCount())
		}

		// if the prepare count is not equal to the commit quorum, jump out
		if b.prepareCount() != graph.quorum2f1 {
			return nil
		}

		// presumably at this point we have a commit quorum.
		// now we need to check: have we already committed for this node and round?  If so, jump out
		if b.hasCommit(receiver) {
			return nil
		}

		// set commit on the bitset for this receiver
		b.setCommit(receiver)

		// set the state data for states.Prepared to the message hash if it's not already there
		p := states.Prepared{Node: node, Round: round, View: m.View}
		if _, exists := s.data[p]; !exists {
			if s.data == nil {
				s.data = stateKV{p: m.Hash}
			} else {
				s.data[p] = m.Hash
			}
		}
		// assert s.data[p] == m.hash

		// return a commit message
		return messages.Commit{Hash: m.Hash, Node: node, Round: round, Sender: receiver, View: m.View}

	case messages.Commit:
		// if the local view we're working on is less than this message's view, jump out
		if v < m.View {
			return nil
		}

		// set commit on the bitset for the message (not sure why we set it above and again here)
		b := s.getBitset(graph.peerCount, m.Pre())
		b.setCommit(m.Sender)
		if log.AtDebug() {
			log.Debugf("Commit count == %d", b.commitCount())
		}

		// if we don't have a commit quorum, jump out
		if b.commitCount() != graph.quorum2f1 {
			return nil
		}

		// if we already have a final value for this node and round, jump out
		nr := nodeRound{node, round}
		if _, exists := s.final[nr]; exists {
			// assert value == m.hash
			return nil
		}

		// set the final value for this node and round to the message hash
		if s.final == nil {
			s.final = map[nodeRound]string{
				nr: m.Hash,
			}
		} else {
			s.final[nr] = m.Hash
		}

		// deliver!
		graph.deliver(node, round, m.Hash)

		// a timeout has occurred and we've received a view change message
	case messages.ViewChange:

		// if the local view number is less than the mesage view, jump out
		if v > m.View {
			return nil
		}

		// maybe "vcs" is "view changes" map? I guess it would make sense that we would need to keep track
		// of which consensus nodes have sent us view change messages, I just wish Go programmers felt that
		// they could afford a few more fucking letters in their variable names. The language has a 70s feel
		// but I've left my platform shoes at home.
		var vcs map[uint64]string
		// TODO: check whether we should store the viewChanged by view number

		// set a state key for the view changed message of the current node, round, and view
		key := states.ViewChanged{Node: node, Round: round, View: v}

		// if there is already a value for the state key, set vcs to that value
		if val, exists := s.data[key]; exists {
			vcs = val.(map[uint64]string)
		} else { // otherwise, set the state store data for the key to the contents of vcs
			vcs = map[uint64]string{}
			if s.data == nil {
				s.data = stateKV{key: vcs}
			} else {
				s.data[key] = vcs
			}
		}
		// store this messages's hash in the view changes map for this message's sender
		vcs[m.Sender] = m.Hash

		// if we don't have a commit quorum of view change messages, jump out
		if len(vcs) != graph.quorum2f1 {
			return nil
		}

		// presumably at this point we DO havea a commit quorum of view change messages

		// Increase the view number
		s.data[states.View{Node: node, Round: round}] = m.View

		// set a local hash variable to the empty string
		var hash string

		// iterate over the values of the view changes map
		for _, hval := range vcs {

			// if the map value is not empty
			if hval != "" {
				// if the hash is not empty and the view changes hash value is equal to the hash
				if hash != "" && hval != hash {
					// log a fatal error
					log.Fatal("Got multiple hashes in a view change",
						fld.NodeID(node), fld.Round(round),
						log.Digest("hash", []byte(hash)), log.Digest("hash.alt", []byte(hval)))
				}
				// otherwise, set the hash to the view changes map hash value
				hash = hval
			}
		}
		// return a new view message
		return messages.NewView{
			Hash: hash, Node: node, Round: round, Sender: receiver, View: m.View,
		}

	case messages.NewView:
		// if the local view number is less than the message view, jump out
		if v > m.View {
			return nil
		}

		// set a state key for the new view message of the current node, round, and view. TODO I think HNV is states.HasNewView.
		key := states.HNV{Node: node, Round: round, View: m.View}

		// if there is already a value for the state key, jump out
		if _, exists := s.data[key]; exists {
			return nil
		}
		if s.data == nil {
			s.data = stateKV{}
		}

		// set the state store data for the key to the message view
		s.data[states.View{Node: node, Round: round}] = m.View
		// TODO: timeout value could overflow uint64 if m.view is over 63 if using `1 << m.view`

		// set a timeout value to the origin's current round plus the state timeout plus 5
		tval := origin.Round + s.timeout + 5 // uint64(10*m.view)

		// append the timeout to the state's timeout values map
		s.timeouts[tval] = append(s.timeouts[tval], timeout{node: node, round: round, view: m.View})

		// set the state store data for the key to true
		s.data[key] = true

		// return a PrePrepare message, which will start consensus over again
		return messages.PrePrepare{Hash: m.Hash, Node: node, Round: round, View: m.View}

	default:
		panic(fmt.Errorf("blockmania: unknown message kind to process: %s", msg.Kind()))

	}

	return nil
}

// eats a list of messages.Message and calls processMessage on each of them
func (graph *Graph) processMessages(s *state, processed map[messages.Message]bool, sender uint64, receiver uint64, origin BlockID, msgs []messages.Message) []messages.Message {
	var out []messages.Message
	for _, msg := range msgs {
		if processed[msg] {
			continue
		}
		resp := graph.processMessage(s, sender, receiver, origin, msg)
		processed[msg] = true
		if resp != nil {
			out = append(out, resp)
		}
	}
	for i := 0; i < len(out); i++ {
		msg := out[i]
		if processed[msg] {
			continue
		}
		resp := graph.processMessage(s, sender, receiver, origin, msg)
		processed[msg] = true
		if resp != nil {
			out = append(out, resp)
		}
	}
	return out
}

func (graph *Graph) run() {
	for {
		select {
		case blockGraph := <-graph.entries:
			entries := make([]*entry, len(blockGraph.Deps))
			graph.mutex.Lock()
			max := blockGraph.Block.Round
			round := graph.round
			for i, dep := range blockGraph.Deps {
				if log.AtDebug() {
					log.Debug("Dep:", fld.BlockID(dep.Block))
				}
				depMax := dep.Block.Round
				rcheck := false
				e := &entry{
					block: dep.Block,
					prev:  dep.Prev,
				}
				if dep.Block.Round != 1 {
					e.deps = make([]BlockID, len(dep.Deps)+1)
					e.deps[0] = dep.Prev
					copy(e.deps[1:], dep.Deps)
					prevMax, exists := graph.max[dep.Prev]
					if !exists {
						rcheck = true
					} else if prevMax > depMax {
						depMax = prevMax
					}
				} else {
					e.deps = dep.Deps
				}
				entries[i] = e
				for _, link := range dep.Deps {
					linkMax, exists := graph.max[link]
					if !exists {
						rcheck = true
					} else if linkMax > depMax {
						depMax = linkMax
					}
				}
				if rcheck && round > depMax {
					depMax = round
				}
				graph.max[dep.Block] = depMax
				if depMax > max {
					max = depMax
				}
			}
			rcheck := false
			if blockGraph.Block.Round != 1 {
				pmax, exists := graph.max[blockGraph.Prev]
				if !exists {
					rcheck = true
				} else if pmax > max {
					max = pmax
				}
			}
			if rcheck && round > max {
				max = round
			}
			graph.max[blockGraph.Block] = max
			graph.blocks = append(graph.blocks, &blockInfo{
				data: blockGraph,
				max:  max,
			})
			graph.mutex.Unlock()
			for _, e := range entries {
				graph.process(e)
			}

			self := &entry{
				block: blockGraph.Block,
				prev:  blockGraph.Prev,
			}
			self.deps = make([]BlockID, len(blockGraph.Deps)+1)
			self.deps[0] = blockGraph.Prev
			for i, dep := range blockGraph.Deps {
				self.deps[i+1] = dep.Block
			}
			graph.process(self)
		case <-graph.ctx.Done():
			return
		}
	}
}

// Add updates the graph and notifies the appropriate controllers.
func (graph *Graph) Add(data *BlockGraph) {
	if log.AtDebug() {
		log.Debug("Adding block to graph", fld.BlockID(data.Block))
	}
	graph.entries <- data
}

// New instantiates a Graph for use by the broadcast/consensus mechanism.
func New(ctx context.Context, cfg *Config, cb func(*Interpreted)) *Graph {
	f := (len(cfg.Peers) - 1) / 3
	g := &Graph{
		blocks:     []*blockInfo{},
		callback:   cb,
		ctx:        ctx,
		entries:    make(chan *BlockGraph, 10000),
		max:        map[BlockID]uint64{},
		peerCount:  len(cfg.Peers),
		peers:      cfg.Peers,
		quorumf1:   f + 1,
		quorum2f:   (2 * f),
		quorum2f1:  (2 * f) + 1,
		resolved:   map[uint64]map[uint64]string{},
		round:      cfg.LastInterpreted + 1,
		self:       cfg.SelfID,
		statess:    map[BlockID]*state{},
		totalNodes: cfg.TotalNodes,
	}
	go g.run()
	return g
}
