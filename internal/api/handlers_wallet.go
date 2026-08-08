package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type balanceResponse struct {
	Address string `json:"address"`
	Balance int64  `json:"balance"`
}

func (s *Server) handleGetBalance(w http.ResponseWriter, r *http.Request) {
	address := chi.URLParam(r, "address")
	if address == "" {
		writeError(w, http.StatusBadRequest, "address is required")
		return
	}
	balance := s.Chain.State.GetBalance(address)
	writeJSON(w, http.StatusOK, balanceResponse{Address: address, Balance: balance})
}

type nonceResponse struct {
	Address string `json:"address"`
	Nonce   int64  `json:"nonce"`
}

func (s *Server) handleGetNonce(w http.ResponseWriter, r *http.Request) {
	address := chi.URLParam(r, "address")
	if address == "" {
		writeError(w, http.StatusBadRequest, "address is required")
		return
	}
	nonce := s.Chain.State.GetNonce(address)
	writeJSON(w, http.StatusOK, nonceResponse{Address: address, Nonce: nonce})
}