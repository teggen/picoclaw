package slackcmd

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/slack-go/slack"
	"github.com/spf13/cobra"
)

type listChannelsAPI interface {
	GetConversationsForUser(params *slack.GetConversationsForUserParameters) ([]slack.Channel, string, error)
	GetUsersInfo(users ...string) (*[]slack.User, error)
}

func newListChannelsCommand(getClient func() *slack.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "list-channels",
		Short: "List DM channels the bot participates in",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return listChannels(getClient(), os.Stdout)
		},
	}
}

func listChannels(api listChannelsAPI, w io.Writer) error {
	var allChannels []slack.Channel
	cursor := ""

	for {
		channels, nextCursor, err := api.GetConversationsForUser(&slack.GetConversationsForUserParameters{
			Types:  []string{"im"},
			Cursor: cursor,
			Limit:  200,
		})
		if err != nil {
			return fmt.Errorf("error listing conversations: %w", err)
		}
		allChannels = append(allChannels, channels...)
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	if len(allChannels) == 0 {
		fmt.Fprintln(w, "No DM channels found.")
		return nil
	}

	// Collect unique user IDs
	userIDs := make([]string, 0, len(allChannels))
	for _, ch := range allChannels {
		if ch.User != "" {
			userIDs = append(userIDs, ch.User)
		}
	}

	// Resolve user names
	nameMap := make(map[string]string)
	if len(userIDs) > 0 {
		users, err := api.GetUsersInfo(userIDs...)
		if err == nil && users != nil {
			for i := range *users {
				u := (*users)[i]
				name := u.RealName
				if name == "" {
					name = u.Name
				}
				nameMap[u.ID] = name
			}
		}
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "CHANNEL ID\tUSER\tUSER ID")
	for _, ch := range allChannels {
		name := nameMap[ch.User]
		if name == "" {
			name = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", ch.ID, name, ch.User)
	}
	return tw.Flush()
}
