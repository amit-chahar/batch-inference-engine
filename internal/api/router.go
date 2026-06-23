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
	r.Post("/job/submit", h.Submit)
	r.Get("/job/{id}/status", h.Status)
	r.Get("/job/{id}/download", h.Download)

	return r
}
