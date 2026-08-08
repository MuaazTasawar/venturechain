package mempool

import (
	"sync"

	"github.com/MuaazTasawar/venturechain/internal/blockchain"
)

// Mempool holds validated-but-not-yet-included transactions, waiting to be
// picked up by whichever validator proposes the next block.
type Mempool struct {
	mu  sync.Mutex
	txs map[string]*blockchain.Transaction // keyed by transaction ID
}

// New returns an empty mempool.
func New() *Mempool {
	return &Mempool{
		txs: make(map[string]*blockchain.Transaction),
	}
}

// Add inserts a transaction into the pool. Returns false if a transaction
// with the same ID is already present (duplicate submission).
func (m *Mempool) Add(tx *blockchain.Transaction) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.txs[tx.ID]; exists {
		return false
	}
	m.txs[tx.ID] = tx
	return true
}

// Remove deletes a transaction from the pool, typically after it has been
// included in a committed block.
func (m *Mempool) Remove(txID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.txs, txID)
}

// RemoveBatch deletes multiple transactions at once, called after a block
// is successfully applied to the chain.
func (m *Mempool) RemoveBatch(txs []*blockchain.Transaction) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, tx := range txs {
		delete(m.txs, tx.ID)
	}
}

// Pending returns up to maxCount transactions currently waiting, in no
// guaranteed order. Used by the proposer to fill the next block.
func (m *Mempool) Pending(maxCount int) []*blockchain.Transaction {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*blockchain.Transaction, 0, maxCount)
	for _, tx := range m.txs {
		if len(result) >= maxCount {
			break
		}
		result = append(result, tx)
	}
	return result
}

// Size returns the current number of pending transactions.
func (m *Mempool) Size() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.txs)
}

// Has reports whether a transaction with the given ID is already pending.
func (m *Mempool) Has(txID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.txs[txID]
	return exists
}