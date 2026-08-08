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