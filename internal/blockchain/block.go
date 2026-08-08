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