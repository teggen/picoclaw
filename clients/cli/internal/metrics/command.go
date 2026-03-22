package metrics

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/clients/cli/internal/client"
)

// NewCommand returns the "metrics" cobra command.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Live metrics dashboard",
		RunE:  run,
	}
	cmd.Flags().DurationP("interval", "i", 2*time.Second, "Refresh interval")
	return cmd
}

func run(cmd *cobra.Command, _ []string) error {
	c, jsonMode, _ := client.FromCommand(cmd)
	interval, _ := cmd.Flags().GetDuration("interval")

	if jsonMode {
		raw, err := c.GetMetricsRaw()
		if err != nil {
			return err
		}
		fmt.Print(string(raw))
		return nil
	}

	m := newModel(c, interval)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("metrics TUI error: %w", err)
	}
	return nil
}
