package metrics

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handler exposes Prometheus metrics over HTTP.
type Handler struct {
	collector *Collector
	apiToken  string
}

// NewHandler creates a new metrics HTTP handler.
func NewHandler(collector *Collector, apiToken string) *Handler {
	return &Handler{collector: collector, apiToken: apiToken}
}

// RegisterRoutes registers the /metrics endpoint on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	handler := promhttp.HandlerFor(h.collector.registry, promhttp.HandlerOpts{})
	mux.Handle("/metrics", h.requireAuth(handler))
}

func (h *Handler) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.apiToken == "" {
			next.ServeHTTP(w, r)
			return
		}

		token := ""
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		}
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if subtle.ConstantTimeCompare([]byte(token), []byte(h.apiToken)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
