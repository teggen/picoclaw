package channels

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
)

func TestBaseChannelIsAllowed(t *testing.T) {
	tests := []struct {
		name      string
		allowList []string
		senderID  string
		want      bool
	}{
		{
			name:      "empty allowlist allows all",
			allowList: nil,
			senderID:  "anyone",
			want:      true,
		},
		{
			name:      "compound sender matches numeric allowlist",
			allowList: []string{"123456"},
			senderID:  "123456|alice",
			want:      true,
		},
		{
			name:      "compound sender matches username allowlist",
			allowList: []string{"@alice"},
			senderID:  "123456|alice",
			want:      true,
		},
		{
			name:      "numeric sender matches legacy compound allowlist",
			allowList: []string{"123456|alice"},
			senderID:  "123456",
			want:      true,
		},
		{
			name:      "non matching sender is denied",
			allowList: []string{"123456"},
			senderID:  "654321|bob",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := NewBaseChannel("test", nil, nil, tt.allowList)
			if got := ch.IsAllowed(tt.senderID); got != tt.want {
				t.Fatalf("IsAllowed(%q) = %v, want %v", tt.senderID, got, tt.want)
			}
		})
	}
}

func TestShouldRespondInGroup(t *testing.T) {
	tests := []struct {
		name        string
		gt          config.GroupTriggerConfig
		isMentioned bool
		content     string
		wantRespond bool
		wantContent string
	}{
		{
			name:        "no config - permissive default",
			gt:          config.GroupTriggerConfig{},
			isMentioned: false,
			content:     "hello world",
			wantRespond: true,
			wantContent: "hello world",
		},
		{
			name:        "no config - mentioned",
			gt:          config.GroupTriggerConfig{},
			isMentioned: true,
			content:     "hello world",
			wantRespond: true,
			wantContent: "hello world",
		},
		{
			name:        "mention_only - not mentioned",
			gt:          config.GroupTriggerConfig{MentionOnly: true},
			isMentioned: false,
			content:     "hello world",
			wantRespond: false,
			wantContent: "hello world",
		},
		{
			name:        "mention_only - mentioned",
			gt:          config.GroupTriggerConfig{MentionOnly: true},
			isMentioned: true,
			content:     "hello world",
			wantRespond: true,
			wantContent: "hello world",
		},
		{
			name:        "prefix match",
			gt:          config.GroupTriggerConfig{Prefixes: []string{"/ask"}},
			isMentioned: false,
			content:     "/ask hello",
			wantRespond: true,
			wantContent: "hello",
		},
		{
			name:        "prefix no match - not mentioned",
			gt:          config.GroupTriggerConfig{Prefixes: []string{"/ask"}},
			isMentioned: false,
			content:     "hello world",
			wantRespond: false,
			wantContent: "hello world",
		},
		{
			name:        "prefix no match - but mentioned",
			gt:          config.GroupTriggerConfig{Prefixes: []string{"/ask"}},
			isMentioned: true,
			content:     "hello world",
			wantRespond: true,
			wantContent: "hello world",
		},
		{
			name:        "multiple prefixes - second matches",
			gt:          config.GroupTriggerConfig{Prefixes: []string{"/ask", "/bot"}},
			isMentioned: false,
			content:     "/bot help me",
			wantRespond: true,
			wantContent: "help me",
		},
		{
			name:        "mention_only with prefixes - mentioned overrides",
			gt:          config.GroupTriggerConfig{MentionOnly: true, Prefixes: []string{"/ask"}},
			isMentioned: true,
			content:     "hello",
			wantRespond: true,
			wantContent: "hello",
		},
		{
			name:        "mention_only with prefixes - not mentioned, no prefix",
			gt:          config.GroupTriggerConfig{MentionOnly: true, Prefixes: []string{"/ask"}},
			isMentioned: false,
			content:     "hello",
			wantRespond: false,
			wantContent: "hello",
		},
		{
			name:        "empty prefix in list is skipped",
			gt:          config.GroupTriggerConfig{Prefixes: []string{"", "/ask"}},
			isMentioned: false,
			content:     "/ask test",
			wantRespond: true,
			wantContent: "test",
		},
		{
			name:        "prefix strips leading whitespace after prefix",
			gt:          config.GroupTriggerConfig{Prefixes: []string{"/ask "}},
			isMentioned: false,
			content:     "/ask hello",
			wantRespond: true,
			wantContent: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := NewBaseChannel("test", nil, nil, nil, WithGroupTrigger(tt.gt))
			gotRespond, gotContent := ch.ShouldRespondInGroup(tt.isMentioned, tt.content)
			if gotRespond != tt.wantRespond {
				t.Errorf("ShouldRespondInGroup() respond = %v, want %v", gotRespond, tt.wantRespond)
			}
			if gotContent != tt.wantContent {
				t.Errorf("ShouldRespondInGroup() content = %q, want %q", gotContent, tt.wantContent)
			}
		})
	}
}

func TestIsAllowedSender(t *testing.T) {
	tests := []struct {
		name      string
		allowList []string
		sender    bus.SenderInfo
		want      bool
	}{
		{
			name:      "empty allowlist allows all",
			allowList: nil,
			sender:    bus.SenderInfo{PlatformID: "anyone"},
			want:      true,
		},
		{
			name:      "numeric ID matches PlatformID",
			allowList: []string{"123456"},
			sender: bus.SenderInfo{
				Platform:    "telegram",
				PlatformID:  "123456",
				CanonicalID: "telegram:123456",
			},
			want: true,
		},
		{
			name:      "canonical format matches",
			allowList: []string{"telegram:123456"},
			sender: bus.SenderInfo{
				Platform:    "telegram",
				PlatformID:  "123456",
				CanonicalID: "telegram:123456",
			},
			want: true,
		},
		{
			name:      "canonical format wrong platform",
			allowList: []string{"discord:123456"},
			sender: bus.SenderInfo{
				Platform:    "telegram",
				PlatformID:  "123456",
				CanonicalID: "telegram:123456",
			},
			want: false,
		},
		{
			name:      "@username matches",
			allowList: []string{"@alice"},
			sender: bus.SenderInfo{
				Platform:    "telegram",
				PlatformID:  "123456",
				CanonicalID: "telegram:123456",
				Username:    "alice",
			},
			want: true,
		},
		{
			name:      "compound id|username matches by ID",
			allowList: []string{"123456|alice"},
			sender: bus.SenderInfo{
				Platform:    "telegram",
				PlatformID:  "123456",
				CanonicalID: "telegram:123456",
				Username:    "alice",
			},
			want: true,
		},
		{
			name:      "non matching sender denied",
			allowList: []string{"654321"},
			sender: bus.SenderInfo{
				Platform:    "telegram",
				PlatformID:  "123456",
				CanonicalID: "telegram:123456",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := NewBaseChannel("test", nil, nil, tt.allowList)
			if got := ch.IsAllowedSender(tt.sender); got != tt.want {
				t.Fatalf("IsAllowedSender(%+v) = %v, want %v", tt.sender, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Mock types for HandleMessage tests
// ---------------------------------------------------------------------------

// mockPlaceholderRecorder tracks calls from HandleMessage.
type mockPlaceholderRecorder struct {
	placeholders  []struct{ channel, chatID, phID string }
	typingStops   []struct{ channel, chatID string }
	reactionUndos []struct{ channel, chatID string }
}

func (r *mockPlaceholderRecorder) RecordPlaceholder(channel, chatID, phID string) {
	r.placeholders = append(r.placeholders, struct{ channel, chatID, phID string }{channel, chatID, phID})
}

func (r *mockPlaceholderRecorder) RecordTypingStop(channel, chatID string, stop func()) {
	r.typingStops = append(r.typingStops, struct{ channel, chatID string }{channel, chatID})
}

func (r *mockPlaceholderRecorder) RecordReactionUndo(channel, chatID string, undo func()) {
	r.reactionUndos = append(r.reactionUndos, struct{ channel, chatID string }{channel, chatID})
}

// typingOwner implements Channel + TypingCapable
type typingOwner struct {
	BaseChannel
	typingStarted atomic.Bool
}

func (o *typingOwner) Start(_ context.Context) error                       { return nil }
func (o *typingOwner) Stop(_ context.Context) error                        { return nil }
func (o *typingOwner) Send(_ context.Context, _ bus.OutboundMessage) error { return nil }
func (o *typingOwner) StartTyping(_ context.Context, _ string) (func(), error) {
	o.typingStarted.Store(true)
	return func() {}, nil
}

// reactionOwner implements Channel + ReactionCapable
type reactionOwner struct {
	BaseChannel
	reacted atomic.Bool
}

func (o *reactionOwner) Start(_ context.Context) error                       { return nil }
func (o *reactionOwner) Stop(_ context.Context) error                        { return nil }
func (o *reactionOwner) Send(_ context.Context, _ bus.OutboundMessage) error { return nil }
func (o *reactionOwner) ReactToMessage(_ context.Context, _, _ string) (func(), error) {
	o.reacted.Store(true)
	return func() {}, nil
}

// placeholderOwner implements Channel + PlaceholderCapable
type placeholderOwner struct {
	BaseChannel
	phSent atomic.Bool
}

func (o *placeholderOwner) Start(_ context.Context) error                       { return nil }
func (o *placeholderOwner) Stop(_ context.Context) error                        { return nil }
func (o *placeholderOwner) Send(_ context.Context, _ bus.OutboundMessage) error { return nil }
func (o *placeholderOwner) SendPlaceholder(_ context.Context, _ string) (string, error) {
	o.phSent.Store(true)
	return "ph-123", nil
}

// ---------------------------------------------------------------------------
// Section 6: HandleMessage tests
// ---------------------------------------------------------------------------

func TestHandleMessage_AllowedSender_Publishes(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ch := NewBaseChannel("test", nil, mb, nil) // nil allowlist = allow all

	go ch.HandleMessage(context.Background(), bus.Peer{}, "msg1", "user1", "chat1", "hello", nil, nil)

	select {
	case msg := <-mb.InboundChan():
		assert.Equal(t, "test", msg.Channel)
		assert.Equal(t, "user1", msg.SenderID)
		assert.Equal(t, "chat1", msg.ChatID)
		assert.Equal(t, "hello", msg.Content)
	case <-time.After(2 * time.Second):
		t.Fatal("expected message to be published")
	}
}

func TestHandleMessage_DeniedSender_NoPublish(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ch := NewBaseChannel("test", nil, mb, []string{"allowed_user"})

	ch.HandleMessage(context.Background(), bus.Peer{}, "msg1", "denied_user", "chat1", "hello", nil, nil)

	select {
	case <-mb.InboundChan():
		t.Fatal("message should not have been published for denied sender")
	case <-time.After(100 * time.Millisecond):
		// success — no message published
	}
}

func TestHandleMessage_DeniedSenderInfo_NoPublish(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ch := NewBaseChannel("test", nil, mb, []string{"allowed_user"})

	sender := bus.SenderInfo{
		Platform:    "telegram",
		PlatformID:  "999",
		CanonicalID: "telegram:999",
	}
	ch.HandleMessage(context.Background(), bus.Peer{}, "msg1", "999", "chat1", "hello", nil, nil, sender)

	select {
	case <-mb.InboundChan():
		t.Fatal("message should not have been published for denied sender")
	case <-time.After(100 * time.Millisecond):
		// success
	}
}

func TestHandleMessage_SenderInfoCanonicalID(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ch := NewBaseChannel("test", nil, mb, nil)

	sender := bus.SenderInfo{
		Platform:    "telegram",
		PlatformID:  "123",
		CanonicalID: "telegram:123",
	}

	go ch.HandleMessage(context.Background(), bus.Peer{}, "msg1", "raw_id", "chat1", "hello", nil, nil, sender)

	select {
	case msg := <-mb.InboundChan():
		assert.Equal(t, "telegram:123", msg.SenderID, "should use CanonicalID when available")
	case <-time.After(2 * time.Second):
		t.Fatal("expected message to be published")
	}
}

func TestHandleMessage_TriggersTyping(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	recorder := &mockPlaceholderRecorder{}
	owner := &typingOwner{}

	ch := NewBaseChannel("test", nil, mb, nil)
	ch.SetOwner(owner)
	ch.SetPlaceholderRecorder(recorder)

	go ch.HandleMessage(context.Background(), bus.Peer{}, "msg1", "user1", "chat1", "hello", nil, nil)

	select {
	case <-mb.InboundChan():
		// wait for message to be processed
	case <-time.After(2 * time.Second):
		t.Fatal("expected message to be published")
	}

	assert.True(t, owner.typingStarted.Load(), "typing should have been started")
	require.Len(t, recorder.typingStops, 1, "should have recorded typing stop")
	assert.Equal(t, "test", recorder.typingStops[0].channel)
	assert.Equal(t, "chat1", recorder.typingStops[0].chatID)
}

func TestHandleMessage_TriggersReaction(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	recorder := &mockPlaceholderRecorder{}
	owner := &reactionOwner{}

	ch := NewBaseChannel("test", nil, mb, nil)
	ch.SetOwner(owner)
	ch.SetPlaceholderRecorder(recorder)

	go ch.HandleMessage(context.Background(), bus.Peer{}, "msg1", "user1", "chat1", "hello", nil, nil)

	select {
	case <-mb.InboundChan():
	case <-time.After(2 * time.Second):
		t.Fatal("expected message to be published")
	}

	assert.True(t, owner.reacted.Load(), "reaction should have been triggered")
	require.Len(t, recorder.reactionUndos, 1, "should have recorded reaction undo")
}

func TestHandleMessage_TriggersPlaceholder(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	recorder := &mockPlaceholderRecorder{}
	owner := &placeholderOwner{}

	ch := NewBaseChannel("test", nil, mb, nil)
	ch.SetOwner(owner)
	ch.SetPlaceholderRecorder(recorder)

	go ch.HandleMessage(context.Background(), bus.Peer{}, "msg1", "user1", "chat1", "hello", nil, nil)

	select {
	case <-mb.InboundChan():
	case <-time.After(2 * time.Second):
		t.Fatal("expected message to be published")
	}

	assert.True(t, owner.phSent.Load(), "placeholder should have been sent")
	require.Len(t, recorder.placeholders, 1, "should have recorded placeholder")
	assert.Equal(t, "ph-123", recorder.placeholders[0].phID)
}

func TestHandleMessage_AudioSkipsPlaceholder(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	recorder := &mockPlaceholderRecorder{}
	owner := &placeholderOwner{}

	ch := NewBaseChannel("test", nil, mb, nil)
	ch.SetOwner(owner)
	ch.SetPlaceholderRecorder(recorder)

	go ch.HandleMessage(context.Background(), bus.Peer{}, "msg1", "user1", "chat1", "[voice] some audio", nil, nil)

	select {
	case <-mb.InboundChan():
	case <-time.After(2 * time.Second):
		t.Fatal("expected message to be published")
	}

	assert.False(t, owner.phSent.Load(), "placeholder should NOT be sent for audio content")
	assert.Empty(t, recorder.placeholders, "no placeholder should be recorded for audio")
}

func TestHandleMessage_NoOwner_NoCapabilityChecks(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ch := NewBaseChannel("test", nil, mb, nil)
	// owner is nil by default — no SetOwner call

	go ch.HandleMessage(context.Background(), bus.Peer{}, "msg1", "user1", "chat1", "hello", nil, nil)

	select {
	case msg := <-mb.InboundChan():
		assert.Equal(t, "hello", msg.Content)
	case <-time.After(2 * time.Second):
		t.Fatal("expected message to be published even without owner")
	}
}

// ---------------------------------------------------------------------------
// Section 7: Trivial accessors
// ---------------------------------------------------------------------------

func TestSetRunning_IsRunning(t *testing.T) {
	ch := NewBaseChannel("test", nil, nil, nil)
	assert.False(t, ch.IsRunning())
	ch.SetRunning(true)
	assert.True(t, ch.IsRunning())
	ch.SetRunning(false)
	assert.False(t, ch.IsRunning())
}

func TestName(t *testing.T) {
	ch := NewBaseChannel("myChannel", nil, nil, nil)
	assert.Equal(t, "myChannel", ch.Name())
}

func TestReasoningChannelID(t *testing.T) {
	ch := NewBaseChannel("test", nil, nil, nil, WithReasoningChannelID("reason-1"))
	assert.Equal(t, "reason-1", ch.ReasoningChannelID())
}

func TestReasoningChannelID_Default(t *testing.T) {
	ch := NewBaseChannel("test", nil, nil, nil)
	assert.Equal(t, "", ch.ReasoningChannelID())
}

func TestSetGetMediaStore(t *testing.T) {
	ch := NewBaseChannel("test", nil, nil, nil)
	assert.Nil(t, ch.GetMediaStore())
	// We can't easily create a real MediaStore, so just verify nil → nil round-trip
	// and that the setter/getter don't panic
	ch.SetMediaStore(nil)
	assert.Nil(t, ch.GetMediaStore())
}

func TestSetGetPlaceholderRecorder(t *testing.T) {
	ch := NewBaseChannel("test", nil, nil, nil)
	assert.Nil(t, ch.GetPlaceholderRecorder())
	recorder := &mockPlaceholderRecorder{}
	ch.SetPlaceholderRecorder(recorder)
	assert.Equal(t, recorder, ch.GetPlaceholderRecorder())
}
