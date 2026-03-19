package slackcmd

import (
	"bytes"
	"testing"

	"github.com/slack-go/slack"
)

type mockListAPI struct {
	conversations [][]slack.Channel // pages of results
	users         *[]slack.User
	convErr       error
	usersErr      error
}

func (m *mockListAPI) GetConversationsForUser(
	params *slack.GetConversationsForUserParameters,
) ([]slack.Channel, string, error) {
	if m.convErr != nil {
		return nil, "", m.convErr
	}
	if len(m.conversations) == 0 {
		return nil, "", nil
	}

	// Determine which page to return based on cursor
	page := 0
	if params.Cursor == "page2" {
		page = 1
	}
	if page >= len(m.conversations) {
		return nil, "", nil
	}

	nextCursor := ""
	if page+1 < len(m.conversations) {
		nextCursor = "page2"
	}
	return m.conversations[page], nextCursor, nil
}

func (m *mockListAPI) GetUsersInfo(users ...string) (*[]slack.User, error) {
	if m.usersErr != nil {
		return nil, m.usersErr
	}
	return m.users, nil
}

func TestListChannels_ShowsDMs(t *testing.T) {
	api := &mockListAPI{
		conversations: [][]slack.Channel{
			{
				{
					GroupConversation: slack.GroupConversation{
						Conversation: slack.Conversation{ID: "D001", User: "U001"},
					},
				},
				{
					GroupConversation: slack.GroupConversation{
						Conversation: slack.Conversation{ID: "D002", User: "U002"},
					},
				},
			},
		},
		users: &[]slack.User{
			{ID: "U001", RealName: "Alice"},
			{ID: "U002", RealName: "Bob"},
		},
	}

	var buf bytes.Buffer
	err := listChannels(api, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	for _, want := range []string{"D001", "D002", "Alice", "Bob", "U001", "U002", "CHANNEL ID"} {
		if !bytes.Contains([]byte(output), []byte(want)) {
			t.Errorf("output missing %q:\n%s", want, output)
		}
	}
}

func TestListChannels_Empty(t *testing.T) {
	api := &mockListAPI{}

	var buf bytes.Buffer
	err := listChannels(api, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := "No DM channels found."; !bytes.Contains(buf.Bytes(), []byte(want)) {
		t.Errorf("expected %q, got: %s", want, buf.String())
	}
}

func TestListChannels_Pagination(t *testing.T) {
	api := &mockListAPI{
		conversations: [][]slack.Channel{
			{
				{
					GroupConversation: slack.GroupConversation{
						Conversation: slack.Conversation{ID: "D001", User: "U001"},
					},
				},
			},
			{
				{
					GroupConversation: slack.GroupConversation{
						Conversation: slack.Conversation{ID: "D002", User: "U002"},
					},
				},
			},
		},
		users: &[]slack.User{
			{ID: "U001", Name: "alice"},
			{ID: "U002", Name: "bob"},
		},
	}

	var buf bytes.Buffer
	err := listChannels(api, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("D001")) || !bytes.Contains([]byte(output), []byte("D002")) {
		t.Errorf("expected both channels in output:\n%s", output)
	}
}
