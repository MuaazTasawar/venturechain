package storage

import (
    "encoding/json"
    "fmt"

    "github.com/MuaazTasawar/venturechain/internal/blockchain"
    badger "github.com/dgraph-io/badger/v4"
)

// Key prefixes used to namespace different record types within the single
// Badger keyspace.
const (
    prefixBlock    = "block:"    // block:<index> -> Block JSON
    prefixMeta     = "meta:"     // meta:height -> latest block index
    prefixBalance  = "balance:"  // balance:<address> -> int64 balance
    prefixNonce    = "nonce:"    // nonce:<address> -> int64 nonce
    prefixLock     = "lock:"     // lock:<contract_id> -> EscrowLock JSON
    prefixContract = "contract:" // contract:<contract_id> -> escrow.Contract JSON
)

// Store persists blocks and world state to disk via BadgerDB so a node can
// restart without replaying the entire chain from genesis over the network.
type Store struct {
    db *badger.DB
}

// Open opens (or creates) a Badger database at the given directory path.
func Open(path string) (*Store, error) {
    opts := badger.DefaultOptions(path).WithLogger(nil)
    db, err := badger.Open(opts)
    if err != nil {
        return nil, fmt.Errorf("failed to open badger store at %s: %w", path, err)
    }
    return &Store{db: db}, nil
}

// Close releases the underlying database file handles.
func (s *Store) Close() error {
    return s.db.Close()
}

// SaveBlock persists a single block and updates the stored chain height.
func (s *Store) SaveBlock(block *blockchain.Block) error {
    data, err := json.Marshal(block)
    if err != nil {
        return err
    }
    return s.db.Update(func(txn *badger.Txn) error {
        key := []byte(fmt.Sprintf("%s%d", prefixBlock, block.Index))
        if err := txn.Set(key, data); err != nil {
            return err
        }
        heightBytes := []byte(fmt.Sprintf("%d", block.Index))
        return txn.Set([]byte(prefixMeta+"height"), heightBytes)
    })
}

// LoadBlock retrieves a single block by index.
func (s *Store) LoadBlock(index int64) (*blockchain.Block, error) {
    var block blockchain.Block
    err := s.db.View(func(txn *badger.Txn) error {
        key := []byte(fmt.Sprintf("%s%d", prefixBlock, index))
        item, err := txn.Get(key)
        if err != nil {
            return err
        }
        return item.Value(func(val []byte) error {
            return json.Unmarshal(val, &block)
        })
    })
    if err != nil {
        return nil, err
    }
    return &block, nil
}

// LoadHeight returns the index of the latest persisted block, or -1 if the
// store is empty (fresh node, needs genesis).
func (s *Store) LoadHeight() (int64, error) {
    var height int64 = -1
    err := s.db.View(func(txn *badger.Txn) error {
        item, err := txn.Get([]byte(prefixMeta + "height"))
        if err == badger.ErrKeyNotFound {
            return nil
        }
        if err != nil {
            return err
        }
        return item.Value(func(val []byte) error {
            _, err := fmt.Sscanf(string(val), "%d", &height)
            return err
        })
    })
    return height, err
}

// LoadAllBlocks reconstructs the full block list from storage, in order,
// for chain initialization on node startup.
func (s *Store) LoadAllBlocks() ([]*blockchain.Block, error) {
    height, err := s.LoadHeight()
    if err != nil {
        return nil, err
    }
    if height < 0 {
        return nil, nil // empty store, caller should seed genesis
    }
    blocks := make([]*blockchain.Block, 0, height+1)
    for i := int64(0); i <= height; i++ {
        b, err := s.LoadBlock(i)
        if err != nil {
            return nil, fmt.Errorf("failed to load block %d: %w", i, err)
        }
        blocks = append(blocks, b)
    }
    return blocks, nil
}

// SaveState persists the full world state (balances, nonces, escrow locks)
// as a snapshot. Called after each block is applied.
func (s *Store) SaveState(state *blockchain.State) error {
    return s.db.Update(func(txn *badger.Txn) error {
        for addr, balance := range state.Balances {
            key := []byte(prefixBalance + addr)
            val := []byte(fmt.Sprintf("%d", balance))
            if err := txn.Set(key, val); err != nil {
                return err
            }
        }
        for addr, nonce := range state.Nonces {
            key := []byte(prefixNonce + addr)
            val := []byte(fmt.Sprintf("%d", nonce))
            if err := txn.Set(key, val); err != nil {
                return err
            }
        }
        for contractID, lock := range state.Locked {
            data, err := json.Marshal(lock)
            if err != nil {
                return err
            }
            key := []byte(prefixLock + contractID)
            if err := txn.Set(key, data); err != nil {
                return err
            }
        }
        return nil
    })
}

// LoadState reconstructs the full world state from storage. Returns an empty
// state (never nil) if nothing has been persisted yet.
func (s *Store) LoadState() (*blockchain.State, error) {
    state := blockchain.NewState()
    err := s.db.View(func(txn *badger.Txn) error {
        opts := badger.DefaultIteratorOptions
        it := txn.NewIterator(opts)
        defer it.Close()

        for it.Rewind(); it.Valid(); it.Next() {
            item := it.Item()
            key := string(item.Key())

            switch {
            case len(key) > len(prefixBalance) && key[:len(prefixBalance)] == prefixBalance:
                addr := key[len(prefixBalance):]
                err := item.Value(func(val []byte) error {
                    var bal int64
                    _, err := fmt.Sscanf(string(val), "%d", &bal)
                    if err != nil {
                        return err
                    }
                    state.Balances[addr] = bal
                    return nil
                })
                if err != nil {
                    return err
                }

            case len(key) > len(prefixNonce) && key[:len(prefixNonce)] == prefixNonce:
                addr := key[len(prefixNonce):]
                err := item.Value(func(val []byte) error {
                    var nonce int64
                    _, err := fmt.Sscanf(string(val), "%d", &nonce)
                    if err != nil {
                        return err
                    }
                    state.Nonces[addr] = nonce
                    return nil
                })
                if err != nil {
                    return err
                }

            case len(key) > len(prefixLock) && key[:len(prefixLock)] == prefixLock:
                err := item.Value(func(val []byte) error {
                    var lock blockchain.EscrowLock
                    if err := json.Unmarshal(val, &lock); err != nil {
                        return err
                    }
                    state.Locked[lock.ContractID] = &lock
                    return nil
                })
                if err != nil {
                    return err
                }
            }
        }
        return nil
    })
    if err != nil {
        return nil, err
    }
    return state, nil
}

// SaveContract persists a single escrow contract (lifecycle state,
// milestones) as JSON, keyed by contract ID. Called on every contract
// mutation so a node restart doesn't lose in-flight contract state.
func (s *Store) SaveContract(contractID string, data []byte) error {
    return s.db.Update(func(txn *badger.Txn) error {
        key := []byte(prefixContract + contractID)
        return txn.Set(key, data)
    })
}

// LoadAllContracts returns the raw JSON bytes of every persisted contract,
// keyed by contract ID, for reconstruction on node startup. Returns raw
// bytes rather than a parsed type to avoid storage depending on the escrow
// package.
func (s *Store) LoadAllContracts() (map[string][]byte, error) {
    result := make(map[string][]byte)
    err := s.db.View(func(txn *badger.Txn) error {
        opts := badger.DefaultIteratorOptions
        opts.Prefix = []byte(prefixContract)
        it := txn.NewIterator(opts)
        defer it.Close()

        for it.Rewind(); it.Valid(); it.Next() {
            item := it.Item()
            key := string(item.Key())
            contractID := key[len(prefixContract):]
            err := item.Value(func(val []byte) error {
                data := make([]byte, len(val))
                copy(data, val)
                result[contractID] = data
                return nil
            })
            if err != nil {
                return err
            }
        }
        return nil
    })
    if err != nil {
        return nil, err
    }
    return result, nil
}