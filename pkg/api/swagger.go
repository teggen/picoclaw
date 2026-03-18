package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed openapi.yaml
var openapiSpec []byte

//go:embed swagger_ui/*
var swaggerUI embed.FS

// handleOpenAPISpec serves the OpenAPI specification.
// GET /api/v1/openapi.yaml
func (h *Handler) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Write(openapiSpec)
}

// swaggerFileServer returns an http.Handler that serves the embedded Swagger UI files.
func (h *Handler) swaggerFileServer() http.Handler {
	sub, _ := fs.Sub(swaggerUI, "swagger_ui")
	return http.FileServer(http.FS(sub))
}
