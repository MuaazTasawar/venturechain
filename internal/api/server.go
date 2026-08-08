package api

import (
    "fmt"
    "net/http"
    "sync/atomic"

    "github.com/MuaazTasawar/venturechain/internal/blockchain"
    "github.com/MuaazTasawar/venturechain/internal/mempool"
    "github.com/MuaazTasawar/venturechain/internal/p2p"
    "github.com/MuaazTasawar/venturechain/internal/storage"
    "github.com/MuaazTasawar/venturechain/internal/wallet"
    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
)

// Server exposes VentureChain's client-facing REST API, used by the
// Venturify Django backend to submit transactions, query balances, and
// drive the escrow contract lifecycle. Runs on a separate port from the
// p2p gossip endpoints (internal/p2p).
type Server struct {
    Chain          *blockchain.Blockchain
    Mempool        *mempool.Mempool
    Peers          *p2p.PeerList
    Contracts      *ContractStore
    TreasuryAddr   string
    EscrowWallet   *wallet.Wallet
    InternalAPIKey string
    opCounter      int64
}

// NewServer wires an already-running chain/mempool/peer set into a REST API
// server instance. store is the same BadgerDB instance the node uses for
// chain persistence - contracts are persisted there too.
func NewServer(chain *blockchain.Blockchain, mp *mempool.Mempool, peers *p2p.PeerList, store *storage.Store, treasuryAddr string, escrowWallet *wallet.Wallet, internalAPIKey string) (*Server, error) {
    contracts, err := NewContractStore(store)
    if err != nil {
        return nil, fmt.Errorf("failed to initialize contract store: %w", err)
    }
    return &Server{
        Chain:          chain,
        Mempool:        mp,
        Peers:          peers,
        Contracts:      contracts,
        TreasuryAddr:   treasuryAddr,
        EscrowWallet:   escrowWallet,
        InternalAPIKey: internalAPIKey,
    }, nil
}

// nextOpNonce returns a monotonically increasing counter used as the nonce
// field on server-signed transactions (MINT, RELEASE_MILESTONE, REFUND,
// ARBITRATE), purely to guarantee unique transaction hashes. These types are
// exempt from real nonce-ordering enforcement at chain level - see
// internal/blockchain/validation.go.
func (s *Server) nextOpNonce() int64 {
    return atomic.AddInt64(&s.opCounter, 1)
}

// submitTx validates a transaction against current chain state, adds it to
// the local mempool, and gossips it to peers. Used by every handler that
// produces a new transaction, whether client-signed or server-signed.
func (s *Server) submitTx(tx *blockchain.Transaction) error {
    if err := blockchain.ValidateTransaction(tx, s.Chain.State); err != nil {
        return err
    }
    s.Mempool.Add(tx)
    go p2p.BroadcastTransaction(s.Peers, tx)
    return nil
}

// Router builds the full client-facing HTTP mux.
func (s *Server) Router() http.Handler {
    r := chi.NewRouter()
    r.Use(middleware.Logger)

    r.Route("/api/wallet", func(r chi.Router) {
        r.Get("/{address}/balance", s.handleGetBalance)
        r.Get("/{address}/nonce", s.handleGetNonce)
    })

    r.Route("/api/chain", func(r chi.Router) {
        r.Get("/height", s.handleChainHeight)
        r.Get("/block/{index}", s.handleGetBlock)
        r.Get("/escrow/{contractID}", s.handleGetOnChainEscrow)
        r.Post("/tx", s.handleSubmitTx)
        r.With(s.requireInternalKey).Post("/mint", s.handleMint)
    })

    r.Route("/api/escrow/contracts", func(r chi.Router) {
        r.Post("/", s.handleCreateContract)
        r.Get("/{contractID}", s.handleGetContract)
        r.Post("/{contractID}/advance", s.handleAdvanceContract)
        r.Get("/{contractID}/funding-tx", s.handleFundingTx)
        r.Post("/{contractID}/confirm-funded", s.handleConfirmFunded)
        r.Post("/{contractID}/milestones/{milestoneID}/submit", s.handleSubmitMilestone)
        r.Post("/{contractID}/milestones/{milestoneID}/dispute", s.handleDisputeMilestone)
        r.With(s.requireInternalKey).Post("/{contractID}/milestones/{milestoneID}/release", s.handleReleaseMilestone)
        r.With(s.requireInternalKey).Post("/{contractID}/arbitrate", s.handleArbitrate)
        r.With(s.requireInternalKey).Post("/{contractID}/refund", s.handleRefund)
    })

    return r
}