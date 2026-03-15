package commands

import (
	"context"
	"strings"
	"testing"
)

func TestContextCommand_NilRuntime(t *testing.T) {
	def := contextCommand()
	var reply string
	err := def.Handler(context.Background(), Request{
		Text:  "/context",
		Reply: func(text string) error { reply = text; return nil },
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != unavailableMsg {
		t.Fatalf("got %q, want %q", reply, unavailableMsg)
	}
}

func TestContextCommand_FormatsOutput(t *testing.T) {
	rt := &Runtime{
		GetContextInfo: func() (*ContextInfo, error) {
			return &ContextInfo{
				EstimatedTokens: 4000,
				ContextWindow:   16000,
				MaxOutputTokens: 8192,
				MessageCount:    12,
				HasSummary:      true,
			}, nil
		},
	}
	def := contextCommand()
	var reply string
	err := def.Handler(context.Background(), Request{
		Text:  "/context",
		Reply: func(text string) error { reply = text; return nil },
	}, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "25%") {
		t.Fatalf("expected 25%% in reply, got %q", reply)
	}
	if !strings.Contains(reply, "~4000 / 16000") {
		t.Fatalf("expected token counts in reply, got %q", reply)
	}
	if !strings.Contains(reply, "Max output: 8192") {
		t.Fatalf("expected max output tokens in reply, got %q", reply)
	}
	if !strings.Contains(reply, "Messages: 12") {
		t.Fatalf("expected message count in reply, got %q", reply)
	}
	if !strings.Contains(reply, "Summary: Yes") {
		t.Fatalf("expected summary status in reply, got %q", reply)
	}
	if !strings.Contains(reply, "[#####---------------]") {
		t.Fatalf("expected visual bar in reply, got %q", reply)
	}
}

func TestContextCommand_ZeroWindow(t *testing.T) {
	rt := &Runtime{
		GetContextInfo: func() (*ContextInfo, error) {
			return &ContextInfo{
				EstimatedTokens: 0,
				ContextWindow:   0,
				MessageCount:    0,
				HasSummary:      false,
			}, nil
		},
	}
	def := contextCommand()
	var reply string
	err := def.Handler(context.Background(), Request{
		Text:  "/context",
		Reply: func(text string) error { reply = text; return nil },
	}, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "0%") {
		t.Fatalf("expected 0%% in reply, got %q", reply)
	}
	if !strings.Contains(reply, "~0 / 0") {
		t.Fatalf("expected zero tokens in reply, got %q", reply)
	}
}
