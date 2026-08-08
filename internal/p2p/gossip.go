package p2p

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/MuaazTasawar/venturechain/internal/blockchain"
)

// BlockMessage is the wire format for gossiping a newly committed block.
type BlockMessage struct {
	Block *blockchain.Block `json:"block"`
}

// TxMessage is the wire format for gossiping a pending transaction.
type TxMessage struct {
	Tx *blockchain.Transaction `json:"tx"`
}

var gossipClient = &http.Client{Timeout: 5 * time.Second}

// BroadcastBlock sends a newly accepted block to every known peer. Failures
// to individual peers are logged, not fatal - gossip is best-effort and a
// peer that missed a block can catch up by re-requesting height on its own.
func BroadcastBlock(peers *PeerList, block *blockchain.Block) {
	msg := BlockMessage{Block: block}
	body, err := json.Marshal(msg)
	if err != nil {
		log.Printf("gossip: failed to marshal block %d: %v", block.Index, err)
		return
	}
	for _, peer := range peers.All() {
		go postJSON(peer.Address+"/p2p/block", body)
	}
}

// BroadcastTransaction sends a newly accepted transaction to every known
// peer so it propagates through the network's mempools.
func BroadcastTransaction(peers *PeerList, tx *blockchain.Transaction) {
	msg := TxMessage{Tx: tx}
	body, err := json.Marshal(msg)
	if err != nil {
		log.Printf("gossip: failed to marshal tx %s: %v", tx.ID, err)
		return
	}
	for _, peer := range peers.All() {
		go postJSON(peer.Address+"/p2p/tx", body)
	}
}

func postJSON(url string, body []byte) {
	resp, err := gossipClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("gossip: failed to reach %s: %v", url, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("gossip: %s rejected message with status %d", url, resp.StatusCode)
	}
}

// AnnounceSelf registers this node with a bootstrap peer by calling its
// /p2p/join endpoint, used once at startup to enter an existing network.
func AnnounceSelf(bootstrapAddress string, self *Peer) error {
	body, err := json.Marshal(self)
	if err != nil {
		return err
	}
	resp, err := gossipClient.Post(bootstrapAddress+"/p2p/join", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to announce to bootstrap %s: %w", bootstrapAddress, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("bootstrap %s rejected join with status %d", bootstrapAddress, resp.StatusCode)
	}
	return nil
}