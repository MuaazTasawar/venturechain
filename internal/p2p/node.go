package p2p

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/MuaazTasawar/venturechain/internal/blockchain"
	"github.com/MuaazTasawar/venturechain/internal/consensus"
	"github.com/MuaazTasawar/venturechain/internal/mempool"
	"github.com/MuaazTasawar/venturechain/internal/storage"
)

// Node ties together the chain, mempool, consensus engine, storage, and
// peer registry into one running VentureChain network participant.
type Node struct {
	Self      *Peer
	Chain     *blockchain.Blockchain
	Mempool   *mempool.Mempool
	Consensus *consensus.Engine
	Store     *storage.Store
	Peers     *PeerList
}

// NewNode wires an already-constructed chain, mempool, consensus engine,
// and store into a p2p-capable node identified by self.
func NewNode(self *Peer, chain *blockchain.Blockchain, mp *mempool.Mempool, eng *consensus.Engine, store *storage.Store) *Node {
	return &Node{
		Self:      self,
		Chain:     chain,
		Mempool:   mp,
		Consensus: eng,
		Store:     store,
		Peers:     NewPeerList(),
	}
}

// Router builds the HTTP mux for node-to-node gossip endpoints. This is kept
// separate from the client-facing REST API (internal/api), which runs on
// its own port and talks to this node's Chain/Mempool directly in-process.
func (n *Node) Router() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/p2p/block", n.handleReceiveBlock)
	mux.HandleFunc("/p2p/tx", n.handleReceiveTx)
	mux.HandleFunc("/p2p/join", n.handleJoin)
	mux.HandleFunc("/p2p/peers", n.handlePeers)
	return mux
}

// handleReceiveBlock accepts a gossiped block from a peer, validates and
// applies it, persists it, then re-broadcasts to propagate the block
// further through the network.
func (n *Node) handleReceiveBlock(w http.ResponseWriter, r *http.Request) {
	var msg BlockMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "invalid block payload", http.StatusBadRequest)
		return
	}
	block := msg.Block
	latest := n.Chain.LatestBlock()

	if block.Index <= latest.Index {
		// Already have this block (or an older one) - ignore silently,
		// this is expected under flooding gossip.
		w.WriteHeader(http.StatusOK)
		return
	}
	if block.Index != latest.Index+1 {
		// Out of order - a full implementation would request the missing
		// range from the sender. For this devnet, reject and let the next
		// gossip round (or manual resync) catch it up.
		http.Error(w, "out of order block, resync required", http.StatusConflict)
		return
	}

	if err := n.Consensus.AcceptBlock(block); err != nil {
		log.Printf("p2p: rejected block %d from network: %v", block.Index, err)
		http.Error(w, "block rejected: "+err.Error(), http.StatusBadRequest)
		return
	}

	if n.Store != nil {
		if err := n.Store.SaveBlock(block); err != nil {
			log.Printf("p2p: failed to persist block %d: %v", block.Index, err)
		}
		if err := n.Store.SaveState(n.Chain.State); err != nil {
			log.Printf("p2p: failed to persist state after block %d: %v", block.Index, err)
		}
	}

	go BroadcastBlock(n.Peers, block)
	w.WriteHeader(http.StatusOK)
}

// handleReceiveTx accepts a gossiped transaction, and if it's new and
// currently valid against local state, adds it to the mempool and
// re-broadcasts it.
func (n *Node) handleReceiveTx(w http.ResponseWriter, r *http.Request) {
	var msg TxMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "invalid tx payload", http.StatusBadRequest)
		return
	}
	tx := msg.Tx

	if n.Mempool.Has(tx.ID) {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := blockchain.ValidateTransaction(tx, n.Chain.State); err != nil {
		http.Error(w, "tx rejected: "+err.Error(), http.StatusBadRequest)
		return
	}
	n.Mempool.Add(tx)
	go BroadcastTransaction(n.Peers, tx)
	w.WriteHeader(http.StatusOK)
}

// handleJoin registers a new peer that announced itself to this node, and
// replies with this node's current known peer list so the joiner can build
// out its own registry in one round trip.
func (n *Node) handleJoin(w http.ResponseWriter, r *http.Request) {
	var peer Peer
	if err := json.NewDecoder(r.Body).Decode(&peer); err != nil {
		http.Error(w, "invalid peer payload", http.StatusBadRequest)
		return
	}
	n.Peers.Add(&peer)
	log.Printf("p2p: peer joined: %s (%s)", peer.ID, peer.Address)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(n.Peers.All())
}

// handlePeers returns the current known peer list, useful for debugging and
// for a joining node to merge registries.
func (n *Node) handlePeers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(n.Peers.All())
}

// Start begins listening for peer HTTP traffic on the given address, e.g.
// ":9001". This runs the gossip endpoints, not the client-facing API.
func (n *Node) Start(addr string) error {
	log.Printf("p2p: node %s listening on %s", n.Self.ID, addr)
	return http.ListenAndServe(addr, n.Router())
}

// RunProductionLoop ticks at the configured block interval and, whenever
// it's this node's turn under PoA rotation, proposes a block from the
// current mempool, applies it locally, persists it, and gossips it out.
func (n *Node) RunProductionLoop() {
	ticker := time.NewTicker(n.Consensus.BlockInterval())
	defer ticker.Stop()

	for range ticker.C {
		latest := n.Chain.LatestBlock()
		nextHeight := latest.Index + 1

		if !n.Consensus.IsMyTurn(nextHeight) {
			continue
		}

		block, err := n.Consensus.ProposeBlock()
		if err != nil {
			log.Printf("p2p: propose failed at height %d: %v", nextHeight, err)
			continue
		}

		if err := n.Consensus.AcceptBlock(block); err != nil {
			log.Printf("p2p: self-accept failed for proposed block %d: %v", block.Index, err)
			continue
		}

		if n.Store != nil {
			if err := n.Store.SaveBlock(block); err != nil {
				log.Printf("p2p: failed to persist proposed block %d: %v", block.Index, err)
			}
			if err := n.Store.SaveState(n.Chain.State); err != nil {
				log.Printf("p2p: failed to persist state after proposing block %d: %v", block.Index, err)
			}
		}

		log.Printf("p2p: produced block %d with %d tx(s)", block.Index, len(block.Transactions))
		BroadcastBlock(n.Peers, block)
	}
}