package api

import (
	"encoding/json"
	"net/http"
)

// writeJSON encodes v as JSON with the given status code, setting the
// Content-Type header consistently across every handler.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error body: {"error": "..."}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// requireInternalKey protects endpoints that only the Venturify backend
// should be able to call (minting, milestone release, arbitration, refund),
// via a shared secret passed in the X-Internal-Api-Key header. This is a
// devnet-appropriate auth model - a production deployment would use mTLS or
// a signed service token instead of a static shared secret.
func (s *Server) requireInternalKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Internal-Api-Key")
		if key == "" || key != s.InternalAPIKey {
			writeError(w, http.StatusUnauthorized, "missing or invalid internal API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}