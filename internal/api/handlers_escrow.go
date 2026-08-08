package api

import (
    "encoding/json"
    "fmt"
    "net/http"
    "sync"

    "github.com/MuaazTasawar/venturechain/internal/escrow"
    "github.com/MuaazTasawar/venturechain/internal/storage"
    "github.com/go-chi/chi/v5"
)

// ContractStore is a BadgerDB-backed registry of escrow contracts, keyed by
// ID. Venturify's Django backend remains the system of record for contract
// metadata (parties, terms text, etc); this store lets the chain node track
// lifecycle state and milestone status locally, surviving node restarts by
// persisting alongside chain state in the same BadgerDB instance.
type ContractStore struct {
    mu        sync.RWMutex
    contracts map[string]*escrow.Contract
    store     *storage.Store
}

// NewContractStore builds a ContractStore backed by the given BadgerDB
// instance and loads any contracts persisted from a previous run.
func NewContractStore(store *storage.Store) (*ContractStore, error) {
    cs := &ContractStore{
        contracts: make(map[string]*escrow.Contract),
        store:     store,
    }
    raw, err := store.LoadAllContracts()
    if err != nil {
        return nil, fmt.Errorf("failed to load persisted contracts: %w", err)
    }
    for id, data := range raw {
        var c escrow.Contract
        if err := json.Unmarshal(data, &c); err != nil {
            return nil, fmt.Errorf("failed to parse persisted contract %s: %w", id, err)
        }
        cs.contracts[id] = &c
    }
    return cs, nil
}

// Save writes the contract to the in-memory map and persists it to
// BadgerDB so it survives a node restart. Returns an error if persistence
// fails - callers should treat that as the write not having fully
// succeeded, even though the in-memory copy is updated.
func (cs *ContractStore) Save(c *escrow.Contract) error {
    cs.mu.Lock()
    cs.contracts[c.ID] = c
    cs.mu.Unlock()

    data, err := json.Marshal(c)
    if err != nil {
        return fmt.Errorf("failed to marshal contract %s: %w", c.ID, err)
    }
    if err := cs.store.SaveContract(c.ID, data); err != nil {
        return fmt.Errorf("failed to persist contract %s: %w", c.ID, err)
    }
    return nil
}

func (cs *ContractStore) Get(id string) (*escrow.Contract, bool) {
    cs.mu.RLock()
    defer cs.mu.RUnlock()
    c, ok := cs.contracts[id]
    return c, ok
}

type createContractRequest struct {
    ID              string `json:"id"`
    InvestorAddress string `json:"investor_address"`
    FounderAddress  string `json:"founder_address"`
    Milestones      []struct {
        ID     string `json:"id"`
        Title  string `json:"title"`
        Amount int64  `json:"amount"`
    } `json:"milestones"`
}

func (s *Server) handleCreateContract(w http.ResponseWriter, r *http.Request) {
    var req createContractRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }
    if req.ID == "" || req.InvestorAddress == "" || req.FounderAddress == "" || len(req.Milestones) == 0 {
        writeError(w, http.StatusBadRequest, "id, investor_address, founder_address, and at least one milestone are required")
        return
    }
    if _, exists := s.Contracts.Get(req.ID); exists {
        writeError(w, http.StatusConflict, "a contract with this id already exists")
        return
    }

    milestones := make([]*escrow.Milestone, 0, len(req.Milestones))
    for _, m := range req.Milestones {
        milestones = append(milestones, escrow.NewMilestone(m.ID, req.ID, m.Title, m.Amount))
    }

    contract, err := escrow.NewContract(req.ID, req.InvestorAddress, req.FounderAddress, milestones)
    if err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    if err := s.Contracts.Save(contract); err != nil {
        writeError(w, http.StatusInternalServerError, "failed to persist contract: "+err.Error())
        return
    }
    writeJSON(w, http.StatusCreated, contract)
}

func (s *Server) handleGetContract(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "contractID")
    contract, ok := s.Contracts.Get(id)
    if !ok {
        writeError(w, http.StatusNotFound, "contract not found")
        return
    }
    writeJSON(w, http.StatusOK, contract)
}

type advanceContractRequest struct {
    ToState string `json:"to_state"`
}

func (s *Server) handleAdvanceContract(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "contractID")
    contract, ok := s.Contracts.Get(id)
    if !ok {
        writeError(w, http.StatusNotFound, "contract not found")
        return
    }
    var req advanceContractRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }
    if err := contract.Advance(req.ToState); err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    if err := s.Contracts.Save(contract); err != nil {
        writeError(w, http.StatusInternalServerError, "failed to persist contract: "+err.Error())
        return
    }
    writeJSON(w, http.StatusOK, contract)
}

func (s *Server) handleFundingTx(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "contractID")
    contract, ok := s.Contracts.Get(id)
    if !ok {
        writeError(w, http.StatusNotFound, "contract not found")
        return
    }
    investorNonce := s.Chain.State.GetNonce(contract.InvestorAddress)
    tx, err := contract.BuildFundingTx(investorNonce)
    if err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    writeJSON(w, http.StatusOK, map[string]interface{}{
        "unsigned_tx": tx,
        "note":        "Sign this with the investor's wallet (internal/wallet), then POST the signed result to /api/chain/tx",
    })
}

func (s *Server) handleConfirmFunded(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "contractID")
    contract, ok := s.Contracts.Get(id)
    if !ok {
        writeError(w, http.StatusNotFound, "contract not found")
        return
    }
    lock := s.Chain.State.GetLock(id)
    if lock == nil {
        writeError(w, http.StatusBadRequest, "no on-chain escrow lock found for this contract yet - submit the funding tx first")
        return
    }
    if lock.Amount != contract.TotalAmount {
        writeError(w, http.StatusBadRequest, "on-chain lock amount does not match contract total")
        return
    }
    if err := contract.Advance(escrow.StateFunded); err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    if err := contract.Advance(escrow.StateInProgress); err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    if err := s.Contracts.Save(contract); err != nil {
        writeError(w, http.StatusInternalServerError, "failed to persist contract: "+err.Error())
        return
    }
    writeJSON(w, http.StatusOK, contract)
}

func (s *Server) handleSubmitMilestone(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "contractID")
    milestoneID := chi.URLParam(r, "milestoneID")
    contract, ok := s.Contracts.Get(id)
    if !ok {
        writeError(w, http.StatusNotFound, "contract not found")
        return
    }
    if err := contract.SubmitMilestone(milestoneID); err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    if err := s.Contracts.Save(contract); err != nil {
        writeError(w, http.StatusInternalServerError, "failed to persist contract: "+err.Error())
        return
    }
    writeJSON(w, http.StatusOK, contract)
}

func (s *Server) handleDisputeMilestone(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "contractID")
    milestoneID := chi.URLParam(r, "milestoneID")
    contract, ok := s.Contracts.Get(id)
    if !ok {
        writeError(w, http.StatusNotFound, "contract not found")
        return
    }
    if err := contract.DisputeMilestone(milestoneID); err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    if err := s.Contracts.Save(contract); err != nil {
        writeError(w, http.StatusInternalServerError, "failed to persist contract: "+err.Error())
        return
    }
    writeJSON(w, http.StatusOK, contract)
}

func (s *Server) handleReleaseMilestone(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "contractID")
    milestoneID := chi.URLParam(r, "milestoneID")
    contract, ok := s.Contracts.Get(id)
    if !ok {
        writeError(w, http.StatusNotFound, "contract not found")
        return
    }
    tx, err := contract.BuildMilestoneReleaseTx(milestoneID, s.EscrowWallet.Key.Address, s.nextOpNonce())
    if err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    if err := tx.Sign(s.EscrowWallet.Key.PrivateKey, s.EscrowWallet.Key.PublicKey); err != nil {
        writeError(w, http.StatusInternalServerError, "failed to sign release transaction: "+err.Error())
        return
    }
    if err := s.submitTx(tx); err != nil {
        writeError(w, http.StatusBadRequest, "release transaction rejected: "+err.Error())
        return
    }
    if err := contract.ApproveMilestone(milestoneID); err != nil {
        writeError(w, http.StatusInternalServerError, "on-chain release succeeded but contract state update failed: "+err.Error())
        return
    }
    if err := s.Contracts.Save(contract); err != nil {
        writeError(w, http.StatusInternalServerError, "on-chain release succeeded but contract persistence failed: "+err.Error())
        return
    }
    writeJSON(w, http.StatusOK, map[string]interface{}{"tx_id": tx.ID, "contract": contract})
}

type arbitrateRequest struct {
    MilestoneID string `json:"milestone_id"`
    AwardTo     string `json:"award_to"`
    Amount      int64  `json:"amount"`
    Terminate   bool   `json:"terminate"`
}

func (s *Server) handleArbitrate(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "contractID")
    contract, ok := s.Contracts.Get(id)
    if !ok {
        writeError(w, http.StatusNotFound, "contract not found")
        return
    }
    var req arbitrateRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }
    tx, err := contract.BuildArbitrationTx(req.MilestoneID, s.EscrowWallet.Key.Address, req.AwardTo, req.Amount, s.nextOpNonce())
    if err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    if err := tx.Sign(s.EscrowWallet.Key.PrivateKey, s.EscrowWallet.Key.PublicKey); err != nil {
        writeError(w, http.StatusInternalServerError, "failed to sign arbitration transaction: "+err.Error())
        return
    }
    if err := s.submitTx(tx); err != nil {
        writeError(w, http.StatusBadRequest, "arbitration transaction rejected: "+err.Error())
        return
    }
    if err := contract.ResolveArbitration(req.MilestoneID, req.Terminate); err != nil {
        writeError(w, http.StatusInternalServerError, "on-chain arbitration succeeded but contract state update failed: "+err.Error())
        return
    }
    if err := s.Contracts.Save(contract); err != nil {
        writeError(w, http.StatusInternalServerError, "on-chain arbitration succeeded but contract persistence failed: "+err.Error())
        return
    }
    writeJSON(w, http.StatusOK, map[string]interface{}{"tx_id": tx.ID, "contract": contract})
}

func (s *Server) handleRefund(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "contractID")
    contract, ok := s.Contracts.Get(id)
    if !ok {
        writeError(w, http.StatusNotFound, "contract not found")
        return
    }
    tx, err := contract.BuildRefundTx(s.EscrowWallet.Key.Address, s.nextOpNonce())
    if err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    if err := tx.Sign(s.EscrowWallet.Key.PrivateKey, s.EscrowWallet.Key.PublicKey); err != nil {
        writeError(w, http.StatusInternalServerError, "failed to sign refund transaction: "+err.Error())
        return
    }
    if err := s.submitTx(tx); err != nil {
        writeError(w, http.StatusBadRequest, "refund transaction rejected: "+err.Error())
        return
    }
    if err := contract.Advance(escrow.StateTerminated); err != nil {
        writeError(w, http.StatusInternalServerError, "on-chain refund succeeded but contract state update failed: "+err.Error())
        return
    }
    if err := s.Contracts.Save(contract); err != nil {
        writeError(w, http.StatusInternalServerError, "on-chain refund succeeded but contract persistence failed: "+err.Error())
        return
    }
    writeJSON(w, http.StatusOK, map[string]interface{}{"tx_id": tx.ID, "contract": contract})
}