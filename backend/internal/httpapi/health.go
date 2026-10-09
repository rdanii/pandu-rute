// Package httpapi contains the HTTP handlers and routing for the Pandu Rute API.
package httpapi

import (
	"encoding/json"
	"net/http"
)

// NewRouter returns the API's HTTP handler with all routes registered.
func NewRouter() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	return mux
}

type healthResponse struct {
	Status string `json:"status"`
}

// handleHealth reports that the process is up and serving HTTP.
// It does not check downstream dependencies (none exist yet).
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok"})
}
