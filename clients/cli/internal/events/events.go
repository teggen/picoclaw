package events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/clients/cli/internal/client"
	"github.com/sipeed/picoclaw/clients/cli/internal/styles"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Watch real-time event stream",
		RunE:  run,
	}
	cmd.Flags().String("type", "", "Event type filter pattern (e.g. tool.*, session.*)")
	cmd.Flags().StringP("session", "s", "", "Session key filter")
	cmd.Flags().Bool("errors", false, "Only show error events")
	return cmd
}

func run(cmd *cobra.Command, _ []string) error {
	c, jsonMode, _ := client.FromCommand(cmd)

	filter := client.EventFilter{}
	filter.Type, _ = cmd.Flags().GetString("type")
	filter.Session, _ = cmd.Flags().GetString("session")
	filter.Errors, _ = cmd.Flags().GetBool("errors")

	conn, err := client.DialEvents(c.BaseURL, filter)
	if err != nil {
		return err
	}
	defer conn.Close()

	// JSON mode: print raw events to stdout.
	if jsonMode {
		for msg := range conn.Messages() {
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			fmt.Println(string(data))
		}
		return nil
	}

	// TUI mode.
	model := newEventsModel(conn)
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("events TUI error: %w", err)
	}
	return nil
}

// Bubble Tea messages.
type eventReceivedMsg client.EventMessage

// eventRecord holds a structured event and its cached compact rendering.
type eventRecord struct {
	msg     client.EventMessage
	compact string
}

type eventsModel struct {
	viewport     viewport.Model
	spinner      spinner.Model
	conn         *client.EventConn
	events       []eventRecord
	cursor       int          // -1 = tail-follow mode, >= 0 = selection mode
	showDetails  bool         // global toggle for expanded details
	expanded     map[int]bool // per-event expansion tracking
	width        int
	height       int
	quitting     bool
	disconnected bool // true while connection is lost and reconnecting
}

func newEventsModel(conn *client.EventConn) eventsModel {
	vp := viewport.New()
	vp.SetWidth(80)
	vp.SetHeight(20)

	sp := spinner.New()

	return eventsModel{
		viewport: vp,
		spinner:  sp,
		conn:     conn,
		cursor:   -1,
		expanded: make(map[int]bool),
	}
}

func (m eventsModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		waitForEvent(m.conn),
	)
}

func (m eventsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 3
		footerHeight := 2
		m.viewport.SetWidth(msg.Width - 4)
		m.viewport.SetHeight(msg.Height - headerHeight - footerHeight)
		m.updateContent()

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if len(m.events) > 0 {
				if m.cursor == -1 {
					m.cursor = len(m.events) - 1
				} else if m.cursor > 0 {
					m.cursor--
				}
				m.updateContent()
				m.scrollToCursor()
			}
		case "down", "j":
			if m.cursor >= 0 && m.cursor < len(m.events)-1 {
				m.cursor++
				m.updateContent()
				m.scrollToCursor()
			}
		case "enter", " ":
			if m.cursor >= 0 && m.cursor < len(m.events) {
				m.expanded[m.cursor] = !m.expanded[m.cursor]
				m.updateContent()
			}
		case "d":
			m.showDetails = !m.showDetails
			m.updateContent()
		case "esc":
			m.cursor = -1
			// Clear per-event expansions when leaving selection mode.
			m.expanded = make(map[int]bool)
			m.updateContent()
			m.viewport.GotoBottom()
		}

	case eventReceivedMsg:
		event := client.EventMessage(msg)
		switch event.Type {
		case "disconnected":
			m.disconnected = true
		case "reconnected":
			m.disconnected = false
		default:
			line := formatEvent(event)
			m.events = append(m.events, eventRecord{msg: event, compact: line})
			m.updateContent()
			if m.cursor == -1 {
				m.viewport.GotoBottom()
			}
		}
		cmds = append(cmds, waitForEvent(m.conn))

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m eventsModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	header := styles.TitleStyle.Render("PicoClaw Events") + "\n"
	separator := styles.MutedStyle.Render(strings.Repeat("─", m.width))

	var watchLine string
	if m.disconnected {
		watchLine = styles.ErrorStyle.Render("  Connection lost, reconnecting... ") + m.spinner.View()
	} else {
		watchLine = styles.MutedStyle.Render("  Watching for events... ") + m.spinner.View()
	}
	helpBar := styles.MutedStyle.Render("  q: quit  ↑↓: select  enter: expand  d: toggle details  esc: deselect")

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Padding(0, 1).
		Render(m.viewport.View())

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		separator,
		box,
		watchLine,
		helpBar,
	)

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m *eventsModel) updateContent() {
	var lines []string
	for i, rec := range m.events {
		line := rec.compact
		if m.cursor == i {
			line = lipgloss.NewStyle().Reverse(true).Bold(true).Render(line)
		}
		lines = append(lines, line)

		if m.showDetails || m.expanded[i] {
			detail := formatDetailBlock(rec.msg, m.viewport.Width())
			if detail != "" {
				lines = append(lines, detail)
			}
		}
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m *eventsModel) scrollToCursor() {
	if m.cursor < 0 {
		return
	}
	// Count lines up to cursor to approximate scroll position.
	lineCount := 0
	for i := 0; i < m.cursor; i++ {
		lineCount++ // compact line
		if m.showDetails || m.expanded[i] {
			detail := formatDetailBlock(m.events[i].msg, m.viewport.Width())
			if detail != "" {
				lineCount += strings.Count(detail, "\n") + 1
			}
		}
	}
	vpHeight := m.viewport.Height()
	yOff := m.viewport.YOffset()
	if lineCount < yOff {
		m.viewport.SetYOffset(lineCount)
	} else if lineCount >= yOff+vpHeight {
		m.viewport.SetYOffset(lineCount - vpHeight + 1)
	}
}

func waitForEvent(conn *client.EventConn) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-conn.Messages()
		if !ok {
			return tea.Quit()
		}
		return eventReceivedMsg(msg)
	}
}

// formatEvent formats an event for TUI display with inline data fields and color.
func formatEvent(event client.EventMessage) string {
	ts := time.UnixMilli(event.Timestamp).Format("15:04:05")
	typeStr := colorizeType(event.Type)
	detail := formatEventDetail(event)

	if detail != "" {
		return fmt.Sprintf("[%s] %s  %s", ts, typeStr, detail)
	}
	return fmt.Sprintf("[%s] %s", ts, typeStr)
}

// colorizeType applies color based on the event type suffix.
func colorizeType(eventType string) string {
	switch {
	case strings.HasSuffix(eventType, ".error"):
		return styles.ErrorStyle.Render(eventType)
	case strings.HasSuffix(eventType, ".completed"):
		return lipgloss.NewStyle().Foreground(styles.Success).Render(eventType)
	case strings.HasSuffix(eventType, ".started"):
		return lipgloss.NewStyle().Foreground(styles.Secondary).Render(eventType)
	default:
		return lipgloss.NewStyle().Foreground(styles.Primary).Render(eventType)
	}
}

// detailKeys are the fields shown in the compact one-liner (order matters).
var detailKeys = []string{"tool", "session", "channel", "agent", "iterations", "duration", "error"}

// detailExclude are fields excluded from the compact one-liner (shown only in expanded view).
var detailExclude = map[string]bool{
	"tool": true, "session": true, "channel": true,
	"agent": true, "iterations": true, "duration": true,
	"error": true, "isError": true,
	"arguments": true, "result": true, "message": true,
}

// formatEventDetail extracts known fields from event data and formats them inline.
func formatEventDetail(event client.EventMessage) string {
	m, ok := event.Data.(map[string]any)
	if !ok || len(m) == 0 {
		return ""
	}

	var parts []string

	for _, key := range detailKeys {
		v, exists := m[key]
		if !exists {
			continue
		}
		switch key {
		case "duration":
			if d, ok := v.(float64); ok {
				parts = append(parts, fmt.Sprintf("duration=%.1fs", d))
			}
		case "error":
			parts = append(parts, styles.ErrorStyle.Render(fmt.Sprintf("error=%v", v)))
		default:
			parts = append(parts, fmt.Sprintf("%s=%v", key, v))
		}
	}

	// Include any unknown keys sorted alphabetically (but not detail-only keys).
	var extra []string
	for k := range m {
		if !detailExclude[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		parts = append(parts, fmt.Sprintf("%s=%v", k, m[k]))
	}

	return strings.Join(parts, " ")
}

// formatDetailBlock renders an expanded detail view for one event as indented key-value pairs.
func formatDetailBlock(event client.EventMessage, width int) string {
	m, ok := event.Data.(map[string]any)
	if !ok || len(m) == 0 {
		return ""
	}

	// Collect all keys sorted, putting known detail-rich keys first.
	richKeys := []string{"arguments", "result", "message"}
	var keys []string
	for _, k := range richKeys {
		if _, exists := m[k]; exists {
			keys = append(keys, k)
		}
	}
	// Add remaining keys not already covered by the compact line or richKeys.
	var remaining []string
	richSet := map[string]bool{"arguments": true, "result": true, "message": true}
	for k := range m {
		if !richSet[k] {
			remaining = append(remaining, k)
		}
	}
	sort.Strings(remaining)
	keys = append(keys, remaining...)

	if len(keys) == 0 {
		return ""
	}

	maxValWidth := width - 8 // indent (4) + tree chars (4)
	if maxValWidth < 20 {
		maxValWidth = 20
	}

	var lines []string
	for i, key := range keys {
		val := formatDetailValue(key, m[key], maxValWidth)
		prefix := styles.MutedStyle.Render("    ├─ ")
		if i == len(keys)-1 {
			prefix = styles.MutedStyle.Render("    └─ ")
		}
		keyStr := styles.LabelStyle.Render(key + ": ")
		valStr := styles.ValueStyle.Render(val)

		firstLine := prefix + keyStr + valStr
		lines = append(lines, firstLine)
	}

	return strings.Join(lines, "\n")
}

// formatDetailValue formats a single value for the detail block.
func formatDetailValue(key string, val any, maxWidth int) string {
	switch key {
	case "arguments":
		s, ok := val.(string)
		if !ok {
			return fmt.Sprintf("%v", val)
		}
		return prettyJSON(s, maxWidth)
	case "result":
		s := fmt.Sprintf("%v", val)
		return truncateLines(s, 10, maxWidth)
	case "message":
		return fmt.Sprintf("%v", val)
	case "duration":
		if d, ok := val.(float64); ok {
			return fmt.Sprintf("%.1fs", d)
		}
		return fmt.Sprintf("%v", val)
	case "isError":
		if b, ok := val.(bool); ok {
			return fmt.Sprintf("%v", b)
		}
		return fmt.Sprintf("%v", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// prettyJSON attempts to format a JSON string with indentation.
// Falls back to the raw string if it's not valid JSON.
func prettyJSON(s string, maxWidth int) string {
	if s == "" {
		return s
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(s), "           ", "  "); err != nil {
		// Not valid JSON, return as-is (truncated).
		if len(s) > maxWidth {
			return s[:maxWidth] + "..."
		}
		return s
	}
	result := buf.String()
	return truncateLines(result, 10, maxWidth)
}

// truncateLines limits multi-line text to maxLines and individual lines to maxWidth.
func truncateLines(s string, maxLines, maxWidth int) string {
	lines := strings.Split(s, "\n")
	truncated := false
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		truncated = true
	}
	for i, line := range lines {
		if len(line) > maxWidth {
			lines[i] = line[:maxWidth] + "..."
		}
	}
	result := strings.Join(lines, "\n")
	if truncated {
		result += "\n... (truncated)"
	}
	return result
}
