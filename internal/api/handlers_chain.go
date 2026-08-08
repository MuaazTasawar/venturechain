package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/MuaazTasawar/venturechain/internal/blockchain"
	"github.com/go-chi/chi/v5"
)

type heightResponse struct {
	Height int64 `json:"height"`
}

func (s *Server) handleChainHeight(w http.ResponseWriter, r *http.Request) {
	latest := s.Chain.LatestBlock()
	writeJSON(w, http.StatusOK, heightResponse{Height: latest.Index})
}

func (s *Server) handleGetBlock(w http.ResponseWriter, r *http.Request) {
	indexStr := chi.URLParam(r, "index")
	index, err := strconv.ParseInt(indexStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid block index")
		return
	}
	block, err := s.Chain.BlockAt(index)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, block)
}

func (s *Server) handleGetOnChainEscrow(w http.ResponseWriter, r *http.Request) {
	contractID := chi.URLParam(r, "contractID")
	lock := s.Chain.State.GetLock(contractID)
	if lock == nil {
		writeError(w, http.StatusNotFound, "no on-chain escrow lock for this contract")
		return
	}
	writeJSON(w, http.StatusOK, lock)
}

// handleSubmitTx accepts a fully-signed transaction (built and signed
// client-side, e.g. by an investor's wallet for a LOCK or TRANSFER), and
// broadcasts it into the network if it passes validation.
func (s *Server) handleSubmitTx(w http.ResponseWriter, r *http.Request) {
	var tx blockchain.Transaction
	if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
		writeError(w, http.StatusBadRequest, "invalid transaction payload")
		return
	}
	if tx.ID == "" {
		tx.ID = tx.Hash()
	}
	if err := s.submitTx(&tx); err != nil {
		writeError(w, http.StatusBadRequest, "transaction rejected: "+err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"tx_id": tx.ID, "status": "pending"})
}

type mintRequest struct {
	To     string `json:"to"`
	Amount int64  `json:"amount"`
}

// handleMint issues new VTC to an address, called by the Venturify backend
// after a confirmed fiat deposit. Protected by requireInternalKey.
func (s *Server) handleMint(w http.ResponseWriter, r *http.Request) {
	var req mintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.To == "" || req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "to and a positive amount are required")
		return
	}
	tx := blockchain.NewTransaction(blockchain.TxMint, s.TreasuryAddr, req.To, req.Amount, "", "", s.nextOpNonce())
	if err := s.submitTx(tx); err != nil {
		writeError(w, http.StatusBadRequest, "mint rejected: "+err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"tx_id": tx.ID, "status": "pending"})
}