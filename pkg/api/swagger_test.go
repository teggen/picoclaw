package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleOpenAPISpec(t *testing.T) {
	h := &Handler{
		eventHub:  NewEventHub(),
		startTime: time.Now(),
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/openapi.yaml", nil)
	h.handleOpenAPISpec(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/x-yaml" {
		t.Fatalf("expected content type application/x-yaml, got %q", w.Header().Get("Content-Type"))
	}
	body := w.Body.String()
	if !strings.Contains(body, "openapi:") {
		t.Fatal("response does not contain openapi spec")
	}
	if !strings.Contains(body, "PicoClaw") {
		t.Fatal("response does not contain PicoClaw title")
	}
}

func TestSwaggerUI(t *testing.T) {
	h := &Handler{
		eventHub:  NewEventHub(),
		startTime: time.Now(),
	}

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Swagger UI redirect.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/docs", nil)
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301 redirect, got %d", w.Code)
	}

	// Swagger UI index.
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/api/v1/docs/", nil)
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for docs/, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "swagger-ui") {
		t.Fatal("swagger UI page does not contain swagger-ui")
	}
}
