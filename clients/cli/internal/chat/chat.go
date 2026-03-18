package chat

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/clients/cli/internal/client"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Interactive chat TUI via WebSocket",
		RunE:  run,
	}
	cmd.Flags().StringP("session", "s", "", "Resume an existing session by ID")
	return cmd
}

func run(cmd *cobra.Command, _ []string) error {
	c, _, _ := client.FromCommand(cmd)
	sessionID, _ := cmd.Flags().GetString("session")

	// Verify gateway is reachable.
	if err := c.Health(); err != nil {
		return err
	}

	conn, err := client.DialChat(c.BaseURL, sessionID)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Use the resolved session ID from the connection.
	sid := conn.SessionID()
	if sid == "" {
		sid = sessionID
	}

	model := newChatModel(conn, c, sid)
	p := tea.NewProgram(model)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("chat TUI error: %w", err)
	}
	return nil
}
