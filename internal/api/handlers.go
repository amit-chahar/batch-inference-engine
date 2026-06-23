package api

import (
	"encoding/json"
	"net/http"
)

// Handler serves HTTP endpoints for the batch inference engine.
type Handler struct {
	version string
}

// NewHandler constructs an API handler.
func NewHandler(version string) *Handler {
	return &Handler{version: version}
}

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Health reports service liveness.
func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:  "ok",
		Version: h.version,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
