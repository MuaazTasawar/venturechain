package blockchain

import (
	"errors"
	"fmt"
	"sync"
)

// EscrowLock tracks funds locked in a contract's escrow, keyed by contract ID.
type EscrowLock struct {
	ContractID string `json:"contract_id"`
	Investor   string `json:"investor"`
	Founder    string `json:"founder"`
	Amount     int64  `json:"amount"`
	Status     string `json:"status"` // LOCKED, FROZEN, RELEASED, REFUNDED
}

// State is the current world state derived from applying every transaction
// in the chain in order.
type State struct {
	mu       sync.RWMutex
	Balances map[string]int64       `json:"balances"`
	Locked   map[string]*EscrowLock `json:"locked"`
	Nonces   map[string]int64       `json:"nonces"`
}

// NewState returns an empty state ready for genesis allocations.
func NewState() *State {
	return &State{
		Balances: make(map[string]int64),
		Locked:   make(map[string]*EscrowLock),
		Nonces:   make(map[string]int64),
	}
}

func (s *State) GetBalance(address string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Balances[address]
}

func (s *State) GetNonce(address string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Nonces[address]
}

func (s *State) GetLock(contractID string) *EscrowLock {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Locked[contractID]
}

// clone returns a deep-enough copy of state for rollback on a rejected block.
func (s *State) clone() *State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	newState := NewState()
	for k, v := range s.Balances {
		newState.Balances[k] = v
	}
	for k, v := range s.Nonces {
		newState.Nonces[k] = v
	}
	for k, v := range s.Locked {
		lockCopy := *v
		newState.Locked[k] = &lockCopy
	}
	return newState
}

// Blockchain holds the ordered chain of blocks and the derived world state.
type Blockchain struct {
	mu     sync.RWMutex
	Blocks []*Block
	State  *State
}

// NewBlockchain creates a chain seeded with a genesis block and applies the
// initial allocations (e.g. the platform treasury balance) directly to state.
func NewBlockchain(initialAllocations map[string]int64) *Blockchain {
	genesis := GenesisBlock()
	state := NewState()
	for addr, amount := range initialAllocations {
		state.Balances[addr] = amount
	}
	return &Blockchain{
		Blocks: []*Block{genesis},
		State:  state,
	}
}

// LatestBlock returns the most recently committed block.
func (bc *Blockchain) LatestBlock() *Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.Blocks[len(bc.Blocks)-1]
}

// AddBlock validates a proposed block against the current chain tip and
// state, applies every transaction in it, and appends it to the chain.
// validatorPubKeyHex is looked up by the caller (consensus layer) from the
// current validator set for the block's ValidatorID.
func (bc *Blockchain) AddBlock(block *Block, validatorPubKeyHex string) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	latest := bc.Blocks[len(bc.Blocks)-1]
	if err := ValidateBlock(block, latest, validatorPubKeyHex); err != nil {
		return fmt.Errorf("block %d rejected: %w", block.Index, err)
	}

	// Apply every transaction. If any fails, the whole block is rejected and
	// no partial state change is kept.
	snapshot := bc.State.clone()
	for _, tx := range block.Transactions {
		if err := ValidateTransaction(tx, bc.State); err != nil {
			bc.State = snapshot
			return fmt.Errorf("block %d rejected: transaction %s invalid: %w", block.Index, tx.ID, err)
		}
		if err := ApplyTransaction(tx, bc.State); err != nil {
			bc.State = snapshot
			return fmt.Errorf("block %d rejected: transaction %s failed to apply: %w", block.Index, tx.ID, err)
		}
	}

	bc.Blocks = append(bc.Blocks, block)
	return nil
}

// ValidateChain walks the full chain from genesis and confirms every block's
// PrevHash and Hash linkage is intact. Used on node startup after loading
// from storage.
func (bc *Blockchain) ValidateChain() error {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	for i := 1; i < len(bc.Blocks); i++ {
		curr := bc.Blocks[i]
		prev := bc.Blocks[i-1]
		if curr.PrevHash != prev.Hash {
			return fmt.Errorf("chain broken at block %d: prev_hash mismatch", curr.Index)
		}
		if curr.calculateHash() != curr.Hash {
			return fmt.Errorf("chain broken at block %d: hash tampering detected", curr.Index)
		}
	}
	return nil
}

var ErrInsufficientBalance = errors.New("insufficient balance")
var ErrInvalidNonce = errors.New("invalid nonce")
var ErrEscrowNotFound = errors.New("escrow lock not found for contract")
var ErrEscrowNotFrozen = errors.New("escrow is not in a frozen state")
var ErrEscrowNotLocked = errors.New("escrow is not in a locked state")