package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handler exposes Prometheus metrics over HTTP.
type Handler struct {
	collector *Collector
}

// NewHandler creates a new metrics HTTP handler.
func NewHandler(collector *Collector) *Handler {
	return &Handler{collector: collector}
}

// RegisterRoutes registers the /metrics endpoint on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/metrics", promhttp.HandlerFor(h.collector.registry, promhttp.HandlerOpts{}))
}
