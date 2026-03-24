package slackcmd

import (
	"fmt"

	"github.com/slack-go/slack"
	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/cmd/picoclaw/internal"
)

func NewSlackCommand() *cobra.Command {
	var client *slack.Client

	cmd := &cobra.Command{
		Use:   "slack",
		Short: "Slack channel management utilities",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := internal.LoadConfig()
			if err != nil {
				return fmt.Errorf("error loading config: %w", err)
			}
			if cfg.Channels.Slack.BotToken() == "" {
				return fmt.Errorf("slack bot_token is not configured")
			}
			client = slack.New(cfg.Channels.Slack.BotToken())
			return nil
		},
	}

	getClient := func() *slack.Client { return client }

	cmd.AddCommand(
		newListChannelsCommand(getClient),
		newClearHistoryCommand(getClient),
	)

	return cmd
}
