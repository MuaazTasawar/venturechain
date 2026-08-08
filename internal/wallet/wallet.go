package wallet

import (
	"fmt"

	"github.com/MuaazTasawar/venturechain/internal/blockchain"
	"github.com/MuaazTasawar/venturechain/pkg/crypto"
)

// Wallet wraps a loaded key with convenience methods for building and
// signing transactions against a known nonce.
type Wallet struct {
	Key *StoredKey
}

// New wraps an already-loaded StoredKey in a Wallet.
func New(key *StoredKey) *Wallet {
	return &Wallet{Key: key}
}

// BuildAndSignTransfer creates a signed TRANSFER transaction from this
// wallet to the given recipient address.
func (w *Wallet) BuildAndSignTransfer(to string, amount int64, nonce int64) (*blockchain.Transaction, error) {
	tx := blockchain.NewTransaction(blockchain.TxTransfer, w.Key.Address, to, amount, "", "", nonce)
	if err := tx.Sign(w.Key.PrivateKey, w.Key.PublicKey); err != nil {
		return nil, fmt.Errorf("failed to sign transfer: %w", err)
	}
	return tx, nil
}

// BuildAndSignLock creates a signed LOCK transaction, locking funds from this
// wallet (the investor) into a contract's escrow, payable to the founder.
func (w *Wallet) BuildAndSignLock(founderAddress, contractID string, amount int64, nonce int64) (*blockchain.Transaction, error) {
	tx := blockchain.NewTransaction(blockchain.TxLock, w.Key.Address, founderAddress, amount, contractID, "", nonce)
	if err := tx.Sign(w.Key.PrivateKey, w.Key.PublicKey); err != nil {
		return nil, fmt.Errorf("failed to sign lock: %w", err)
	}
	return tx, nil
}

// VerifyAddress confirms this wallet's stored address actually matches its
// stored public key - a sanity check after loading a key file from disk.
func (w *Wallet) VerifyAddress() bool {
	expected := crypto.AddressFromPubKeyHex(w.Key.PublicKey)
	return expected == w.Key.Address
}