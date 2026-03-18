package status

import (
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/clients/cli/internal/client"
	"github.com/sipeed/picoclaw/clients/cli/internal/styles"
)

func NewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show gateway status dashboard",
		RunE:  run,
	}
}

func run(cmd *cobra.Command, _ []string) error {
	c, jsonMode, _ := client.FromCommand(cmd)

	if jsonMode {
		raw, err := c.GetStatusRaw()
		if err != nil {
			return jsonError(err)
		}
		fmt.Println(string(raw))
		return nil
	}

	st, err := c.GetStatus()
	if err != nil {
		return err
	}

	toolCount := countAny(st.Tools)
	agentCount := countAny(st.Agents)

	channelList := "none"
	if len(st.Channels) > 0 {
		channelList = joinStrings(st.Channels, ", ")
	}

	content := fmt.Sprintf(
		"%s\n\n%s  %s\n%s  %s\n%s  %s\n\n%s  %s\n%s  %s\n%s  %s\n%s  %s",
		styles.TitleStyle.Render("PicoClaw Gateway Status"),
		styles.LabelStyle.Render("Status   "),
		styles.ValueStyle.Render(st.Status),
		styles.LabelStyle.Render("Uptime   "),
		styles.ValueStyle.Render(st.Uptime),
		styles.LabelStyle.Render("Model    "),
		styles.ValueStyle.Render(st.Model),
		styles.LabelStyle.Render("Channels "),
		styles.ValueStyle.Render(channelList),
		styles.LabelStyle.Render("Tools    "),
		styles.ValueStyle.Render(fmt.Sprintf("%d loaded", toolCount)),
		styles.LabelStyle.Render("Agents   "),
		styles.ValueStyle.Render(fmt.Sprintf("%d registered", agentCount)),
		styles.LabelStyle.Render("Skills   "),
		styles.ValueStyle.Render(formatSkills(st.Skills)),
	)

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Padding(1, 2).
		Render(content)

	fmt.Println(box)
	return nil
}

func countAny(v any) int {
	switch val := v.(type) {
	case []any:
		return len(val)
	case float64:
		return int(val)
	case string:
		return 0
	default:
		return 0
	}
}

func formatSkills(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case map[string]any:
		installed, _ := val["installed"].(float64)
		total, _ := val["total"].(float64)
		if total > 0 {
			return fmt.Sprintf("%.0f/%.0f available", installed, total)
		}
		return fmt.Sprintf("%.0f installed", installed)
	default:
		return "n/a"
	}
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

func jsonError(err error) error {
	fmt.Fprintf(os.Stderr, `{"error":%q}`+"\n", err.Error())
	return err
}
