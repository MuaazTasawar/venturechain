# ── pkg/crypto/signing.go ──
New-Item -ItemType Directory -Path pkg\crypto -Force | Out-Null
$content = @'
package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

// KeyPair holds a generated wallet identity.
type KeyPair struct {
	PrivateKeyHex string
	PublicKeyHex  string
	Address       string
}

// GenerateKeyPair creates a new secp256k1 private/public key pair and derives
// a VentureChain address from the public key.
func GenerateKeyPair() (*KeyPair, error) {
	priv, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return nil, err
	}
	pub := priv.PubKey()

	privHex := hex.EncodeToString(priv.Serialize())
	pubHex := hex.EncodeToString(pub.SerializeCompressed())
	address := AddressFromPubKeyHex(pubHex)

	return &KeyPair{
		PrivateKeyHex: privHex,
		PublicKeyHex:  pubHex,
		Address:       address,
	}, nil
}

// AddressFromPubKeyHex derives a human-readable VentureChain address from a
// hex-encoded compressed public key: VTC + first 40 hex chars of sha256(pubkey).
func AddressFromPubKeyHex(pubKeyHex string) string {
	pubBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(pubBytes)
	return "VTC" + hex.EncodeToString(hash[:])[:40]
}

// Sign signs a message hash with a hex-encoded private key and returns a
// hex-encoded DER signature.
func Sign(privKeyHex string, hash []byte) (string, error) {
	privBytes, err := hex.DecodeString(privKeyHex)
	if err != nil {
		return "", err
	}
	if len(privBytes) != 32 {
		return "", errors.New("invalid private key length")
	}
	priv := secp256k1.PrivKeyFromBytes(privBytes)
	sig := ecdsa.Sign(priv, hash)
	return hex.EncodeToString(sig.Serialize()), nil
}

// Verify checks a hex-encoded DER signature against a message hash and a
// hex-encoded compressed public key.
func Verify(pubKeyHex string, hash []byte, sigHex string) (bool, error) {
	pubBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return false, err
	}
	pub, err := secp256k1.ParsePubKey(pubBytes)
	if err != nil {
		return false, err
	}
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false, err
	}
	sig, err := ecdsa.ParseDERSignature(sigBytes)
	if err != nil {
		return false, err
	}
	return sig.Verify(hash, pub), nil
}

// HashBytes returns the sha256 hash of arbitrary data.
func HashBytes(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}
'@
[System.IO.File]::WriteAllText("$PWD\pkg\crypto\signing.go", $content)

# ── internal/blockchain/transaction.go ──
New-Item -ItemType Directory -Path internal\blockchain -Force | Out-Null
$content = @'
package blockchain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/MuaazTasawar/venturechain/pkg/crypto"
)

// Transaction types supported natively by the chain. Escrow logic is built
// directly into these types instead of a smart contract VM.
const (
	TxMint             = "MINT"              // treasury mints VTC on confirmed fiat deposit
	TxTransfer         = "TRANSFER"          // plain wallet-to-wallet transfer
	TxLock             = "LOCK"              // investor locks funds into a contract's escrow
	TxReleaseMilestone = "RELEASE_MILESTONE" // escrow releases funds to founder on milestone approval
	TxRefund           = "REFUND"            // escrow refunds locked funds back to investor
	TxFreezeDispute    = "FREEZE_DISPUTE"    // escrow is frozen pending arbitration
	TxArbitrate        = "ARBITRATE"         // arbitrator resolves a frozen escrow, splitting funds
)

// Transaction is the atomic unit of state change on VentureChain.
type Transaction struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	From        string `json:"from"`
	To          string `json:"to"`
	Amount      int64  `json:"amount"`
	ContractID  string `json:"contract_id,omitempty"`
	MilestoneID string `json:"milestone_id,omitempty"`
	Nonce       int64  `json:"nonce"`
	Timestamp   int64  `json:"timestamp"`
	PublicKey   string `json:"public_key"`
	Signature   string `json:"signature"`
}

// NewTransaction builds an unsigned transaction with a generated ID and the
// current timestamp. Call Sign() before broadcasting.
func NewTransaction(txType, from, to string, amount int64, contractID, milestoneID string, nonce int64) *Transaction {
	tx := &Transaction{
		Type:        txType,
		From:        from,
		To:          to,
		Amount:      amount,
		ContractID:  contractID,
		MilestoneID: milestoneID,
		Nonce:       nonce,
		Timestamp:   time.Now().Unix(),
	}
	tx.ID = tx.Hash()
	return tx
}

// canonicalBytes returns a deterministic byte representation of the
// transaction, excluding the signature, used for both hashing and signing.
func (tx *Transaction) canonicalBytes() []byte {
	payload := struct {
		Type        string `json:"type"`
		From        string `json:"from"`
		To          string `json:"to"`
		Amount      int64  `json:"amount"`
		ContractID  string `json:"contract_id,omitempty"`
		MilestoneID string `json:"milestone_id,omitempty"`
		Nonce       int64  `json:"nonce"`
		Timestamp   int64  `json:"timestamp"`
	}{
		Type:        tx.Type,
		From:        tx.From,
		To:          tx.To,
		Amount:      tx.Amount,
		ContractID:  tx.ContractID,
		MilestoneID: tx.MilestoneID,
		Nonce:       tx.Nonce,
		Timestamp:   tx.Timestamp,
	}
	b, _ := json.Marshal(payload)
	return b
}

// Hash returns the hex-encoded sha256 hash of the transaction's canonical
// fields. This also serves as the transaction ID.
func (tx *Transaction) Hash() string {
	sum := sha256.Sum256(tx.canonicalBytes())
	return hex.EncodeToString(sum[:])
}

// Sign signs the transaction hash with the sender's hex-encoded private key
// and populates PublicKey and Signature.
func (tx *Transaction) Sign(privKeyHex, pubKeyHex string) error {
	hashBytes, err := hex.DecodeString(tx.Hash())
	if err != nil {
		return err
	}
	sig, err := crypto.Sign(privKeyHex, hashBytes)
	if err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}
	tx.PublicKey = pubKeyHex
	tx.Signature = sig
	return nil
}

// VerifySignature checks that the transaction's signature is valid for its
// hash and the attached public key, and that the public key matches From.
// MINT transactions are exempt since they originate from validator consensus,
// not a signed wallet action - authorized separately by the API layer.
func (tx *Transaction) VerifySignature() (bool, error) {
	if tx.Type == TxMint {
		return true, nil
	}
	if tx.Signature == "" || tx.PublicKey == "" {
		return false, errors.New("missing signature or public key")
	}
	expectedAddress := crypto.AddressFromPubKeyHex(tx.PublicKey)
	if expectedAddress != tx.From {
		return false, errors.New("public key does not match sender address")
	}
	hashBytes, err := hex.DecodeString(tx.Hash())
	if err != nil {
		return false, err
	}
	return crypto.Verify(tx.PublicKey, hashBytes, tx.Signature)
}
'@
[System.IO.File]::WriteAllText("$PWD\internal\blockchain\transaction.go", $content)

# ── internal/blockchain/block.go ──
$content = @'
package blockchain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/MuaazTasawar/venturechain/pkg/crypto"
)

// Block is a batch of validated transactions produced by a validator under
// proof-of-authority consensus.
type Block struct {
	Index        int64          `json:"index"`
	Timestamp    int64          `json:"timestamp"`
	PrevHash     string         `json:"prev_hash"`
	Hash         string         `json:"hash"`
	MerkleRoot   string         `json:"merkle_root"`
	Transactions []*Transaction `json:"transactions"`
	ValidatorID  string         `json:"validator_id"`
	Signature    string         `json:"signature"`
}

// NewBlock builds an unsigned block from a set of transactions. Call Sign()
// afterwards with the proposing validator's key.
func NewBlock(index int64, prevHash string, txs []*Transaction, validatorID string) *Block {
	b := &Block{
		Index:        index,
		Timestamp:    time.Now().Unix(),
		PrevHash:     prevHash,
		Transactions: txs,
		ValidatorID:  validatorID,
	}
	b.MerkleRoot = b.calculateMerkleRoot()
	b.Hash = b.calculateHash()
	return b
}

// calculateMerkleRoot computes a simple sequential hash over all transaction
// hashes in the block, giving a single fingerprint for the transaction set.
func (b *Block) calculateMerkleRoot() string {
	if len(b.Transactions) == 0 {
		empty := sha256.Sum256([]byte("empty"))
		return hex.EncodeToString(empty[:])
	}
	combined := ""
	for _, tx := range b.Transactions {
		combined += tx.Hash()
	}
	sum := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(sum[:])
}

// calculateHash returns the hex-encoded sha256 hash of the block header
// fields (everything except the header's own Hash and Signature).
func (b *Block) calculateHash() string {
	header := struct {
		Index       int64  `json:"index"`
		Timestamp   int64  `json:"timestamp"`
		PrevHash    string `json:"prev_hash"`
		MerkleRoot  string `json:"merkle_root"`
		ValidatorID string `json:"validator_id"`
	}{
		Index:       b.Index,
		Timestamp:   b.Timestamp,
		PrevHash:    b.PrevHash,
		MerkleRoot:  b.MerkleRoot,
		ValidatorID: b.ValidatorID,
	}
	data, _ := json.Marshal(header)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Sign signs the block hash with the proposing validator's hex-encoded
// private key.
func (b *Block) Sign(privKeyHex string) error {
	hashBytes, err := hex.DecodeString(b.Hash)
	if err != nil {
		return err
	}
	sig, err := crypto.Sign(privKeyHex, hashBytes)
	if err != nil {
		return err
	}
	b.Signature = sig
	return nil
}

// VerifySignature checks the block's signature against the given validator
// public key (looked up by the caller from the validator set).
func (b *Block) VerifySignature(validatorPubKeyHex string) (bool, error) {
	hashBytes, err := hex.DecodeString(b.Hash)
	if err != nil {
		return false, err
	}
	return crypto.Verify(validatorPubKeyHex, hashBytes, b.Signature)
}

// GenesisBlock creates the first block of the chain. It carries no
// transactions of its own - initial allocations are applied directly to
// state when the chain is initialized (see blockchain.go).
func GenesisBlock() *Block {
	b := &Block{
		Index:        0,
		Timestamp:    time.Now().Unix(),
		PrevHash:     "0000000000000000000000000000000000000000000000000000000000000000",
		Transactions: []*Transaction{},
		ValidatorID:  "genesis",
	}
	b.MerkleRoot = b.calculateMerkleRoot()
	b.Hash = b.calculateHash()
	return b
}
'@
[System.IO.File]::WriteAllText("$PWD\internal\blockchain\block.go", $content)

# ── internal/blockchain/blockchain.go ──
$content = @'
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
'@
[System.IO.File]::WriteAllText("$PWD\internal\blockchain\blockchain.go", $content)

# ── internal/blockchain/validation.go ──
$content = @'
package blockchain

import (
	"fmt"
)

// ValidateBlock performs structural and authority checks on a proposed block
// before its transactions are ever applied to state.
func ValidateBlock(block *Block, prevBlock *Block, validatorPubKeyHex string) error {
	if block.Index != prevBlock.Index+1 {
		return fmt.Errorf("expected index %d, got %d", prevBlock.Index+1, block.Index)
	}
	if block.PrevHash != prevBlock.Hash {
		return fmt.Errorf("prev_hash does not match latest block hash")
	}
	if block.calculateHash() != block.Hash {
		return fmt.Errorf("block hash does not match computed hash")
	}
	if block.calculateMerkleRoot() != block.MerkleRoot {
		return fmt.Errorf("merkle root does not match transaction set")
	}
	valid, err := block.VerifySignature(validatorPubKeyHex)
	if err != nil {
		return fmt.Errorf("signature verification error: %w", err)
	}
	if !valid {
		return fmt.Errorf("block signature invalid for validator %s", block.ValidatorID)
	}
	return nil
}

// ValidateTransaction checks a transaction's signature, nonce ordering, and
// type-specific escrow business rules against the current state, without
// mutating it. Call ApplyTransaction afterwards to commit the effect.
func ValidateTransaction(tx *Transaction, state *State) error {
	valid, err := tx.VerifySignature()
	if err != nil {
		return fmt.Errorf("signature check failed: %w", err)
	}
	if !valid {
		return fmt.Errorf("invalid signature")
	}

	if tx.Amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	switch tx.Type {
	case TxMint:
		// Only the platform treasury may mint - enforced by the API layer
		// checking the caller's role before a MINT tx ever reaches the
		// mempool. At chain level we just require a valid recipient.
		if tx.To == "" {
			return fmt.Errorf("mint requires a recipient")
		}
		return nil

	case TxTransfer:
		return checkSenderFunds(tx, state)

	case TxLock:
		if tx.ContractID == "" {
			return fmt.Errorf("lock requires a contract_id")
		}
		if existing := state.GetLock(tx.ContractID); existing != nil {
			return fmt.Errorf("contract %s already has an active escrow lock", tx.ContractID)
		}
		return checkSenderFunds(tx, state)

	case TxReleaseMilestone:
		lock := state.GetLock(tx.ContractID)
		if lock == nil {
			return ErrEscrowNotFound
		}
		if lock.Status != "LOCKED" {
			return ErrEscrowNotLocked
		}
		if tx.Amount > lock.Amount {
			return fmt.Errorf("release amount %d exceeds remaining escrow %d", tx.Amount, lock.Amount)
		}
		if tx.To != lock.Founder {
			return fmt.Errorf("milestone release must pay the founder on record")
		}
		return nil

	case TxRefund:
		lock := state.GetLock(tx.ContractID)
		if lock == nil {
			return ErrEscrowNotFound
		}
		if lock.Status != "LOCKED" && lock.Status != "FROZEN" {
			return fmt.Errorf("escrow must be locked or frozen to refund")
		}
		if tx.To != lock.Investor {
			return fmt.Errorf("refund must pay the investor on record")
		}
		return nil

	case TxFreezeDispute:
		lock := state.GetLock(tx.ContractID)
		if lock == nil {
			return ErrEscrowNotFound
		}
		if lock.Status != "LOCKED" {
			return ErrEscrowNotLocked
		}
		return nil

	case TxArbitrate:
		lock := state.GetLock(tx.ContractID)
		if lock == nil {
			return ErrEscrowNotFound
		}
		if lock.Status != "FROZEN" {
			return ErrEscrowNotFrozen
		}
		if tx.Amount > lock.Amount {
			return fmt.Errorf("arbitrated amount %d exceeds remaining escrow %d", tx.Amount, lock.Amount)
		}
		if tx.To != lock.Founder && tx.To != lock.Investor {
			return fmt.Errorf("arbitration must pay either the founder or investor on record")
		}
		return nil

	default:
		return fmt.Errorf("unknown transaction type: %s", tx.Type)
	}
}

func checkSenderFunds(tx *Transaction, state *State) error {
	balance := state.GetBalance(tx.From)
	if balance < tx.Amount {
		return ErrInsufficientBalance
	}
	expectedNonce := state.GetNonce(tx.From)
	if tx.Nonce != expectedNonce {
		return fmt.Errorf("%w: expected %d, got %d", ErrInvalidNonce, expectedNonce, tx.Nonce)
	}
	return nil
}

// ApplyTransaction mutates state according to a transaction that has already
// passed ValidateTransaction. It must never be called without a prior
// successful validation.
func ApplyTransaction(tx *Transaction, state *State) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	switch tx.Type {
	case TxMint:
		state.Balances[tx.To] += tx.Amount

	case TxTransfer:
		state.Balances[tx.From] -= tx.Amount
		state.Balances[tx.To] += tx.Amount
		state.Nonces[tx.From]++

	case TxLock:
		state.Balances[tx.From] -= tx.Amount
		state.Locked[tx.ContractID] = &EscrowLock{
			ContractID: tx.ContractID,
			Investor:   tx.From,
			Founder:    tx.To,
			Amount:     tx.Amount,
			Status:     "LOCKED",
		}
		state.Nonces[tx.From]++

	case TxReleaseMilestone:
		lock := state.Locked[tx.ContractID]
		lock.Amount -= tx.Amount
		state.Balances[tx.To] += tx.Amount
		if lock.Amount == 0 {
			lock.Status = "RELEASED"
		}

	case TxRefund:
		lock := state.Locked[tx.ContractID]
		refundAmount := lock.Amount
		state.Balances[tx.To] += refundAmount
		lock.Amount = 0
		lock.Status = "REFUNDED"

	case TxFreezeDispute:
		lock := state.Locked[tx.ContractID]
		lock.Status = "FROZEN"

	case TxArbitrate:
		lock := state.Locked[tx.ContractID]
		lock.Amount -= tx.Amount
		state.Balances[tx.To] += tx.Amount
		if lock.Amount == 0 {
			lock.Status = "RELEASED"
		} else {
			lock.Status = "LOCKED"
		}

	default:
		return fmt.Errorf("unknown transaction type: %s", tx.Type)
	}
	return nil
}
'@
[System.IO.File]::WriteAllText("$PWD\internal\blockchain\validation.go", $content)
