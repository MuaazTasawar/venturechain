package consensus

import (
	"fmt"
	"time"

	"github.com/MuaazTasawar/venturechain/internal/blockchain"
	"github.com/MuaazTasawar/venturechain/internal/mempool"
)

// Engine drives proof-of-authority block production for a single local
// validator identity, rotating proposer duty across the configured
// ValidatorSet.
type Engine struct {
	Validators  *ValidatorSet
	LocalID     string // this node's validator ID, e.g. "validator-1"
	LocalPriv   string // this node's hex-encoded private key
	Chain       *blockchain.Blockchain
	Mempool     *mempool.Mempool
	MaxTxsPerBlock int
}

// NewEngine wires together everything the PoA loop needs for one node.
func NewEngine(vs *ValidatorSet, localID, localPriv string, chain *blockchain.Blockchain, mp *mempool.Mempool) *Engine {
	return &Engine{
		Validators:     vs,
		LocalID:        localID,
		LocalPriv:      localPriv,
		Chain:          chain,
		Mempool:        mp,
		MaxTxsPerBlock: 200,
	}
}

// IsMyTurn reports whether this node is the proposer for the given height.
func (e *Engine) IsMyTurn(height int64) bool {
	return e.Validators.ProposerForHeight(height).ID == e.LocalID
}

// ProposeBlock builds, signs, and returns a new block for the given height
// if this node is the current proposer, using the current mempool contents.
// It does NOT append the block to the chain - the caller (node/p2p layer)
// is responsible for broadcasting it and calling AddBlock once accepted.
func (e *Engine) ProposeBlock() (*blockchain.Block, error) {
	latest := e.Chain.LatestBlock()
	nextHeight := latest.Index + 1

	if !e.IsMyTurn(nextHeight) {
		return nil, fmt.Errorf("not this node's turn to propose block %d", nextHeight)
	}

	pending := e.Mempool.Pending(e.MaxTxsPerBlock)
	block := blockchain.NewBlock(nextHeight, latest.Hash, pending, e.LocalID)

	if err := block.Sign(e.LocalPriv); err != nil {
		return nil, fmt.Errorf("failed to sign proposed block: %w", err)
	}
	return block, nil
}

// AcceptBlock validates and commits a block received from the network
// (proposed by another validator, or the local one). On success, the
// contained transactions are removed from the local mempool.
func (e *Engine) AcceptBlock(block *blockchain.Block) error {
	pubKey, err := e.Validators.PublicKeyFor(block.ValidatorID)
	if err != nil {
		return fmt.Errorf("block from unknown validator: %w", err)
	}

	expectedProposer := e.Validators.ProposerForHeight(block.Index)
	if expectedProposer.ID != block.ValidatorID {
		return fmt.Errorf("block %d proposed by %s, expected proposer %s", block.Index, block.ValidatorID, expectedProposer.ID)
	}

	if err := e.Chain.AddBlock(block, pubKey); err != nil {
		return err
	}

	e.Mempool.RemoveBatch(block.Transactions)
	return nil
}

// BlockInterval returns the configured block time as a time.Duration, used
// by the node's production loop to schedule proposal attempts.
func (e *Engine) BlockInterval() time.Duration {
	return time.Duration(e.Validators.BlockTimeSeconds) * time.Second
}