package slackcmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

type slackAPI interface {
	AuthTest() (*slack.AuthTestResponse, error)
	GetConversationHistory(
		params *slack.GetConversationHistoryParameters,
	) (*slack.GetConversationHistoryResponse, error)
	DeleteMessage(channel, messageTimestamp string) (string, string, error)
}

func newClearHistoryCommand(getClient func() *slack.Client) *cobra.Command {
	var (
		channelID string
		dryRun    bool
		delay     time.Duration
	)

	cmd := &cobra.Command{
		Use:   "clear-history",
		Short: "Delete bot messages from a Slack channel",
		Long:  "Delete all messages sent by the bot in a specific channel. Only bot-owned messages can be deleted.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()

			found, deleted, errors, err := clearHistory(ctx, getClient(), channelID, dryRun, delay, os.Stdout)
			if err != nil {
				return err
			}

			if dryRun {
				fmt.Fprintf(os.Stdout, "Dry run: found %d bot messages\n", found)
			} else {
				fmt.Fprintf(os.Stdout, "Deleted %d of %d bot messages", deleted, found)
				if errors > 0 {
					fmt.Fprintf(os.Stdout, " (%d errors)", errors)
				}
				fmt.Fprintln(os.Stdout)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&channelID, "channel", "c", "", "Slack channel/DM ID")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "List what would be deleted without deleting")
	cmd.Flags().DurationVar(&delay, "delay", 1200*time.Millisecond, "Pause between delete calls for rate limiting")
	_ = cmd.MarkFlagRequired("channel")

	return cmd
}

func clearHistory(
	ctx context.Context,
	api slackAPI,
	channelID string,
	dryRun bool,
	delay time.Duration,
	w io.Writer,
) (found, deleted, errors int, err error) {
	// Get bot identity
	authResp, err := api.AuthTest()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("auth test failed: %w", err)
	}
	botUserID := authResp.UserID
	botID := authResp.BotID

	// Collect bot message timestamps
	var timestamps []string
	cursor := ""

	for {
		resp, err := api.GetConversationHistory(&slack.GetConversationHistoryParameters{
			ChannelID: channelID,
			Limit:     200,
			Cursor:    cursor,
		})
		if err != nil {
			return 0, 0, 0, fmt.Errorf("error getting conversation history: %w", err)
		}

		for _, msg := range resp.Messages {
			if msg.User == botUserID || msg.BotID == botID {
				timestamps = append(timestamps, msg.Timestamp)
			}
		}

		if !resp.HasMore || resp.ResponseMetaData.NextCursor == "" {
			break
		}
		cursor = resp.ResponseMetaData.NextCursor
	}

	found = len(timestamps)

	if dryRun || found == 0 {
		return found, 0, 0, nil
	}

	// Delete messages
	for i, ts := range timestamps {
		if err := ctx.Err(); err != nil {
			fmt.Fprintf(w, "Interrupted after %d deletions\n", deleted)
			return found, deleted, errors, nil
		}

		fmt.Fprintf(w, "\rDeleting message %d/%d...", i+1, found)

		_, _, deleteErr := api.DeleteMessage(channelID, ts)
		if deleteErr != nil {
			if rateLimitErr, ok := deleteErr.(*slack.RateLimitedError); ok {
				fmt.Fprintf(w, "Rate limited, waiting %s...\n", rateLimitErr.RetryAfter)
				select {
				case <-time.After(rateLimitErr.RetryAfter):
				case <-ctx.Done():
					return found, deleted, errors, nil
				}
				// Retry the same message
				_, _, deleteErr = api.DeleteMessage(channelID, ts)
			}

			if deleteErr != nil {
				errors++
				fmt.Fprintf(w, "Error deleting message %s: %v\n", ts, deleteErr)
				continue
			}
		}

		deleted++

		// Delay between deletes (except after last one)
		if i < len(timestamps)-1 && delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				fmt.Fprintf(w, "Interrupted after %d deletions\n", deleted)
				return found, deleted, errors, nil
			}
		}
	}

	return found, deleted, errors, nil
}
