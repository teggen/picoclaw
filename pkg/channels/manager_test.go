package channels

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
)

// mockChannel is a test double that delegates Send to a configurable function.
type mockChannel struct {
	BaseChannel
	sendFn            func(ctx context.Context, msg bus.OutboundMessage) error
	sentMessages      []bus.OutboundMessage
	placeholdersSent  int
	editedMessages    int
	lastPlaceholderID string
}

func (m *mockChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	m.sentMessages = append(m.sentMessages, msg)
	return m.sendFn(ctx, msg)
}

func (m *mockChannel) Start(ctx context.Context) error { return nil }
func (m *mockChannel) Stop(ctx context.Context) error  { return nil }

func (m *mockChannel) SendPlaceholder(ctx context.Context, chatID string) (string, error) {
	m.placeholdersSent++
	m.lastPlaceholderID = "mock-ph-123"
	return m.lastPlaceholderID, nil
}

func (m *mockChannel) EditMessage(ctx context.Context, chatID, messageID, content string) error {
	m.editedMessages++
	return nil
}

type mockMediaChannel struct {
	mockChannel
	sendMediaFn       func(ctx context.Context, msg bus.OutboundMediaMessage) error
	sentMediaMessages []bus.OutboundMediaMessage
}

func (m *mockMediaChannel) SendMedia(ctx context.Context, msg bus.OutboundMediaMessage) error {
	m.sentMediaMessages = append(m.sentMediaMessages, msg)
	if m.sendMediaFn != nil {
		return m.sendMediaFn(ctx, msg)
	}
	return nil
}

type mockDeletingMediaChannel struct {
	mockMediaChannel
	deleteCalls int
	lastDeleted struct {
		chatID    string
		messageID string
	}
}

func (m *mockDeletingMediaChannel) DeleteMessage(
	_ context.Context,
	chatID string,
	messageID string,
) error {
	m.deleteCalls++
	m.lastDeleted.chatID = chatID
	m.lastDeleted.messageID = messageID
	return nil
}

// newTestManager creates a minimal Manager suitable for unit tests.
func newTestManager() *Manager {
	return &Manager{
		channels: make(map[string]Channel),
		workers:  make(map[string]*channelWorker),
	}
}

func TestSendWithRetry_Success(t *testing.T) {
	m := newTestManager()
	var callCount int
	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
			callCount++
			return nil
		},
	}
	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}

	ctx := context.Background()
	msg := bus.OutboundMessage{Channel: "test", ChatID: "1", Content: "hello"}

	m.sendWithRetry(ctx, "test", w, msg)

	if callCount != 1 {
		t.Fatalf("expected 1 Send call, got %d", callCount)
	}
}

func TestSendWithRetry_TemporaryThenSuccess(t *testing.T) {
	m := newTestManager()
	var callCount int
	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
			callCount++
			if callCount <= 2 {
				return fmt.Errorf("network error: %w", ErrTemporary)
			}
			return nil
		},
	}
	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}

	ctx := context.Background()
	msg := bus.OutboundMessage{Channel: "test", ChatID: "1", Content: "hello"}

	m.sendWithRetry(ctx, "test", w, msg)

	if callCount != 3 {
		t.Fatalf("expected 3 Send calls (2 failures + 1 success), got %d", callCount)
	}
}

func TestSendWithRetry_PermanentFailure(t *testing.T) {
	m := newTestManager()
	var callCount int
	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
			callCount++
			return fmt.Errorf("bad chat ID: %w", ErrSendFailed)
		},
	}
	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}

	ctx := context.Background()
	msg := bus.OutboundMessage{Channel: "test", ChatID: "1", Content: "hello"}

	m.sendWithRetry(ctx, "test", w, msg)

	if callCount != 1 {
		t.Fatalf("expected 1 Send call (no retry for permanent failure), got %d", callCount)
	}
}

func TestSendWithRetry_NotRunning(t *testing.T) {
	m := newTestManager()
	var callCount int
	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
			callCount++
			return ErrNotRunning
		},
	}
	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}

	ctx := context.Background()
	msg := bus.OutboundMessage{Channel: "test", ChatID: "1", Content: "hello"}

	m.sendWithRetry(ctx, "test", w, msg)

	if callCount != 1 {
		t.Fatalf("expected 1 Send call (no retry for ErrNotRunning), got %d", callCount)
	}
}

func TestSendWithRetry_RateLimitRetry(t *testing.T) {
	m := newTestManager()
	var callCount int
	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
			callCount++
			if callCount == 1 {
				return fmt.Errorf("429: %w", ErrRateLimit)
			}
			return nil
		},
	}
	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}

	ctx := context.Background()
	msg := bus.OutboundMessage{Channel: "test", ChatID: "1", Content: "hello"}

	start := time.Now()
	m.sendWithRetry(ctx, "test", w, msg)
	elapsed := time.Since(start)

	if callCount != 2 {
		t.Fatalf("expected 2 Send calls (1 rate limit + 1 success), got %d", callCount)
	}
	// Should have waited at least rateLimitDelay (1s) but allow some slack
	if elapsed < 900*time.Millisecond {
		t.Fatalf("expected at least ~1s delay for rate limit retry, got %v", elapsed)
	}
}

func TestSendWithRetry_MaxRetriesExhausted(t *testing.T) {
	m := newTestManager()
	var callCount int
	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
			callCount++
			return fmt.Errorf("timeout: %w", ErrTemporary)
		},
	}
	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}

	ctx := context.Background()
	msg := bus.OutboundMessage{Channel: "test", ChatID: "1", Content: "hello"}

	m.sendWithRetry(ctx, "test", w, msg)

	expected := maxRetries + 1 // initial attempt + maxRetries retries
	if callCount != expected {
		t.Fatalf("expected %d Send calls, got %d", expected, callCount)
	}
}

func TestSendMedia_Success(t *testing.T) {
	m := newTestManager()
	var callCount int
	ch := &mockMediaChannel{
		sendMediaFn: func(_ context.Context, _ bus.OutboundMediaMessage) error {
			callCount++
			return nil
		},
	}
	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}
	m.channels["test"] = ch
	m.workers["test"] = w

	err := m.SendMedia(context.Background(), bus.OutboundMediaMessage{
		Channel: "test",
		ChatID:  "chat1",
		Parts:   []bus.MediaPart{{Ref: "media://abc"}},
	})
	if err != nil {
		t.Fatalf("SendMedia() error = %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 SendMedia call, got %d", callCount)
	}
}

func TestSendMedia_PropagatesFailure(t *testing.T) {
	m := newTestManager()
	ch := &mockMediaChannel{
		sendMediaFn: func(_ context.Context, _ bus.OutboundMediaMessage) error {
			return fmt.Errorf("bad upload: %w", ErrSendFailed)
		},
	}
	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}
	m.channels["test"] = ch
	m.workers["test"] = w

	err := m.SendMedia(context.Background(), bus.OutboundMediaMessage{
		Channel: "test",
		ChatID:  "chat1",
		Parts:   []bus.MediaPart{{Ref: "media://abc"}},
	})
	if err == nil {
		t.Fatal("expected SendMedia to return error")
	}
	if !errors.Is(err, ErrSendFailed) {
		t.Fatalf("expected ErrSendFailed, got %v", err)
	}
}

func TestSendMedia_UnsupportedChannelReturnsError(t *testing.T) {
	m := newTestManager()
	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
			return nil
		},
	}
	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}
	m.channels["test"] = ch
	m.workers["test"] = w

	err := m.SendMedia(context.Background(), bus.OutboundMediaMessage{
		Channel: "test",
		ChatID:  "chat1",
		Parts:   []bus.MediaPart{{Ref: "media://abc"}},
	})
	if err == nil {
		t.Fatal("expected SendMedia to return error for unsupported channel")
	}
	if !strings.Contains(err.Error(), "does not support media sending") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendMedia_DeletesPlaceholderBeforeSending(t *testing.T) {
	m := newTestManager()
	ch := &mockDeletingMediaChannel{
		mockMediaChannel: mockMediaChannel{
			sendMediaFn: func(_ context.Context, _ bus.OutboundMediaMessage) error {
				return nil
			},
		},
	}
	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}
	m.channels["test"] = ch
	m.workers["test"] = w
	m.RecordPlaceholder("test", "chat1", "placeholder-1")

	err := m.SendMedia(context.Background(), bus.OutboundMediaMessage{
		Channel: "test",
		ChatID:  "chat1",
		Parts:   []bus.MediaPart{{Ref: "media://abc"}},
	})
	if err != nil {
		t.Fatalf("SendMedia() error = %v", err)
	}
	if ch.deleteCalls != 1 {
		t.Fatalf("expected placeholder delete to be called once, got %d", ch.deleteCalls)
	}
	if ch.lastDeleted.chatID != "chat1" || ch.lastDeleted.messageID != "placeholder-1" {
		t.Fatalf("unexpected placeholder deletion target: %+v", ch.lastDeleted)
	}
	if len(ch.sentMediaMessages) != 1 {
		t.Fatalf("expected media to be sent once, got %d", len(ch.sentMediaMessages))
	}
}

func TestSendWithRetry_UnknownError(t *testing.T) {
	m := newTestManager()
	var callCount int
	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
			callCount++
			if callCount == 1 {
				return errors.New("random unexpected error")
			}
			return nil
		},
	}
	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}

	ctx := context.Background()
	msg := bus.OutboundMessage{Channel: "test", ChatID: "1", Content: "hello"}

	m.sendWithRetry(ctx, "test", w, msg)

	if callCount != 2 {
		t.Fatalf("expected 2 Send calls (unknown error treated as temporary), got %d", callCount)
	}
}

func TestSendWithRetry_ContextCancelled(t *testing.T) {
	m := newTestManager()
	var callCount int
	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
			callCount++
			return fmt.Errorf("timeout: %w", ErrTemporary)
		},
	}
	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())
	msg := bus.OutboundMessage{Channel: "test", ChatID: "1", Content: "hello"}

	// Cancel context after first Send attempt returns
	ch.sendFn = func(_ context.Context, _ bus.OutboundMessage) error {
		callCount++
		cancel()
		return fmt.Errorf("timeout: %w", ErrTemporary)
	}

	m.sendWithRetry(ctx, "test", w, msg)

	// Should have called Send once, then noticed ctx canceled during backoff
	if callCount != 1 {
		t.Fatalf("expected 1 Send call before context cancellation, got %d", callCount)
	}
}

func TestWorkerRateLimiter(t *testing.T) {
	m := newTestManager()

	var mu sync.Mutex
	var sendTimes []time.Time

	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
			mu.Lock()
			sendTimes = append(sendTimes, time.Now())
			mu.Unlock()
			return nil
		},
	}

	// Create a worker with a low rate: 2 msg/s, burst 1
	w := &channelWorker{
		ch:      ch,
		queue:   make(chan bus.OutboundMessage, 10),
		done:    make(chan struct{}),
		limiter: rate.NewLimiter(2, 1),
	}

	ctx := t.Context()

	go m.runWorker(ctx, "test", w)

	// Enqueue 4 messages
	for i := range 4 {
		w.queue <- bus.OutboundMessage{Channel: "test", ChatID: "1", Content: fmt.Sprintf("msg%d", i)}
	}

	// Wait enough time for all messages to be sent (4 msgs at 2/s = ~2s, give extra margin)
	time.Sleep(3 * time.Second)

	mu.Lock()
	times := make([]time.Time, len(sendTimes))
	copy(times, sendTimes)
	mu.Unlock()

	if len(times) != 4 {
		t.Fatalf("expected 4 sends, got %d", len(times))
	}

	// Verify rate limiting: total duration should be at least 1s
	// (first message immediate, then ~500ms between each subsequent one at 2/s)
	totalDuration := times[len(times)-1].Sub(times[0])
	if totalDuration < 1*time.Second {
		t.Fatalf("expected total duration >= 1s for 4 msgs at 2/s rate, got %v", totalDuration)
	}
}

func TestNewChannelWorker_DefaultRate(t *testing.T) {
	ch := &mockChannel{}
	w := newChannelWorker("unknown_channel", ch)

	if w.limiter == nil {
		t.Fatal("expected limiter to be non-nil")
	}
	if w.limiter.Limit() != rate.Limit(defaultRateLimit) {
		t.Fatalf("expected rate limit %v, got %v", rate.Limit(defaultRateLimit), w.limiter.Limit())
	}
}

func TestNewChannelWorker_ConfiguredRate(t *testing.T) {
	ch := &mockChannel{}

	for name, expectedRate := range channelRateConfig {
		w := newChannelWorker(name, ch)
		if w.limiter.Limit() != rate.Limit(expectedRate) {
			t.Fatalf("channel %s: expected rate %v, got %v", name, expectedRate, w.limiter.Limit())
		}
	}
}

func TestRunWorker_MessageSplitting(t *testing.T) {
	m := newTestManager()

	var mu sync.Mutex
	var received []string

	ch := &mockChannelWithLength{
		mockChannel: mockChannel{
			sendFn: func(_ context.Context, msg bus.OutboundMessage) error {
				mu.Lock()
				received = append(received, msg.Content)
				mu.Unlock()
				return nil
			},
		},
		maxLen: 5,
	}

	w := &channelWorker{
		ch:      ch,
		queue:   make(chan bus.OutboundMessage, 10),
		done:    make(chan struct{}),
		limiter: rate.NewLimiter(rate.Inf, 1),
	}

	ctx := t.Context()

	go m.runWorker(ctx, "test", w)

	// Send a message that should be split
	w.queue <- bus.OutboundMessage{Channel: "test", ChatID: "1", Content: "hello world"}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	count := len(received)
	mu.Unlock()

	if count < 2 {
		t.Fatalf("expected message to be split into at least 2 chunks, got %d", count)
	}
}

// mockChannelWithLength implements MessageLengthProvider.
type mockChannelWithLength struct {
	mockChannel
	maxLen int
}

func (m *mockChannelWithLength) MaxMessageLength() int {
	return m.maxLen
}

func TestSendWithRetry_ExponentialBackoff(t *testing.T) {
	m := newTestManager()

	var callTimes []time.Time
	var callCount atomic.Int32
	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
			callTimes = append(callTimes, time.Now())
			callCount.Add(1)
			return fmt.Errorf("timeout: %w", ErrTemporary)
		},
	}
	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}

	ctx := context.Background()
	msg := bus.OutboundMessage{Channel: "test", ChatID: "1", Content: "hello"}

	start := time.Now()
	m.sendWithRetry(ctx, "test", w, msg)
	totalElapsed := time.Since(start)

	// With maxRetries=3: attempts at 0, ~500ms, ~1.5s, ~3.5s
	// Total backoff: 500ms + 1s + 2s = 3.5s
	// Allow some margin
	if totalElapsed < 3*time.Second {
		t.Fatalf("expected total elapsed >= 3s for exponential backoff, got %v", totalElapsed)
	}

	if int(callCount.Load()) != maxRetries+1 {
		t.Fatalf("expected %d calls, got %d", maxRetries+1, callCount.Load())
	}
}

// --- Phase 10: preSend orchestration tests ---

// mockMessageEditor is a channel that supports MessageEditor.
type mockMessageEditor struct {
	mockChannel
	editFn func(ctx context.Context, chatID, messageID, content string) error
}

func (m *mockMessageEditor) EditMessage(ctx context.Context, chatID, messageID, content string) error {
	return m.editFn(ctx, chatID, messageID, content)
}

func TestPreSend_PlaceholderEditSuccess(t *testing.T) {
	m := newTestManager()
	var sendCalled bool
	var editCalled bool

	ch := &mockMessageEditor{
		mockChannel: mockChannel{
			sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
				sendCalled = true
				return nil
			},
		},
		editFn: func(_ context.Context, chatID, messageID, content string) error {
			editCalled = true
			if chatID != "123" {
				t.Fatalf("expected chatID 123, got %s", chatID)
			}
			if messageID != "456" {
				t.Fatalf("expected messageID 456, got %s", messageID)
			}
			if content != "hello" {
				t.Fatalf("expected content 'hello', got %s", content)
			}
			return nil
		},
	}

	// Register placeholder
	m.RecordPlaceholder("test", "123", "456")

	msg := bus.OutboundMessage{Channel: "test", ChatID: "123", Content: "hello"}
	edited := m.preSend(context.Background(), "test", msg, ch)

	if !edited {
		t.Fatal("expected preSend to return true (placeholder edited)")
	}
	if !editCalled {
		t.Fatal("expected EditMessage to be called")
	}
	if sendCalled {
		t.Fatal("expected Send to NOT be called when placeholder edited")
	}
}

func TestPreSend_PlaceholderEditFails_FallsThrough(t *testing.T) {
	m := newTestManager()

	ch := &mockMessageEditor{
		mockChannel: mockChannel{
			sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
				return nil
			},
		},
		editFn: func(_ context.Context, _, _, _ string) error {
			return fmt.Errorf("edit failed")
		},
	}

	m.RecordPlaceholder("test", "123", "456")

	msg := bus.OutboundMessage{Channel: "test", ChatID: "123", Content: "hello"}
	edited := m.preSend(context.Background(), "test", msg, ch)

	if edited {
		t.Fatal("expected preSend to return false when edit fails")
	}
}

func TestInvokeTypingStop_CallsRegisteredStop(t *testing.T) {
	m := newTestManager()
	var stopCalled bool

	m.RecordTypingStop("telegram", "chat123", func() {
		stopCalled = true
	})

	m.InvokeTypingStop("telegram", "chat123")

	if !stopCalled {
		t.Fatal("expected typing stop func to be called")
	}
}

func TestInvokeTypingStop_NoOpWhenNoEntry(t *testing.T) {
	m := newTestManager()
	// Should not panic
	m.InvokeTypingStop("telegram", "nonexistent")
}

func TestInvokeTypingStop_Idempotent(t *testing.T) {
	m := newTestManager()
	var callCount int

	m.RecordTypingStop("telegram", "chat123", func() {
		callCount++
	})

	m.InvokeTypingStop("telegram", "chat123")
	m.InvokeTypingStop("telegram", "chat123") // Second call: entry already removed, no-op

	if callCount != 1 {
		t.Fatalf("expected stop to be called once, got %d", callCount)
	}
}

func TestPreSend_TypingStopCalled(t *testing.T) {
	m := newTestManager()
	var stopCalled bool

	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
			return nil
		},
	}

	m.RecordTypingStop("test", "123", func() {
		stopCalled = true
	})

	msg := bus.OutboundMessage{Channel: "test", ChatID: "123", Content: "hello"}
	m.preSend(context.Background(), "test", msg, ch)

	if !stopCalled {
		t.Fatal("expected typing stop func to be called")
	}
}

func TestPreSend_NoRegisteredState(t *testing.T) {
	m := newTestManager()

	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
			return nil
		},
	}

	msg := bus.OutboundMessage{Channel: "test", ChatID: "123", Content: "hello"}
	edited := m.preSend(context.Background(), "test", msg, ch)

	if edited {
		t.Fatal("expected preSend to return false with no registered state")
	}
}

func TestPreSend_TypingAndPlaceholder(t *testing.T) {
	m := newTestManager()
	var stopCalled bool
	var editCalled bool

	ch := &mockMessageEditor{
		mockChannel: mockChannel{
			sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
				return nil
			},
		},
		editFn: func(_ context.Context, _, _, _ string) error {
			editCalled = true
			return nil
		},
	}

	m.RecordTypingStop("test", "123", func() {
		stopCalled = true
	})
	m.RecordPlaceholder("test", "123", "456")

	msg := bus.OutboundMessage{Channel: "test", ChatID: "123", Content: "hello"}
	edited := m.preSend(context.Background(), "test", msg, ch)

	if !stopCalled {
		t.Fatal("expected typing stop to be called")
	}
	if !editCalled {
		t.Fatal("expected EditMessage to be called")
	}
	if !edited {
		t.Fatal("expected preSend to return true")
	}
}

func TestRecordPlaceholder_ConcurrentSafe(t *testing.T) {
	m := newTestManager()

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			chatID := fmt.Sprintf("chat_%d", i%10)
			m.RecordPlaceholder("test", chatID, fmt.Sprintf("msg_%d", i))
		}(i)
	}
	wg.Wait()
}

func TestRecordTypingStop_ConcurrentSafe(t *testing.T) {
	m := newTestManager()

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			chatID := fmt.Sprintf("chat_%d", i%10)
			m.RecordTypingStop("test", chatID, func() {})
		}(i)
	}
	wg.Wait()
}

func TestRecordTypingStop_ReplacesExistingStop(t *testing.T) {
	m := newTestManager()
	var oldStopCalls int
	var newStopCalls int

	m.RecordTypingStop("test", "123", func() {
		oldStopCalls++
	})

	m.RecordTypingStop("test", "123", func() {
		newStopCalls++
	})

	if oldStopCalls != 1 {
		t.Fatalf("expected previous typing stop to be called once when replaced, got %d", oldStopCalls)
	}
	if newStopCalls != 0 {
		t.Fatalf("expected replacement typing stop to stay active until preSend, got %d calls", newStopCalls)
	}

	msg := bus.OutboundMessage{Channel: "test", ChatID: "123", Content: "hello"}
	m.preSend(context.Background(), "test", msg, &mockChannel{})

	if newStopCalls != 1 {
		t.Fatalf("expected replacement typing stop to be called by preSend, got %d", newStopCalls)
	}
	if oldStopCalls != 1 {
		t.Fatalf("expected previous typing stop to not be called again, got %d", oldStopCalls)
	}
}

func TestSendWithRetry_PreSendEditsPlaceholder(t *testing.T) {
	m := newTestManager()
	var sendCalled bool

	ch := &mockMessageEditor{
		mockChannel: mockChannel{
			sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
				sendCalled = true
				return nil
			},
		},
		editFn: func(_ context.Context, _, _, _ string) error {
			return nil // edit succeeds
		},
	}

	m.RecordPlaceholder("test", "123", "456")

	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}

	msg := bus.OutboundMessage{Channel: "test", ChatID: "123", Content: "hello"}
	m.sendWithRetry(context.Background(), "test", w, msg)

	if sendCalled {
		t.Fatal("expected Send to NOT be called when placeholder was edited")
	}
}

// --- Dispatcher exit tests (Step 1) ---

func TestDispatcherExitsOnCancel(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	m := &Manager{
		channels: make(map[string]Channel),
		workers:  make(map[string]*channelWorker),
		bus:      mb,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		m.dispatchOutbound(ctx)
		close(done)
	}()

	// Cancel context and verify the dispatcher exits quickly
	cancel()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("dispatchOutbound did not exit within 2s after context cancel")
	}
}

func TestDispatcherMediaExitsOnCancel(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	m := &Manager{
		channels: make(map[string]Channel),
		workers:  make(map[string]*channelWorker),
		bus:      mb,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		m.dispatchOutboundMedia(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("dispatchOutboundMedia did not exit within 2s after context cancel")
	}
}

// --- TTL Janitor tests (Step 2) ---

func TestTypingStopJanitorEviction(t *testing.T) {
	m := newTestManager()

	var stopCalled atomic.Bool
	// Store a typing entry with a creation time far in the past
	m.typingStops.Store("test:123", typingEntry{
		stop:      func() { stopCalled.Store(true) },
		createdAt: time.Now().Add(-10 * time.Minute), // well past typingStopTTL
	})

	// Run janitor with a short-lived context
	ctx, cancel := context.WithCancel(context.Background())

	// Manually trigger the janitor logic once by simulating a tick
	go func() {
		// Override janitor to run immediately
		now := time.Now()
		m.typingStops.Range(func(key, value any) bool {
			if entry, ok := value.(typingEntry); ok {
				if now.Sub(entry.createdAt) > typingStopTTL {
					if _, loaded := m.typingStops.LoadAndDelete(key); loaded {
						entry.stop()
					}
				}
			}
			return true
		})
		cancel()
	}()

	<-ctx.Done()

	if !stopCalled.Load() {
		t.Fatal("expected typing stop function to be called by janitor eviction")
	}

	// Verify entry was deleted
	if _, loaded := m.typingStops.Load("test:123"); loaded {
		t.Fatal("expected typing entry to be deleted after eviction")
	}
}

func TestPlaceholderJanitorEviction(t *testing.T) {
	m := newTestManager()

	// Store a placeholder entry with a creation time far in the past
	m.placeholders.Store("test:456", placeholderEntry{
		id:        "msg_old",
		createdAt: time.Now().Add(-20 * time.Minute), // well past placeholderTTL
	})

	// Simulate janitor logic
	now := time.Now()
	m.placeholders.Range(func(key, value any) bool {
		if entry, ok := value.(placeholderEntry); ok {
			if now.Sub(entry.createdAt) > placeholderTTL {
				m.placeholders.Delete(key)
			}
		}
		return true
	})

	// Verify entry was deleted
	if _, loaded := m.placeholders.Load("test:456"); loaded {
		t.Fatal("expected placeholder entry to be deleted after eviction")
	}
}

func TestPreSendStillWorksWithWrappedTypes(t *testing.T) {
	m := newTestManager()
	var stopCalled bool
	var editCalled bool

	ch := &mockMessageEditor{
		mockChannel: mockChannel{
			sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
				return nil
			},
		},
		editFn: func(_ context.Context, chatID, messageID, content string) error {
			editCalled = true
			if messageID != "ph_id" {
				t.Fatalf("expected messageID ph_id, got %s", messageID)
			}
			return nil
		},
	}

	// Use the new wrapped types via the public API
	m.RecordTypingStop("test", "chat1", func() {
		stopCalled = true
	})
	m.RecordPlaceholder("test", "chat1", "ph_id")

	msg := bus.OutboundMessage{Channel: "test", ChatID: "chat1", Content: "response"}
	edited := m.preSend(context.Background(), "test", msg, ch)

	if !stopCalled {
		t.Fatal("expected typing stop to be called via wrapped type")
	}
	if !editCalled {
		t.Fatal("expected EditMessage to be called via wrapped type")
	}
	if !edited {
		t.Fatal("expected preSend to return true")
	}
}

// --- Lazy worker creation tests (Step 6) ---

func TestLazyWorkerCreation(t *testing.T) {
	m := newTestManager()

	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
			return nil
		},
	}

	// RegisterChannel should NOT create a worker
	m.RegisterChannel("lazy", ch)

	m.mu.RLock()
	_, chExists := m.channels["lazy"]
	_, wExists := m.workers["lazy"]
	m.mu.RUnlock()

	if !chExists {
		t.Fatal("expected channel to be registered")
	}
	if wExists {
		t.Fatal("expected worker to NOT be created by RegisterChannel (lazy creation)")
	}
}

// --- FastID uniqueness test (Step 5) ---

func TestBuildMediaScope_FastIDUniqueness(t *testing.T) {
	seen := make(map[string]bool)

	for range 1000 {
		scope := BuildMediaScope("test", "chat1", "")
		if seen[scope] {
			t.Fatalf("duplicate scope generated: %s", scope)
		}
		seen[scope] = true
	}

	// Verify format: "channel:chatID:id"
	scope := BuildMediaScope("telegram", "42", "")
	parts := 0
	for _, c := range scope {
		if c == ':' {
			parts++
		}
	}
	if parts != 2 {
		t.Fatalf("expected scope to have 2 colons (channel:chatID:id), got: %s", scope)
	}
}

func TestBuildMediaScope_WithMessageID(t *testing.T) {
	scope := BuildMediaScope("discord", "chat99", "msg123")
	expected := "discord:chat99:msg123"
	if scope != expected {
		t.Fatalf("expected %s, got %s", expected, scope)
	}
}

func TestManager_PlaceholderConsumedByResponse(t *testing.T) {
	mgr := &Manager{
		channels:     make(map[string]Channel),
		workers:      make(map[string]*channelWorker),
		placeholders: sync.Map{},
	}

	mockCh := &mockChannel{
		sendFn: func(ctx context.Context, msg bus.OutboundMessage) error {
			return nil
		},
	}
	worker := newChannelWorker("mock", mockCh)
	mgr.channels["mock"] = mockCh
	mgr.workers["mock"] = worker

	ctx := context.Background()
	key := "mock:chat-1"

	// Simulate a placeholder recorded by base.go HandleMessage
	mgr.RecordPlaceholder("mock", "chat-1", "ph-123")

	if _, ok := mgr.placeholders.Load(key); !ok {
		t.Fatal("expected placeholder to be recorded")
	}

	// Transcription feedback arrives first — it should consume the placeholder
	// and be delivered via EditMessage, not Send.
	msgTranscript := bus.OutboundMessage{
		Channel: "mock",
		ChatID:  "chat-1",
		Content: "Transcript: hello",
	}
	mgr.sendWithRetry(ctx, "mock", worker, msgTranscript)

	if mockCh.editedMessages != 1 {
		t.Errorf("expected 1 edited message (placeholder consumed by transcript), got %d", mockCh.editedMessages)
	}
	if len(mockCh.sentMessages) != 0 {
		t.Errorf("expected 0 normal messages (transcript used edit), got %d", len(mockCh.sentMessages))
	}

	// Placeholder should be gone now
	if _, ok := mgr.placeholders.Load(key); ok {
		t.Error("expected placeholder to be removed after being consumed")
	}

	// Final LLM response arrives — no placeholder left, so it goes through Send
	msgFinal := bus.OutboundMessage{
		Channel: "mock",
		ChatID:  "chat-1",
		Content: "Final Answer",
	}
	mgr.sendWithRetry(ctx, "mock", worker, msgFinal)

	if len(mockCh.sentMessages) != 1 {
		t.Errorf("expected 1 normal message sent, got %d", len(mockCh.sentMessages))
	}
}

func TestSendMessage_Synchronous(t *testing.T) {
	m := newTestManager()

	var received []bus.OutboundMessage
	ch := &mockChannel{
		sendFn: func(_ context.Context, msg bus.OutboundMessage) error {
			received = append(received, msg)
			return nil
		},
	}

	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}
	m.channels["test"] = ch
	m.workers["test"] = w

	msg := bus.OutboundMessage{
		Channel:          "test",
		ChatID:           "123",
		Content:          "hello world",
		ReplyToMessageID: "msg-456",
	}

	err := m.SendMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// SendMessage is synchronous — message should already be delivered
	if len(received) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(received))
	}
	if received[0].ReplyToMessageID != "msg-456" {
		t.Fatalf("expected ReplyToMessageID msg-456, got %s", received[0].ReplyToMessageID)
	}
	if received[0].Content != "hello world" {
		t.Fatalf("expected content 'hello world', got %s", received[0].Content)
	}
}

func TestSendMessage_UnknownChannel(t *testing.T) {
	m := newTestManager()

	msg := bus.OutboundMessage{
		Channel: "nonexistent",
		ChatID:  "123",
		Content: "hello",
	}

	err := m.SendMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for unknown channel")
	}
}

func TestSendMessage_NoWorker(t *testing.T) {
	m := newTestManager()

	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error { return nil },
	}
	m.channels["test"] = ch
	// No worker registered

	msg := bus.OutboundMessage{
		Channel: "test",
		ChatID:  "123",
		Content: "hello",
	}

	err := m.SendMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error when no worker exists")
	}
}

func TestSendMessage_WithRetry(t *testing.T) {
	m := newTestManager()

	var callCount int
	ch := &mockChannel{
		sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
			callCount++
			if callCount == 1 {
				return fmt.Errorf("transient: %w", ErrTemporary)
			}
			return nil
		},
	}

	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}
	m.channels["test"] = ch
	m.workers["test"] = w

	msg := bus.OutboundMessage{
		Channel: "test",
		ChatID:  "123",
		Content: "retry me",
	}

	err := m.SendMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if callCount != 2 {
		t.Fatalf("expected 2 Send calls (1 failure + 1 success), got %d", callCount)
	}
}

func TestSendMessage_WithSplitting(t *testing.T) {
	m := newTestManager()

	var received []string
	ch := &mockChannelWithLength{
		mockChannel: mockChannel{
			sendFn: func(_ context.Context, msg bus.OutboundMessage) error {
				received = append(received, msg.Content)
				return nil
			},
		},
		maxLen: 5,
	}

	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}
	m.channels["test"] = ch
	m.workers["test"] = w

	msg := bus.OutboundMessage{
		Channel: "test",
		ChatID:  "123",
		Content: "hello world",
	}

	err := m.SendMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(received) < 2 {
		t.Fatalf("expected message to be split into at least 2 chunks, got %d", len(received))
	}
}

func TestSendMessage_PreservesOrdering(t *testing.T) {
	m := newTestManager()

	var order []string
	ch := &mockChannel{
		sendFn: func(_ context.Context, msg bus.OutboundMessage) error {
			order = append(order, msg.Content)
			return nil
		},
	}

	w := &channelWorker{
		ch:      ch,
		limiter: rate.NewLimiter(rate.Inf, 1),
	}
	m.channels["test"] = ch
	m.workers["test"] = w

	// Send two messages sequentially — they must arrive in order
	_ = m.SendMessage(context.Background(), bus.OutboundMessage{
		Channel: "test", ChatID: "1", Content: "first",
	})
	_ = m.SendMessage(context.Background(), bus.OutboundMessage{
		Channel: "test", ChatID: "1", Content: "second",
	})

	if len(order) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(order))
	}
	if order[0] != "first" || order[1] != "second" {
		t.Fatalf("expected [first, second], got %v", order)
	}
}

func TestManager_SendPlaceholder(t *testing.T) {
	mgr := &Manager{
		channels:     make(map[string]Channel),
		workers:      make(map[string]*channelWorker),
		placeholders: sync.Map{},
	}

	mockCh := &mockChannel{
		sendFn: func(ctx context.Context, msg bus.OutboundMessage) error {
			return nil
		},
	}
	mgr.channels["mock"] = mockCh

	ctx := context.Background()

	// SendPlaceholder should send a placeholder and record it
	ok := mgr.SendPlaceholder(ctx, "mock", "chat-1")
	if !ok {
		t.Fatal("expected SendPlaceholder to succeed")
	}
	if mockCh.placeholdersSent != 1 {
		t.Errorf("expected 1 placeholder sent, got %d", mockCh.placeholdersSent)
	}

	key := "mock:chat-1"
	if _, loaded := mgr.placeholders.Load(key); !loaded {
		t.Error("expected placeholder to be recorded in manager")
	}

	// SendPlaceholder on unknown channel should return false
	ok = mgr.SendPlaceholder(ctx, "unknown", "chat-1")
	if ok {
		t.Error("expected SendPlaceholder to fail for unknown channel")
	}
}

// --- Reload tests ---

// reloadMockChannel is a controllable channel for Reload tests.
type reloadMockChannel struct {
	BaseChannel
	stopped  atomic.Bool
	sendFn   func(ctx context.Context, msg bus.OutboundMessage) error
	startErr error
}

func (r *reloadMockChannel) Start(ctx context.Context) error { return r.startErr }
func (r *reloadMockChannel) Stop(ctx context.Context) error {
	r.stopped.Store(true)
	return nil
}

func (r *reloadMockChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if r.sendFn != nil {
		return r.sendFn(ctx, msg)
	}
	return nil
}

func TestReloadRemovesChannelWithoutDeadlock(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ch := &reloadMockChannel{}

	m := &Manager{
		channels: map[string]Channel{"test_reload_rm": ch},
		workers:  make(map[string]*channelWorker),
		bus:      mb,
		config:   config.DefaultConfig(),
	}

	// Simulate a running worker for this channel.
	dispatchCtx, cancel := context.WithCancel(context.Background())
	m.dispatchTask = &asyncTask{cancel: cancel}
	w := newChannelWorker("test_reload_rm", ch)
	m.workers["test_reload_rm"] = w
	go m.runWorker(dispatchCtx, "test_reload_rm", w)
	go m.runMediaWorker(dispatchCtx, "test_reload_rm", w)

	// Set channel hashes so Reload sees "test_reload_rm" as existing.
	m.channelHashes = map[string]string{"test_reload_rm": "old_hash"}

	// Reload with an empty config removes the channel.
	// This must not deadlock — the old code spawned goroutines that re-acquired the lock.
	done := make(chan error, 1)
	go func() {
		done <- m.Reload(context.Background(), config.DefaultConfig())
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Reload returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Reload deadlocked")
	}

	if !ch.stopped.Load() {
		t.Error("expected channel to be stopped")
	}
	m.mu.RLock()
	_, exists := m.channels["test_reload_rm"]
	_, wExists := m.workers["test_reload_rm"]
	m.mu.RUnlock()
	if exists {
		t.Error("channel should have been removed from map")
	}
	if wExists {
		t.Error("worker should have been removed from map")
	}
}

func TestReloadCancelsPreviousDispatcher(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	m := &Manager{
		channels: make(map[string]Channel),
		workers:  make(map[string]*channelWorker),
		bus:      mb,
		config:   config.DefaultConfig(),
	}

	// Simulate an initial StartAll by creating a dispatch context.
	oldCtx, oldCancel := context.WithCancel(context.Background())
	m.dispatchTask = &asyncTask{cancel: oldCancel}

	oldDispatcherDone := make(chan struct{})
	go func() {
		m.dispatchOutbound(oldCtx)
		close(oldDispatcherDone)
	}()

	m.channelHashes = map[string]string{}

	// Reload should cancel the old dispatcher context.
	err := m.Reload(context.Background(), config.DefaultConfig())
	if err != nil {
		t.Fatalf("Reload returned error: %v", err)
	}

	// The old dispatcher should have exited because its context was cancelled.
	select {
	case <-oldDispatcherDone:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("old dispatcher was not cancelled by Reload")
	}

	// Clean up the new dispatcher.
	m.mu.RLock()
	task := m.dispatchTask
	m.mu.RUnlock()
	if task != nil {
		task.cancel()
	}
}

func TestDispatcherSurvivesClosedQueue(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	goodSent := make(chan struct{}, 1)
	goodCh := &reloadMockChannel{
		sendFn: func(ctx context.Context, msg bus.OutboundMessage) error {
			select {
			case goodSent <- struct{}{}:
			default:
			}
			return nil
		},
	}

	m := &Manager{
		channels: map[string]Channel{"good": goodCh},
		workers:  make(map[string]*channelWorker),
		bus:      mb,
	}

	dispatchCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	goodW := newChannelWorker("good", goodCh)
	m.workers["good"] = goodW
	go m.runWorker(dispatchCtx, "good", goodW)
	go m.runMediaWorker(dispatchCtx, "good", goodW)

	// Create a stale worker with a closed queue to simulate the reload race.
	staleW := newChannelWorker("stale", &reloadMockChannel{})
	close(staleW.queue) // closed — sending here would panic
	close(staleW.mediaDone)
	close(staleW.done)
	m.channels["stale"] = &reloadMockChannel{}
	m.workers["stale"] = staleW

	go m.dispatchOutbound(dispatchCtx)

	// Send a message to the stale channel — dispatcher should recover, not crash.
	mb.PublishOutbound(context.Background(), bus.OutboundMessage{Channel: "stale", Content: "boom"})
	// Small delay to let the dispatcher process the stale message.
	time.Sleep(50 * time.Millisecond)

	// Now send to the good channel — dispatcher should still be alive.
	mb.PublishOutbound(context.Background(), bus.OutboundMessage{Channel: "good", Content: "hello"})

	select {
	case <-goodSent:
		// success — dispatcher survived the closed queue
	case <-time.After(2 * time.Second):
		t.Fatal("dispatcher died after encountering closed queue; message to good channel was never delivered")
	}
}

// --- Lifecycle tests ---

func TestNewManager(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	cfg := config.DefaultConfig()
	m, err := NewManager(cfg, mb, nil)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	if m.bus != mb {
		t.Error("bus not set")
	}
	if m.config != cfg {
		t.Error("config not set")
	}
	if m.channels == nil {
		t.Error("channels map is nil")
	}
	if m.workers == nil {
		t.Error("workers map is nil")
	}
	if m.channelHashes == nil {
		t.Error("channelHashes map is nil")
	}
	// No channels enabled in default config, so maps should be empty.
	if len(m.channels) != 0 {
		t.Errorf("expected 0 channels, got %d", len(m.channels))
	}
}

func TestStartAllAndStopAll(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	var started, stopped atomic.Int32
	ch := &reloadMockChannel{
		sendFn: func(ctx context.Context, msg bus.OutboundMessage) error { return nil },
	}
	// Override Start/Stop tracking by using a wrapper
	trackCh := &trackingChannel{
		Channel: ch,
		onStart: func() { started.Add(1) },
		onStop:  func() { stopped.Add(1) },
	}

	m := &Manager{
		channels: map[string]Channel{"test_lifecycle": trackCh},
		workers:  make(map[string]*channelWorker),
		bus:      mb,
	}

	ctx := context.Background()
	if err := m.StartAll(ctx); err != nil {
		t.Fatalf("StartAll returned error: %v", err)
	}

	if started.Load() != 1 {
		t.Errorf("expected 1 channel started, got %d", started.Load())
	}

	// Verify worker was created
	m.mu.RLock()
	_, wExists := m.workers["test_lifecycle"]
	m.mu.RUnlock()
	if !wExists {
		t.Error("worker should exist after StartAll")
	}

	// Verify message delivery through the full pipeline
	delivered := make(chan struct{}, 1)
	trackCh.sendFn = func(ctx context.Context, msg bus.OutboundMessage) error {
		select {
		case delivered <- struct{}{}:
		default:
		}
		return nil
	}
	mb.PublishOutbound(ctx, bus.OutboundMessage{Channel: "test_lifecycle", Content: "hello"})

	select {
	case <-delivered:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("message not delivered through StartAll pipeline")
	}

	// StopAll
	if err := m.StopAll(ctx); err != nil {
		t.Fatalf("StopAll returned error: %v", err)
	}

	if stopped.Load() != 1 {
		t.Errorf("expected 1 channel stopped, got %d", stopped.Load())
	}

	// Verify dispatcher context was cancelled (dispatchTask set to nil)
	m.mu.RLock()
	task := m.dispatchTask
	m.mu.RUnlock()
	if task != nil {
		t.Error("dispatchTask should be nil after StopAll")
	}
}

// trackingChannel wraps a Channel and calls hooks on Start/Stop.
type trackingChannel struct {
	Channel
	sendFn  func(ctx context.Context, msg bus.OutboundMessage) error
	onStart func()
	onStop  func()
}

func (t *trackingChannel) Start(ctx context.Context) error {
	if t.onStart != nil {
		t.onStart()
	}
	return t.Channel.Start(ctx)
}

func (t *trackingChannel) Stop(ctx context.Context) error {
	if t.onStop != nil {
		t.onStop()
	}
	return t.Channel.Stop(ctx)
}

func (t *trackingChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if t.sendFn != nil {
		return t.sendFn(ctx, msg)
	}
	return nil
}

func TestStartAllSkipsFailedChannels(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	failCh := &reloadMockChannel{startErr: errors.New("start failed")}
	okCh := &reloadMockChannel{}

	m := &Manager{
		channels: map[string]Channel{
			"fail_ch": failCh,
			"ok_ch":   okCh,
		},
		workers: make(map[string]*channelWorker),
		bus:     mb,
	}

	if err := m.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll returned error: %v", err)
	}

	m.mu.RLock()
	_, failW := m.workers["fail_ch"]
	_, okW := m.workers["ok_ch"]
	m.mu.RUnlock()

	if failW {
		t.Error("failed channel should not have a worker")
	}
	if !okW {
		t.Error("ok channel should have a worker")
	}

	// Cleanup
	m.StopAll(context.Background())
}

func TestStartAllNoChannels(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	m := &Manager{
		channels: make(map[string]Channel),
		workers:  make(map[string]*channelWorker),
		bus:      mb,
	}

	// Should not error even with no channels
	if err := m.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll with no channels returned error: %v", err)
	}

	// Dispatcher should still be running — verify by StopAll not hanging
	done := make(chan error, 1)
	go func() {
		done <- m.StopAll(context.Background())
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StopAll returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("StopAll hung with no channels")
	}
}

func TestUnregisterChannel(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ch := &reloadMockChannel{}
	m := &Manager{
		channels: map[string]Channel{"unreg_test": ch},
		workers:  make(map[string]*channelWorker),
		bus:      mb,
	}

	// Create and start a worker
	dispatchCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newChannelWorker("unreg_test", ch)
	m.workers["unreg_test"] = w
	go m.runWorker(dispatchCtx, "unreg_test", w)
	go m.runMediaWorker(dispatchCtx, "unreg_test", w)

	// Unregister should close queues, wait for workers, and clean up maps
	done := make(chan struct{})
	go func() {
		m.UnregisterChannel("unreg_test")
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("UnregisterChannel hung")
	}

	m.mu.RLock()
	_, chExists := m.channels["unreg_test"]
	_, wExists := m.workers["unreg_test"]
	m.mu.RUnlock()

	if chExists {
		t.Error("channel should have been removed")
	}
	if wExists {
		t.Error("worker should have been removed")
	}
}

func TestUnregisterNonexistentChannel(t *testing.T) {
	m := &Manager{
		channels: make(map[string]Channel),
		workers:  make(map[string]*channelWorker),
	}

	// Should not panic or hang
	done := make(chan struct{})
	go func() {
		m.UnregisterChannel("does_not_exist")
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("UnregisterChannel hung on nonexistent channel")
	}
}

func TestSetupHTTPServer(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	m := &Manager{
		channels: make(map[string]Channel),
		workers:  make(map[string]*channelWorker),
		bus:      mb,
	}

	extraCalled := false
	m.SetupHTTPServer(":0", nil, func(mux *http.ServeMux) {
		extraCalled = true
		mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {})
	})

	if m.httpServer == nil {
		t.Fatal("httpServer should be set")
	}
	if m.mux == nil {
		t.Fatal("mux should be set")
	}
	if !extraCalled {
		t.Error("extra handler should have been called")
	}
	if m.httpServer.ReadTimeout != 30*time.Second {
		t.Error("unexpected ReadTimeout")
	}
	if m.httpServer.WriteTimeout != 30*time.Second {
		t.Error("unexpected WriteTimeout")
	}
}

func TestSetupHTTPServerWithWebhookChannel(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	wh := &mockWebhookChannel{
		reloadMockChannel: reloadMockChannel{},
		webhookPath:       "/webhook/test",
	}

	m := &Manager{
		channels: map[string]Channel{"webhook_ch": wh},
		workers:  make(map[string]*channelWorker),
		bus:      mb,
	}

	m.SetupHTTPServer(":0", nil)

	if m.httpServer == nil {
		t.Fatal("httpServer should be set")
	}
	// The webhook handler should have been registered — we can't easily test
	// mux routing, but we verify the setup didn't panic.
}

// mockWebhookChannel implements WebhookHandler for testing.
type mockWebhookChannel struct {
	reloadMockChannel
	webhookPath string
}

func (m *mockWebhookChannel) WebhookPath() string                              { return m.webhookPath }
func (m *mockWebhookChannel) ServeHTTP(w http.ResponseWriter, r *http.Request) {}

// mockStreamer is a test double for bus.Streamer.
type mockStreamer struct {
	updateFn   func(ctx context.Context, content string) error
	finalizeFn func(ctx context.Context, content string) error
	cancelFn   func(ctx context.Context)
}

func (s *mockStreamer) Update(ctx context.Context, content string) error {
	if s.updateFn != nil {
		return s.updateFn(ctx, content)
	}
	return nil
}

func (s *mockStreamer) Finalize(ctx context.Context, content string) error {
	if s.finalizeFn != nil {
		return s.finalizeFn(ctx, content)
	}
	return nil
}

func (s *mockStreamer) Cancel(ctx context.Context) {
	if s.cancelFn != nil {
		s.cancelFn(ctx)
	}
}

// mockStreamingChannel embeds mockChannel and implements StreamingCapable.
type mockStreamingChannel struct {
	mockChannel
	beginStreamFn func(ctx context.Context, chatID string) (Streamer, error)
}

func (m *mockStreamingChannel) BeginStream(ctx context.Context, chatID string) (Streamer, error) {
	return m.beginStreamFn(ctx, chatID)
}

func TestHiddenValuesCorrectMapping(t *testing.T) {
	cfg := config.DefaultConfig()
	ch := cfg.Channels

	// Test DingTalk — should use DingTalk secret, not QQ
	dtValue := map[string]any{}
	hiddenValues("dingtalk", dtValue, ch)
	if _, ok := dtValue["secret"]; !ok {
		t.Error("dingtalk should have secret key")
	}

	// Test QQ — should use QQ secret, not DingTalk
	qqValue := map[string]any{}
	hiddenValues("qq", qqValue, ch)
	if _, ok := qqValue["secret"]; !ok {
		t.Error("qq should have secret key")
	}

	// Test all branches of hiddenValues for coverage
	for _, tc := range []struct {
		name     string
		expected []string
	}{
		{"pico", []string{"token"}},
		{"telegram", []string{"token"}},
		{"discord", []string{"token"}},
		{"slack", []string{"bot_token", "app_token"}},
		{"matrix", []string{"token"}},
		{"onebot", []string{"token"}},
		{"line", []string{"token", "secret"}},
		{"dingtalk", []string{"secret"}},
		{"qq", []string{"secret"}},
		{"irc", []string{"password", "serv_password", "sasl_password"}},
		{"feishu", []string{"app_secret", "encrypt_key", "verification_token"}},
		{"wecom", nil}, // removed channel, no keys expected
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := map[string]any{}
			hiddenValues(tc.name, v, ch)
			for _, key := range tc.expected {
				if _, ok := v[key]; !ok {
					t.Errorf("hiddenValues(%q) missing key %q", tc.name, key)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Section 1: GetStreamer & finalizeHookStreamer
// ---------------------------------------------------------------------------

func TestGetStreamer_ChannelNotFound(t *testing.T) {
	m := newTestManager()
	s, ok := m.GetStreamer(context.Background(), "nonexistent", "chat1")
	assert.Nil(t, s)
	assert.False(t, ok)
}

func TestGetStreamer_NotStreamingCapable(t *testing.T) {
	m := newTestManager()
	ch := &mockChannel{sendFn: func(_ context.Context, _ bus.OutboundMessage) error { return nil }}
	m.channels["test"] = ch
	s, ok := m.GetStreamer(context.Background(), "test", "chat1")
	assert.Nil(t, s)
	assert.False(t, ok)
}

func TestGetStreamer_BeginStreamError(t *testing.T) {
	m := newTestManager()
	ch := &mockStreamingChannel{
		mockChannel: mockChannel{sendFn: func(_ context.Context, _ bus.OutboundMessage) error { return nil }},
		beginStreamFn: func(_ context.Context, _ string) (Streamer, error) {
			return nil, errors.New("stream unavailable")
		},
	}
	m.channels["test"] = ch
	s, ok := m.GetStreamer(context.Background(), "test", "chat1")
	assert.Nil(t, s)
	assert.False(t, ok)
}

func TestGetStreamer_Success(t *testing.T) {
	m := newTestManager()
	ms := &mockStreamer{}
	ch := &mockStreamingChannel{
		mockChannel: mockChannel{sendFn: func(_ context.Context, _ bus.OutboundMessage) error { return nil }},
		beginStreamFn: func(_ context.Context, _ string) (Streamer, error) {
			return ms, nil
		},
	}
	m.channels["test"] = ch
	s, ok := m.GetStreamer(context.Background(), "test", "chat1")
	require.True(t, ok)
	require.NotNil(t, s)
}

func TestGetStreamer_FinalizeMarksStreamActive(t *testing.T) {
	m := newTestManager()
	ms := &mockStreamer{}
	ch := &mockStreamingChannel{
		mockChannel: mockChannel{sendFn: func(_ context.Context, _ bus.OutboundMessage) error { return nil }},
		beginStreamFn: func(_ context.Context, _ string) (Streamer, error) {
			return ms, nil
		},
	}
	m.channels["test"] = ch

	s, ok := m.GetStreamer(context.Background(), "test", "chat1")
	require.True(t, ok)

	// Before Finalize, streamActive should not be set
	_, loaded := m.streamActive.Load("test:chat1")
	assert.False(t, loaded)

	// Finalize should mark streamActive
	err := s.Finalize(context.Background(), "final content")
	require.NoError(t, err)
	val, loaded := m.streamActive.Load("test:chat1")
	assert.True(t, loaded)
	assert.Equal(t, true, val)
}

func TestGetStreamer_FinalizeError_NoStreamActive(t *testing.T) {
	m := newTestManager()
	ms := &mockStreamer{
		finalizeFn: func(_ context.Context, _ string) error {
			return errors.New("finalize failed")
		},
	}
	ch := &mockStreamingChannel{
		mockChannel: mockChannel{sendFn: func(_ context.Context, _ bus.OutboundMessage) error { return nil }},
		beginStreamFn: func(_ context.Context, _ string) (Streamer, error) {
			return ms, nil
		},
	}
	m.channels["test"] = ch

	s, ok := m.GetStreamer(context.Background(), "test", "chat1")
	require.True(t, ok)

	err := s.Finalize(context.Background(), "content")
	assert.Error(t, err)
	_, loaded := m.streamActive.Load("test:chat1")
	assert.False(t, loaded)
}

// ---------------------------------------------------------------------------
// Section 2: SendToChannel
// ---------------------------------------------------------------------------

func TestSendToChannel_ChannelNotFound(t *testing.T) {
	m := newTestManager()
	err := m.SendToChannel(context.Background(), "nonexistent", "chat1", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSendToChannel_WithWorker(t *testing.T) {
	m := newTestManager()
	ch := &mockChannel{sendFn: func(_ context.Context, _ bus.OutboundMessage) error { return nil }}
	m.channels["test"] = ch
	w := &channelWorker{
		ch:         ch,
		queue:      make(chan bus.OutboundMessage, 10),
		mediaQueue: make(chan bus.OutboundMediaMessage, 10),
		done:       make(chan struct{}),
		mediaDone:  make(chan struct{}),
		limiter:    rate.NewLimiter(rate.Inf, 1),
	}
	m.workers["test"] = w

	err := m.SendToChannel(context.Background(), "test", "chat1", "hello")
	require.NoError(t, err)

	// Verify message was enqueued
	select {
	case msg := <-w.queue:
		assert.Equal(t, "test", msg.Channel)
		assert.Equal(t, "chat1", msg.ChatID)
		assert.Equal(t, "hello", msg.Content)
	default:
		t.Fatal("expected message in worker queue")
	}
}

func TestSendToChannel_NoWorker_FallbackSend(t *testing.T) {
	m := newTestManager()
	var sent bus.OutboundMessage
	ch := &mockChannel{sendFn: func(_ context.Context, msg bus.OutboundMessage) error {
		sent = msg
		return nil
	}}
	m.channels["test"] = ch
	// No worker registered

	err := m.SendToChannel(context.Background(), "test", "chat1", "hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", sent.Content)
}

func TestSendToChannel_ContextCanceled(t *testing.T) {
	m := newTestManager()
	ch := &mockChannel{sendFn: func(_ context.Context, _ bus.OutboundMessage) error { return nil }}
	m.channels["test"] = ch
	// Create a worker with an unbuffered queue (will block)
	w := &channelWorker{
		ch:         ch,
		queue:      make(chan bus.OutboundMessage), // unbuffered = full
		mediaQueue: make(chan bus.OutboundMediaMessage, 10),
		done:       make(chan struct{}),
		mediaDone:  make(chan struct{}),
		limiter:    rate.NewLimiter(rate.Inf, 1),
	}
	m.workers["test"] = w

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := m.SendToChannel(ctx, "test", "chat1", "hello")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ---------------------------------------------------------------------------
// Section 3: TTL Janitor - reaction eviction + context cancellation
// ---------------------------------------------------------------------------

func TestReactionUndoJanitorEviction(t *testing.T) {
	m := newTestManager()

	var undoCalled atomic.Bool

	// Store a reaction entry with a timestamp far in the past (exceeding typingStopTTL)
	m.reactionUndos.Store("test:chat1", reactionEntry{
		undo:      func() { undoCalled.Store(true) },
		createdAt: time.Now().Add(-10 * time.Minute), // well past 5m TTL
	})

	// Manually trigger the janitor logic once by simulating a tick
	// (same pattern as TestTypingStopJanitorEviction)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		now := time.Now()
		m.reactionUndos.Range(func(key, value any) bool {
			if entry, ok := value.(reactionEntry); ok {
				if now.Sub(entry.createdAt) > typingStopTTL {
					if _, loaded := m.reactionUndos.LoadAndDelete(key); loaded {
						entry.undo()
					}
				}
			}
			return true
		})
		cancel()
	}()

	<-ctx.Done()

	assert.True(t, undoCalled.Load(), "expired reaction undo should have been called")
	_, exists := m.reactionUndos.Load("test:chat1")
	assert.False(t, exists, "expired reaction entry should have been removed")
}

func TestJanitorRespectsContextCancellation(t *testing.T) {
	m := newTestManager()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		m.runTTLJanitor(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// success - janitor exited
	case <-time.After(2 * time.Second):
		t.Fatal("janitor did not exit after context cancellation")
	}
}

// ---------------------------------------------------------------------------
// Section 4: GetChannel, GetStatus, GetEnabledChannels
// ---------------------------------------------------------------------------

func TestGetChannel_Found(t *testing.T) {
	m := newTestManager()
	ch := &mockChannel{sendFn: func(_ context.Context, _ bus.OutboundMessage) error { return nil }}
	m.channels["test"] = ch

	got, ok := m.GetChannel("test")
	assert.True(t, ok)
	assert.Equal(t, ch, got)
}

func TestGetChannel_NotFound(t *testing.T) {
	m := newTestManager()
	got, ok := m.GetChannel("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestGetStatus_Empty(t *testing.T) {
	m := newTestManager()
	status := m.GetStatus()
	assert.Empty(t, status)
}

func TestGetStatus_WithChannels(t *testing.T) {
	m := newTestManager()
	ch1 := &mockChannel{sendFn: func(_ context.Context, _ bus.OutboundMessage) error { return nil }}
	ch1.SetRunning(true)
	ch2 := &mockChannel{sendFn: func(_ context.Context, _ bus.OutboundMessage) error { return nil }}
	ch2.SetRunning(false)
	m.channels["running"] = ch1
	m.channels["stopped"] = ch2

	status := m.GetStatus()
	require.Len(t, status, 2)

	runningStatus := status["running"].(map[string]any)
	assert.Equal(t, true, runningStatus["enabled"])
	assert.Equal(t, true, runningStatus["running"])

	stoppedStatus := status["stopped"].(map[string]any)
	assert.Equal(t, true, stoppedStatus["enabled"])
	assert.Equal(t, false, stoppedStatus["running"])
}

func TestGetEnabledChannels_Empty(t *testing.T) {
	m := newTestManager()
	names := m.GetEnabledChannels()
	assert.Empty(t, names)
}

func TestGetEnabledChannels_Multiple(t *testing.T) {
	m := newTestManager()
	m.channels["alpha"] = &mockChannel{sendFn: func(_ context.Context, _ bus.OutboundMessage) error { return nil }}
	m.channels["beta"] = &mockChannel{sendFn: func(_ context.Context, _ bus.OutboundMessage) error { return nil }}

	names := m.GetEnabledChannels()
	assert.Len(t, names, 2)
	assert.ElementsMatch(t, []string{"alpha", "beta"}, names)
}
