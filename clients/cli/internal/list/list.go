package list

import (
	"encoding/json"
	"fmt"
	"strconv"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/clients/cli/internal/client"
	"github.com/sipeed/picoclaw/clients/cli/internal/styles"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:       "list <resource>",
		Short:     "List resources (channels, agents, tools)",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"channels", "agents", "tools"},
		RunE:      run,
	}
	return cmd
}

func run(cmd *cobra.Command, args []string) error {
	c, jsonMode, _ := client.FromCommand(cmd)
	resource := args[0]

	switch resource {
	case "channels":
		return listChannels(c, jsonMode)
	case "agents":
		return listAgents(c, jsonMode)
	case "tools":
		return listTools(c, jsonMode)
	default:
		return fmt.Errorf("unknown resource: %s (use channels, agents, or tools)", resource)
	}
}

func listChannels(c *client.Client, jsonMode bool) error {
	channels, err := c.ListChannels()
	if err != nil {
		return err
	}
	if jsonMode {
		return printJSON(channels)
	}

	rows := make([][]string, 0, len(channels))
	for i, ch := range channels {
		enabled := "yes"
		if !ch.Enabled {
			enabled = "no"
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), ch.Name, enabled})
	}

	fmt.Println(renderTable([]string{"#", "NAME", "ENABLED"}, rows))
	return nil
}

func listAgents(c *client.Client, jsonMode bool) error {
	agents, err := c.ListAgents()
	if err != nil {
		return err
	}
	if jsonMode {
		return printJSON(agents)
	}

	rows := make([][]string, 0, len(agents))
	for i, a := range agents {
		model := a.Model
		if len(model) > 24 {
			model = model[:22] + ".."
		}
		rows = append(rows, []string{
			strconv.Itoa(i + 1),
			a.ID,
			model,
			strconv.Itoa(a.MaxIterations),
			strconv.Itoa(a.MaxTokens),
		})
	}

	fmt.Println(renderTable([]string{"#", "ID", "MODEL", "MAX_ITER", "MAX_TOKENS"}, rows))
	return nil
}

func listTools(c *client.Client, jsonMode bool) error {
	tools, err := c.ListTools()
	if err != nil {
		return err
	}
	if jsonMode {
		return printJSON(tools)
	}

	rows := make([][]string, 0, len(tools))
	for i, t := range tools {
		desc := t.Description
		if len(desc) > 50 {
			desc = desc[:48] + ".."
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), t.Name, desc})
	}

	fmt.Println(renderTable([]string{"#", "NAME", "DESCRIPTION"}, rows))
	return nil
}

func renderTable(headers []string, rows [][]string) string {
	headerStyle := lipgloss.NewStyle().
		Foreground(styles.Primary).
		Bold(true).
		Align(lipgloss.Center)
	cellStyle := lipgloss.NewStyle().Padding(0, 1)
	oddRowStyle := cellStyle.Foreground(lipgloss.Color("245"))
	evenRowStyle := cellStyle.Foreground(lipgloss.Color("241"))

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(styles.Primary)).
		StyleFunc(func(row, col int) lipgloss.Style {
			switch {
			case row == table.HeaderRow:
				return headerStyle
			case row%2 == 0:
				return evenRowStyle
			default:
				return oddRowStyle
			}
		}).
		Headers(headers...)

	for _, row := range rows {
		t.Row(row...)
	}

	return t.Render()
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
