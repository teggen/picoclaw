package metrics

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sipeed/picoclaw/clients/cli/internal/styles"
)

func renderLLMPanel(snap, prev *MetricsSnapshot, w, h int, active bool) string {
	border := panelBorder(active)
	title := styles.TitleStyle.Render("LLM Stats")

	if snap == nil {
		return border.Width(w).Height(h).Render(title + "\n\n" + styles.MutedStyle.Render("  No data"))
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")

	if len(snap.LLMCalls) == 0 {
		lines = append(lines, styles.MutedStyle.Render("  No LLM calls yet"))
	} else {
		header := fmt.Sprintf("  %-16s %6s %6s %8s %7s", "Model", "OK", "Err", "Avg", "Tokens")
		lines = append(lines, styles.LabelStyle.Render(header))

		models := sortedKeys(snap.LLMCalls)
		for _, model := range models {
			calls := snap.LLMCalls[model]
			dur := snap.LLMDuration[model]
			prompt := snap.LLMPromptTok[model]
			complete := snap.LLMCompleteTok[model]
			avg := avgDuration(dur)
			tokens := prompt + complete

			line := fmt.Sprintf("  %-16s %6.0f %6.0f %7.1fs %7s",
				truncStr(model, 16), calls.Success, calls.Error, avg, formatCount(tokens))

			if prev != nil {
				if delta := rateDelta(calls.Success+calls.Error, prevTotal(prev.LLMCalls, model)); delta != "" {
					line += " " + styles.MutedStyle.Render(delta)
				}
			}
			lines = append(lines, styles.ValueStyle.Render(line))
		}
	}

	content := strings.Join(lines, "\n")
	return border.Width(w).Height(h).Render(content)
}

func renderToolPanel(snap, prev *MetricsSnapshot, w, h int, active bool) string {
	border := panelBorder(active)
	title := styles.TitleStyle.Render("Tool Stats")

	if snap == nil {
		return border.Width(w).Height(h).Render(title + "\n\n" + styles.MutedStyle.Render("  No data"))
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")

	if len(snap.ToolCalls) == 0 {
		lines = append(lines, styles.MutedStyle.Render("  No tool calls yet"))
	} else {
		header := fmt.Sprintf("  %-20s %6s %6s %8s", "Tool", "OK", "Err", "Avg")
		lines = append(lines, styles.LabelStyle.Render(header))

		tools := sortedKeys(snap.ToolCalls)
		for _, tool := range tools {
			calls := snap.ToolCalls[tool]
			dur := snap.ToolDuration[tool]
			avg := avgDuration(dur)

			line := fmt.Sprintf("  %-20s %6.0f %6.0f %7.1fs",
				truncStr(tool, 20), calls.Success, calls.Error, avg)

			if prev != nil {
				if delta := rateDelta(calls.Success+calls.Error, prevTotal(prev.ToolCalls, tool)); delta != "" {
					line += " " + styles.MutedStyle.Render(delta)
				}
			}
			lines = append(lines, styles.ValueStyle.Render(line))
		}
	}

	content := strings.Join(lines, "\n")
	return border.Width(w).Height(h).Render(content)
}

func renderMessagePanel(snap, prev *MetricsSnapshot, w, h int, active bool) string {
	border := panelBorder(active)
	title := styles.TitleStyle.Render("Messages")

	if snap == nil {
		return border.Width(w).Height(h).Render(title + "\n\n" + styles.MutedStyle.Render("  No data"))
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")

	// Inbound
	lines = append(lines, styles.LabelStyle.Render("  Inbound"))
	if len(snap.Inbound) == 0 {
		lines = append(lines, styles.MutedStyle.Render("    (none)"))
	} else {
		for _, ch := range sortedStringKeys(snap.Inbound) {
			lines = append(lines, styles.ValueStyle.Render(
				fmt.Sprintf("    %-14s %6.0f", ch, snap.Inbound[ch])))
		}
	}

	// Outbound
	lines = append(lines, styles.LabelStyle.Render("  Outbound"))
	if len(snap.Outbound) == 0 {
		lines = append(lines, styles.MutedStyle.Render("    (none)"))
	} else {
		for _, ch := range sortedStringKeys(snap.Outbound) {
			lines = append(lines, styles.ValueStyle.Render(
				fmt.Sprintf("    %-14s %6.0f", ch, snap.Outbound[ch])))
		}
	}

	// Turns
	lines = append(lines, styles.LabelStyle.Render("  Turns"))
	lines = append(lines, styles.ValueStyle.Render(
		fmt.Sprintf("    ok: %.0f  err: %.0f", snap.Turns.Success, snap.Turns.Error)))

	// Sessions
	lines = append(lines, styles.LabelStyle.Render("  Sessions"))
	lines = append(lines, styles.ValueStyle.Render(
		fmt.Sprintf("    %.0f active", snap.ActiveSessions)))

	content := strings.Join(lines, "\n")
	return border.Width(w).Height(h).Render(content)
}

func renderSystemPanel(snap *MetricsSnapshot, w, h int, active bool) string {
	border := panelBorder(active)
	title := styles.TitleStyle.Render("System")

	if snap == nil {
		return border.Width(w).Height(h).Render(title + "\n\n" + styles.MutedStyle.Render("  No data"))
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, row("Goroutines", fmt.Sprintf("%.0f", snap.Goroutines)))
	lines = append(lines, row("Heap", formatBytes(snap.HeapAlloc)))
	lines = append(lines, row("Sys Memory", formatBytes(snap.SysMemory)))
	lines = append(lines, row("RSS", formatBytes(snap.RSS)))

	content := strings.Join(lines, "\n")
	return border.Width(w).Height(h).Render(content)
}

func panelBorder(active bool) lipgloss.Style {
	color := styles.Primary
	if active {
		color = styles.Secondary
	}
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(0, 1)
}

func row(label, value string) string {
	return "  " + styles.LabelStyle.Render(fmt.Sprintf("%-14s", label)) + " " + styles.ValueStyle.Render(value)
}

func formatBytes(b float64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", b/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", b/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", b/(1<<10))
	default:
		return fmt.Sprintf("%.0f B", b)
	}
}

func formatCount(v float64) string {
	switch {
	case v >= 1_000_000:
		return fmt.Sprintf("%.1fM", v/1_000_000)
	case v >= 1_000:
		return fmt.Sprintf("%.1fk", v/1_000)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}

func avgDuration(h HistogramData) float64 {
	if h.Count == 0 {
		return 0
	}
	return h.Sum / float64(h.Count)
}

func truncStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func rateDelta(current, previous float64) string {
	diff := current - previous
	if diff <= 0 {
		return ""
	}
	return fmt.Sprintf("+%.0f", diff)
}

func prevTotal(m map[string]CounterByStatus, key string) float64 {
	c, ok := m[key]
	if !ok {
		return 0
	}
	return c.Success + c.Error
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringKeys(m map[string]float64) []string {
	return sortedKeys(m)
}
