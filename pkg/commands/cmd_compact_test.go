package commands

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestCompactCommand_NilRuntime(t *testing.T) {
	def := compactCommand()
	var reply string
	err := def.Handler(context.Background(), Request{
		Text:  "/compact",
		Reply: func(text string) error { reply = text; return nil },
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != unavailableMsg {
		t.Fatalf("got %q, want %q", reply, unavailableMsg)
	}
}

func TestCompactCommand_ReportsFreedTokens(t *testing.T) {
	rt := &Runtime{
		GetContextInfo: func() (*ContextInfo, error) {
			return &ContextInfo{
				EstimatedTokens: 8000,
				ContextWindow:   16000,
				MessageCount:    20,
				HasSummary:      false,
			}, nil
		},
		CompactHistory: func(_ context.Context) (*ContextInfo, error) {
			return &ContextInfo{
				EstimatedTokens: 2000,
				ContextWindow:   16000,
				MessageCount:    4,
				HasSummary:      true,
			}, nil
		},
	}
	def := compactCommand()
	var reply string
	err := def.Handler(context.Background(), Request{
		Text:  "/compact",
		Reply: func(text string) error { reply = text; return nil },
	}, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "~6000 tokens") {
		t.Fatalf("expected freed tokens in reply, got %q", reply)
	}
	if !strings.Contains(reply, "87%") {
		t.Fatalf("expected remaining percentage in reply, got %q", reply)
	}
	if !strings.Contains(reply, "Messages: 4") {
		t.Fatalf("expected message count in reply, got %q", reply)
	}
}

func TestCompactCommand_AlreadyCompact(t *testing.T) {
	info := &ContextInfo{
		EstimatedTokens: 2000,
		ContextWindow:   16000,
		MessageCount:    4,
		HasSummary:      true,
	}
	rt := &Runtime{
		GetContextInfo: func() (*ContextInfo, error) {
			return info, nil
		},
		CompactHistory: func(_ context.Context) (*ContextInfo, error) {
			return info, nil
		},
	}
	def := compactCommand()
	var reply string
	err := def.Handler(context.Background(), Request{
		Text:  "/compact",
		Reply: func(text string) error { reply = text; return nil },
	}, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "already compact") {
		t.Fatalf("expected already compact message, got %q", reply)
	}
}

func TestCompactCommand_Error(t *testing.T) {
	rt := &Runtime{
		GetContextInfo: func() (*ContextInfo, error) {
			return &ContextInfo{
				EstimatedTokens: 8000,
				ContextWindow:   16000,
				MessageCount:    20,
			}, nil
		},
		CompactHistory: func(_ context.Context) (*ContextInfo, error) {
			return nil, fmt.Errorf("summarization failed")
		},
	}
	def := compactCommand()
	var reply string
	err := def.Handler(context.Background(), Request{
		Text:  "/compact",
		Reply: func(text string) error { reply = text; return nil },
	}, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "summarization failed") {
		t.Fatalf("expected error message in reply, got %q", reply)
	}
}
