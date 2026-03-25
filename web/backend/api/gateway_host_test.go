package api

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestPublicLauncherDoesNotOverrideGatewayBind(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 18790
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	h.SetServerOptions(18800, true, true, nil)

	if got := h.effectiveGatewayBindHost(cfg); got != "127.0.0.1" {
		t.Fatalf("effectiveGatewayBindHost() = %q, want %q (public mode should not override)", got, "127.0.0.1")
	}
}

func TestBuildWsURLUsesRequestHostWhenGatewayBindIsWildcard(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)
	h.SetServerOptions(18800, false, false, nil)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "" // empty = wildcard, should use request host
	cfg.Gateway.Port = 18790

	req := httptest.NewRequest("GET", "http://launcher.local/api/pico/token", nil)
	req.Host = "192.168.1.9:18800"

	if got := h.buildWsURL(req, cfg); got != "ws://192.168.1.9:18800/pico/ws" {
		t.Fatalf("buildWsURL() = %q, want %q", got, "ws://192.168.1.9:18800/pico/ws")
	}
}

func TestBuildWsURLUsesConfigHostWhenSet(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)
	h.SetServerOptions(18800, false, false, nil)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "10.0.0.5"
	cfg.Gateway.Port = 18790

	req := httptest.NewRequest("GET", "http://launcher.local/api/pico/token", nil)
	req.Host = "192.168.1.9:18800"

	if got := h.buildWsURL(req, cfg); got != "ws://10.0.0.5:18800/pico/ws" {
		t.Fatalf("buildWsURL() = %q, want %q", got, "ws://10.0.0.5:18800/pico/ws")
	}
}

func TestGatewayProbeHostUsesLoopbackForWildcardBind(t *testing.T) {
	if got := gatewayProbeHost("0.0.0.0"); got != "127.0.0.1" {
		t.Fatalf("gatewayProbeHost() = %q, want %q", got, "127.0.0.1")
	}
}

func TestGatewayProxyURLUsesConfiguredHost(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "192.168.1.10"
	cfg.Gateway.Port = 18791
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	if got := h.gatewayProxyURL().String(); got != "http://192.168.1.10:18791" {
		t.Fatalf("gatewayProxyURL() = %q, want %q", got, "http://192.168.1.10:18791")
	}
}

func TestGetGatewayHealthUsesConfiguredHost(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "192.168.1.10"
	cfg.Gateway.Port = 18791

	originalHealthGet := gatewayHealthGet
	t.Cleanup(func() {
		gatewayHealthGet = originalHealthGet
	})

	var requestedURL string
	gatewayHealthGet = func(url string, timeout time.Duration) (*http.Response, error) {
		requestedURL = url
		return nil, errors.New("probe failed")
	}

	_, statusCode, err := h.getGatewayHealth(cfg, time.Second)
	_ = statusCode
	_ = err

	if requestedURL != "http://192.168.1.10:18791/health" {
		t.Fatalf("health url = %q, want %q", requestedURL, "http://192.168.1.10:18791/health")
	}
}

func TestGetGatewayHealthUsesConfigHostForPublicLauncher(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)
	h.SetServerOptions(18800, true, true, nil)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "192.168.1.10"
	cfg.Gateway.Port = 18791

	originalHealthGet := gatewayHealthGet
	t.Cleanup(func() {
		gatewayHealthGet = originalHealthGet
	})

	var requestedURL string
	gatewayHealthGet = func(url string, timeout time.Duration) (*http.Response, error) {
		requestedURL = url
		return nil, errors.New("probe failed")
	}

	_, statusCode, err := h.getGatewayHealth(cfg, time.Second)
	_ = statusCode
	_ = err

	// Public mode no longer overrides the gateway host — config host is used directly.
	if requestedURL != "http://192.168.1.10:18791/health" {
		t.Fatalf("health url = %q, want %q", requestedURL, "http://192.168.1.10:18791/health")
	}
}

func TestBuildWsURLUsesWSSWhenForwardedProtoIsHTTPS(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "0.0.0.0"
	cfg.Gateway.Port = 18790

	req := httptest.NewRequest("GET", "http://launcher.local/api/pico/token", nil)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")

	if got := h.buildWsURL(req, cfg); got != "wss://chat.example.com:18800/pico/ws" {
		t.Fatalf("buildWsURL() = %q, want %q", got, "wss://chat.example.com:18800/pico/ws")
	}
}

func TestBuildWsURLUsesWSSWhenRequestIsTLS(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "0.0.0.0"
	cfg.Gateway.Port = 18790

	req := httptest.NewRequest("GET", "https://launcher.local/api/pico/token", nil)
	req.Host = "secure.example.com"
	req.TLS = &tls.ConnectionState{}

	if got := h.buildWsURL(req, cfg); got != "wss://secure.example.com:18800/pico/ws" {
		t.Fatalf("buildWsURL() = %q, want %q", got, "wss://secure.example.com:18800/pico/ws")
	}
}

func TestBuildWsURLPrefersForwardedHTTPOverTLS(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "0.0.0.0"
	cfg.Gateway.Port = 18790

	req := httptest.NewRequest("GET", "https://launcher.local/api/pico/token", nil)
	req.Host = "chat.example.com"
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("X-Forwarded-Proto", "http")

	if got := h.buildWsURL(req, cfg); got != "ws://chat.example.com:18800/pico/ws" {
		t.Fatalf("buildWsURL() = %q, want %q", got, "ws://chat.example.com:18800/pico/ws")
	}
}
