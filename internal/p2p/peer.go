package p2p

import "sync"

// Peer represents another VentureChain node reachable over HTTP.
type Peer struct {
	ID      string `json:"id"`      // validator ID, e.g. "validator-2"
	Address string `json:"address"` // base URL, e.g. "http://192.168.1.20:9002"
}

// PeerList is a thread-safe registry of known peers for this node.
type PeerList struct {
	mu    sync.RWMutex
	peers map[string]*Peer
}

// NewPeerList returns an empty peer registry.
func NewPeerList() *PeerList {
	return &PeerList{peers: make(map[string]*Peer)}
}

// Add registers a peer, overwriting any existing entry with the same ID.
func (pl *PeerList) Add(p *Peer) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.peers[p.ID] = p
}

// Remove deregisters a peer by ID.
func (pl *PeerList) Remove(id string) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	delete(pl.peers, id)
}

// All returns a snapshot slice of every known peer.
func (pl *PeerList) All() []*Peer {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	result := make([]*Peer, 0, len(pl.peers))
	for _, p := range pl.peers {
		result = append(result, p)
	}
	return result
}

// Has reports whether a peer with the given ID is already registered.
func (pl *PeerList) Has(id string) bool {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	_, ok := pl.peers[id]
	return ok
}