package channels

import (
	"errors"
	"fmt"
	"testing"
)

func TestClassifySendError(t *testing.T) {
	raw := fmt.Errorf("some API error")

	tests := []struct {
		name       string
		statusCode int
		wantIs     error
		wantNil    bool
	}{
		{"429 -> ErrRateLimit", 429, ErrRateLimit, false},
		{"500 -> ErrTemporary", 500, ErrTemporary, false},
		{"502 -> ErrTemporary", 502, ErrTemporary, false},
		{"503 -> ErrTemporary", 503, ErrTemporary, false},
		{"401 -> ErrAuthFailed", 401, ErrAuthFailed, false},
		{"403 -> ErrAuthFailed", 403, ErrAuthFailed, false},
		{"400 -> ErrSendFailed", 400, ErrSendFailed, false},
		{"404 -> ErrSendFailed", 404, ErrSendFailed, false},
		{"200 -> raw error", 200, nil, false},
		{"201 -> raw error", 201, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ClassifySendError(tt.statusCode, raw)
			if err == nil {
				t.Fatal("expected non-nil error")
			}
			if tt.wantIs != nil {
				if !errors.Is(err, tt.wantIs) {
					t.Errorf("errors.Is(err, %v) = false, want true; err = %v", tt.wantIs, err)
				}
			} else {
				// Should return the raw error unchanged
				if err != raw {
					t.Errorf("expected raw error to be returned unchanged for status %d, got %v", tt.statusCode, err)
				}
			}
		})
	}
}

func TestClassifySendErrorNoFalsePositive(t *testing.T) {
	raw := fmt.Errorf("some error")

	// 429 should NOT match ErrTemporary, ErrSendFailed, or ErrAuthFailed
	err := ClassifySendError(429, raw)
	if errors.Is(err, ErrTemporary) {
		t.Error("429 should not match ErrTemporary")
	}
	if errors.Is(err, ErrSendFailed) {
		t.Error("429 should not match ErrSendFailed")
	}
	if errors.Is(err, ErrAuthFailed) {
		t.Error("429 should not match ErrAuthFailed")
	}

	// 500 should NOT match ErrRateLimit or ErrSendFailed
	err = ClassifySendError(500, raw)
	if errors.Is(err, ErrRateLimit) {
		t.Error("500 should not match ErrRateLimit")
	}
	if errors.Is(err, ErrSendFailed) {
		t.Error("500 should not match ErrSendFailed")
	}

	// 401 should NOT match ErrRateLimit, ErrTemporary, or ErrSendFailed
	err = ClassifySendError(401, raw)
	if errors.Is(err, ErrRateLimit) {
		t.Error("401 should not match ErrRateLimit")
	}
	if errors.Is(err, ErrTemporary) {
		t.Error("401 should not match ErrTemporary")
	}
	if errors.Is(err, ErrSendFailed) {
		t.Error("401 should not match ErrSendFailed")
	}

	// 400 should NOT match ErrRateLimit, ErrTemporary, or ErrAuthFailed
	err = ClassifySendError(400, raw)
	if errors.Is(err, ErrRateLimit) {
		t.Error("400 should not match ErrRateLimit")
	}
	if errors.Is(err, ErrTemporary) {
		t.Error("400 should not match ErrTemporary")
	}
	if errors.Is(err, ErrAuthFailed) {
		t.Error("400 should not match ErrAuthFailed")
	}
}

func TestClassifyAPIError(t *testing.T) {
	tests := []struct {
		name   string
		errMsg string
		wantIs error
	}{
		{"rate limit text", "rate limit exceeded", ErrRateLimit},
		{"429 in message", "HTTP 429 too many requests", ErrRateLimit},
		{"unauthorized", "unauthorized: invalid token", ErrAuthFailed},
		{"forbidden", "forbidden: missing permissions", ErrAuthFailed},
		{"401 in message", "HTTP 401 bad credentials", ErrAuthFailed},
		{"403 in message", "HTTP 403 access denied", ErrAuthFailed},
		{"invalid chat ID", "invalid chat ID", ErrSendFailed},
		{"not found", "channel not found", ErrSendFailed},
		{"bad request", "bad request: missing field", ErrSendFailed},
		{"400 in message", "HTTP 400 malformed", ErrSendFailed},
		{"timeout", "context deadline exceeded (timeout)", ErrTemporary},
		{"connection refused", "dial tcp: connection refused", ErrTemporary},
		{"eof", "unexpected EOF", ErrTemporary},
		{"500 in message", "HTTP 500 internal server error", ErrTemporary},
		{"sdk error", "send failed", ErrTemporary},
		{"unknown error", "something went wrong", ErrTemporary},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ClassifyAPIError(fmt.Errorf("%s", tt.errMsg))
			if err == nil {
				t.Fatal("expected non-nil error")
			}
			if !errors.Is(err, tt.wantIs) {
				t.Errorf("ClassifyAPIError(%q): errors.Is(err, %v) = false; err = %v", tt.errMsg, tt.wantIs, err)
			}
		})
	}
}

func TestClassifyAPIError_Nil(t *testing.T) {
	if err := ClassifyAPIError(nil); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestClassifyNetError(t *testing.T) {
	t.Run("nil error returns nil", func(t *testing.T) {
		if err := ClassifyNetError(nil); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("non-nil error wraps as ErrTemporary", func(t *testing.T) {
		raw := fmt.Errorf("connection refused")
		err := ClassifyNetError(raw)
		if err == nil {
			t.Fatal("expected non-nil error")
		}
		if !errors.Is(err, ErrTemporary) {
			t.Errorf("errors.Is(err, ErrTemporary) = false, want true; err = %v", err)
		}
	})
}
