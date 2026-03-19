package slackcmd

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/slack-go/slack"
)

type mockSlackAPI struct {
	authResp *slack.AuthTestResponse
	authErr  error

	// Pages of history responses
	historyPages []*slack.GetConversationHistoryResponse
	historyErr   error
	historyPage  int

	deletedTimestamps []string
	deleteErrors      map[string]error
}

func (m *mockSlackAPI) AuthTest() (*slack.AuthTestResponse, error) {
	return m.authResp, m.authErr
}

func (m *mockSlackAPI) GetConversationHistory(
	params *slack.GetConversationHistoryParameters,
) (*slack.GetConversationHistoryResponse, error) {
	if m.historyErr != nil {
		return nil, m.historyErr
	}
	if m.historyPage >= len(m.historyPages) {
		return &slack.GetConversationHistoryResponse{}, nil
	}
	resp := m.historyPages[m.historyPage]
	m.historyPage++
	return resp, nil
}

func (m *mockSlackAPI) DeleteMessage(channel, messageTimestamp string) (string, string, error) {
	if m.deleteErrors != nil {
		if err, ok := m.deleteErrors[messageTimestamp]; ok {
			return "", "", err
		}
	}
	m.deletedTimestamps = append(m.deletedTimestamps, messageTimestamp)
	return channel, messageTimestamp, nil
}

func newAuthResp(userID, botID string) *slack.AuthTestResponse {
	return &slack.AuthTestResponse{UserID: userID, BotID: botID}
}

func TestClearHistory_DryRun(t *testing.T) {
	api := &mockSlackAPI{
		authResp: newAuthResp("U_BOT", "B_BOT"),
		historyPages: []*slack.GetConversationHistoryResponse{
			{
				Messages: []slack.Message{
					{Msg: slack.Msg{User: "U_BOT", Timestamp: "1.1"}},
					{Msg: slack.Msg{User: "U_OTHER", Timestamp: "1.2"}},
					{Msg: slack.Msg{User: "U_BOT", Timestamp: "1.3"}},
				},
			},
		},
	}

	var buf bytes.Buffer
	found, deleted, errors, err := clearHistory(context.Background(), api, "C001", true, 0, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != 2 {
		t.Errorf("expected 2 found, got %d", found)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted in dry run, got %d", deleted)
	}
	if errors != 0 {
		t.Errorf("expected 0 errors, got %d", errors)
	}
	if len(api.deletedTimestamps) != 0 {
		t.Errorf("dry run should not delete, but deleted %v", api.deletedTimestamps)
	}
}

func TestClearHistory_DeletesOnlyBotMessages(t *testing.T) {
	api := &mockSlackAPI{
		authResp: newAuthResp("U_BOT", "B_BOT"),
		historyPages: []*slack.GetConversationHistoryResponse{
			{
				Messages: []slack.Message{
					{Msg: slack.Msg{User: "U_BOT", Timestamp: "1.1"}},
					{Msg: slack.Msg{User: "U_HUMAN", Timestamp: "1.2"}},
					{Msg: slack.Msg{BotID: "B_BOT", Timestamp: "1.3"}},
					{Msg: slack.Msg{User: "U_OTHER", Timestamp: "1.4"}},
				},
			},
		},
	}

	var buf bytes.Buffer
	found, deleted, errors, err := clearHistory(context.Background(), api, "C001", false, 0, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != 2 {
		t.Errorf("expected 2 found, got %d", found)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}
	if errors != 0 {
		t.Errorf("expected 0 errors, got %d", errors)
	}
	if len(api.deletedTimestamps) != 2 {
		t.Fatalf("expected 2 deletions, got %d", len(api.deletedTimestamps))
	}
	if api.deletedTimestamps[0] != "1.1" || api.deletedTimestamps[1] != "1.3" {
		t.Errorf("unexpected deletions: %v", api.deletedTimestamps)
	}
}

func TestClearHistory_Pagination(t *testing.T) {
	api := &mockSlackAPI{
		authResp: newAuthResp("U_BOT", "B_BOT"),
		historyPages: []*slack.GetConversationHistoryResponse{
			{
				HasMore: true,
				Messages: []slack.Message{
					{Msg: slack.Msg{User: "U_BOT", Timestamp: "1.1"}},
				},
				ResponseMetaData: struct {
					NextCursor string `json:"next_cursor"`
				}{NextCursor: "cursor2"},
			},
			{
				Messages: []slack.Message{
					{Msg: slack.Msg{User: "U_BOT", Timestamp: "2.1"}},
				},
			},
		},
	}

	var buf bytes.Buffer
	found, deleted, _, err := clearHistory(context.Background(), api, "C001", false, 0, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != 2 || deleted != 2 {
		t.Errorf("expected 2 found and deleted, got found=%d deleted=%d", found, deleted)
	}
}

func TestClearHistory_EmptyChannel(t *testing.T) {
	api := &mockSlackAPI{
		authResp:     newAuthResp("U_BOT", "B_BOT"),
		historyPages: []*slack.GetConversationHistoryResponse{{Messages: []slack.Message{}}},
	}

	var buf bytes.Buffer
	found, deleted, errors, err := clearHistory(context.Background(), api, "C001", false, 0, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != 0 || deleted != 0 || errors != 0 {
		t.Errorf("expected all zeros, got found=%d deleted=%d errors=%d", found, deleted, errors)
	}
}

func TestClearHistory_DeleteError(t *testing.T) {
	api := &mockSlackAPI{
		authResp: newAuthResp("U_BOT", "B_BOT"),
		historyPages: []*slack.GetConversationHistoryResponse{
			{
				Messages: []slack.Message{
					{Msg: slack.Msg{User: "U_BOT", Timestamp: "1.1"}},
					{Msg: slack.Msg{User: "U_BOT", Timestamp: "1.2"}},
					{Msg: slack.Msg{User: "U_BOT", Timestamp: "1.3"}},
				},
			},
		},
		deleteErrors: map[string]error{
			"1.2": fmt.Errorf("message_not_found"),
		},
	}

	var buf bytes.Buffer
	found, deleted, errors, err := clearHistory(context.Background(), api, "C001", false, 0, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != 3 {
		t.Errorf("expected 3 found, got %d", found)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}
	if errors != 1 {
		t.Errorf("expected 1 error, got %d", errors)
	}
}

func TestClearHistory_RateLimitRetry(t *testing.T) {
	rateLimitErr := &slack.RateLimitedError{RetryAfter: 1 * time.Millisecond}
	callCount := 0

	api := &mockSlackAPI{
		authResp: newAuthResp("U_BOT", "B_BOT"),
		historyPages: []*slack.GetConversationHistoryResponse{
			{
				Messages: []slack.Message{
					{Msg: slack.Msg{User: "U_BOT", Timestamp: "1.1"}},
				},
			},
		},
		deleteErrors: map[string]error{
			"1.1": rateLimitErr,
		},
	}

	// Override DeleteMessage to succeed on retry
	originalDelete := api.DeleteMessage
	_ = originalDelete
	// We need a custom mock for this test since the map-based mock always returns the error
	retryAPI := &retryMockAPI{
		mockSlackAPI: api,
		failFirst:    true,
		retryAfter:   1 * time.Millisecond,
		callCount:    &callCount,
	}

	var buf bytes.Buffer
	found, deleted, errors, err := clearHistory(context.Background(), retryAPI, "C001", false, 0, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != 1 {
		t.Errorf("expected 1 found, got %d", found)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted after retry, got %d", deleted)
	}
	if errors != 0 {
		t.Errorf("expected 0 errors after retry, got %d", errors)
	}
	if callCount != 2 {
		t.Errorf("expected 2 delete calls (initial + retry), got %d", callCount)
	}
}

// retryMockAPI wraps mockSlackAPI to simulate rate limiting then success on retry.
type retryMockAPI struct {
	*mockSlackAPI
	failFirst  bool
	retryAfter time.Duration
	callCount  *int
}

func (r *retryMockAPI) DeleteMessage(channel, messageTimestamp string) (string, string, error) {
	*r.callCount++
	if r.failFirst && *r.callCount == 1 {
		return "", "", &slack.RateLimitedError{RetryAfter: r.retryAfter}
	}
	return channel, messageTimestamp, nil
}
