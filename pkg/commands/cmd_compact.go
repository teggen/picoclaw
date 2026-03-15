package commands

import (
	"context"
	"fmt"
)

func compactCommand() Definition {
	return Definition{
		Name:        "compact",
		Description: "Compact conversation history",
		Usage:       "/compact",
		Handler: func(ctx context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.GetContextInfo == nil || rt.CompactHistory == nil {
				return req.Reply(unavailableMsg)
			}

			before, err := rt.GetContextInfo()
			if err != nil {
				return req.Reply("Failed to get context info: " + err.Error())
			}

			after, err := rt.CompactHistory(ctx)
			if err != nil {
				return req.Reply("Failed to compact history: " + err.Error())
			}

			freed := before.EstimatedTokens - after.EstimatedTokens
			if freed <= 0 {
				return req.Reply("Conversation is already compact.")
			}

			remaining := 0
			if after.ContextWindow > 0 {
				remaining = (after.ContextWindow - after.EstimatedTokens) * 100 / after.ContextWindow
			}
			if remaining < 0 {
				remaining = 0
			}

			return req.Reply(fmt.Sprintf(
				"Conversation compacted.\nFreed: ~%d tokens\nRemaining: %d%% of context window available\nMessages: %d",
				freed, remaining, after.MessageCount,
			))
		},
	}
}
