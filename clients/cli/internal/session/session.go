package session

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/clients/cli/internal/client"
	"github.com/sipeed/picoclaw/clients/cli/internal/styles"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Session management",
	}

	cmd.AddCommand(
		newListCommand(),
		newShowCommand(),
		newDeleteCommand(),
	)
	return cmd
}

func newListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List sessions",
		RunE:  runList,
	}
}

func newShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show session messages",
		Args:  cobra.ExactArgs(1),
		RunE:  runShow,
	}
}

func newDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a session",
		Args:  cobra.ExactArgs(1),
		RunE:  runDelete,
	}
}

func runList(cmd *cobra.Command, _ []string) error {
	c, jsonMode, _ := client.FromCommand(cmd)

	sessions, err := c.ListSessions()
	if err != nil {
		return err
	}

	if jsonMode {
		return printJSON(sessions)
	}

	if len(sessions) == 0 {
		fmt.Println(styles.MutedStyle.Render("No sessions found."))
		return nil
	}

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
		Headers("#", "ID", "TITLE", "MESSAGES", "UPDATED")

	for i, s := range sessions {
		title := s.Title
		if len(title) > 30 {
			title = title[:28] + ".."
		}
		updated := s.Updated
		if len(updated) > 19 {
			updated = updated[:19]
		}
		t.Row(strconv.Itoa(i+1), s.ID, title, strconv.Itoa(s.MessageCount), updated)
	}

	fmt.Println(t.Render())
	return nil
}

func runShow(cmd *cobra.Command, args []string) error {
	c, jsonMode, _ := client.FromCommand(cmd)

	sess, err := c.GetSession(args[0])
	if err != nil {
		return err
	}

	if jsonMode {
		return printJSON(sess)
	}

	fmt.Println(styles.TitleStyle.Render(fmt.Sprintf("Session: %s", sess.ID)))
	if sess.Summary != "" {
		fmt.Println(styles.MutedStyle.Render(sess.Summary))
	}
	fmt.Println()

	for _, msg := range sess.Messages {
		switch msg.Role {
		case "user":
			fmt.Printf("%s %s\n\n", styles.UserBubble.Render("you:"), msg.Content)
		case "assistant":
			fmt.Printf("%s %s\n\n", styles.AgentBubble.Render("assistant:"), msg.Content)
		}
	}

	fmt.Println(styles.MutedStyle.Render(fmt.Sprintf("Created: %s  Updated: %s", sess.Created, sess.Updated)))
	return nil
}

func runDelete(cmd *cobra.Command, args []string) error {
	c, jsonMode, _ := client.FromCommand(cmd)

	// Prompt for confirmation unless in JSON mode.
	if !jsonMode {
		fmt.Printf("Delete session %s? [y/N] ", args[0])
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	if err := c.DeleteSession(args[0]); err != nil {
		if jsonMode {
			fmt.Fprintf(os.Stderr, `{"error":%q}`+"\n", err.Error())
		}
		return err
	}

	if jsonMode {
		fmt.Println(`{"deleted":true}`)
	} else {
		fmt.Println(styles.ValueStyle.Render("Session deleted."))
	}
	return nil
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
