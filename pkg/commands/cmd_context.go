package commands

import (
	"context"
	"fmt"
	"strings"
)

func contextCommand() Definition {
	return Definition{
		Name:        "context",
		Description: "Show context window usage",
		Usage:       "/context",
		Handler: func(_ context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.GetContextInfo == nil {
				return req.Reply(unavailableMsg)
			}
			info, err := rt.GetContextInfo()
			if err != nil {
				return req.Reply("Failed to get context info: " + err.Error())
			}
			return req.Reply(formatContextInfo(info))
		},
	}
}

func formatContextInfo(info *ContextInfo) string {
	var sb strings.Builder
	sb.WriteString("Context Usage\n")

	pct := 0
	if info.ContextWindow > 0 {
		pct = info.EstimatedTokens * 100 / info.ContextWindow
	}
	if pct > 100 {
		pct = 100
	}

	// 20-char visual bar
	filled := pct * 20 / 100
	bar := strings.Repeat("#", filled) + strings.Repeat("-", 20-filled)
	sb.WriteString(fmt.Sprintf("[%s] %d%%\n", bar, pct))
	sb.WriteString(fmt.Sprintf("Tokens: ~%d / %d\n", info.EstimatedTokens, info.ContextWindow))
	sb.WriteString(fmt.Sprintf("Max output: %d\n", info.MaxOutputTokens))
	sb.WriteString(fmt.Sprintf("Messages: %d\n", info.MessageCount))

	summary := "No"
	if info.HasSummary {
		summary = "Yes"
	}
	sb.WriteString(fmt.Sprintf("Summary: %s", summary))

	return sb.String()
}
