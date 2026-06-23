package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter builds the HTTP router with all registered routes.
// chi gives us path params (/job/{id}/status) and lightweight middleware.
func NewRouter(h *Handler) http.Handler {
	r := chi.NewRouter()

	// Standard middleware: request tracing, client IP, panic recovery.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/health", h.Health)
	// Job routes (submit, status, download) are added in Steps 11–13.

	return r
}
