package chat

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sipeed/picoclaw/clients/cli/internal/client"
	"github.com/sipeed/picoclaw/clients/cli/internal/styles"
)

type chatMessage struct {
	Role    string
	Content string
}

// Bubble Tea messages.
type (
	wsMessageMsg      client.APIMessage
	wsDisconnectedMsg struct{}
	wsReconnectedMsg  struct{}
	errMsg            struct{ err error }
)

type chatModel struct {
	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model

	messages    []chatMessage
	conn        *client.ChatConn
	apiClient   *client.Client
	sessionID   string
	typing      bool
	err         error
	width       int
	height      int
	quitting    bool
	sentHistory []string // previously sent messages
	historyIdx  int      // current position; len(sentHistory) = "not browsing"
	savedInput  string   // saves current textarea content when entering history mode
}

func newChatModel(conn *client.ChatConn, apiClient *client.Client, sessionID string) chatModel {
	ta := textarea.New()
	ta.Prompt = "> "
	ta.SetHeight(3)
	ta.Focus()
	ta.ShowLineNumbers = false

	// Shift+Enter inserts newline; Up/Down reserved for history navigation.
	ta.KeyMap.InsertNewline.SetKeys("shift+enter", "ctrl+j")
	ta.KeyMap.LineNext.SetEnabled(false)
	ta.KeyMap.LinePrevious.SetEnabled(false)

	vp := viewport.New()
	vp.SetWidth(80)
	vp.SetHeight(20)

	// Disable arrow/letter-based viewport scrolling; keep only PageUp/PageDown.
	vp.KeyMap.Up.SetEnabled(false)
	vp.KeyMap.Down.SetEnabled(false)
	vp.KeyMap.HalfPageUp.SetEnabled(false)
	vp.KeyMap.HalfPageDown.SetEnabled(false)
	vp.KeyMap.PageUp.SetKeys("pgup")
	vp.KeyMap.PageDown.SetKeys("pgdown")

	sp := spinner.New()

	return chatModel{
		viewport:  vp,
		textarea:  ta,
		spinner:   sp,
		conn:      conn,
		apiClient: apiClient,
		sessionID: sessionID,
	}
}

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		waitForMessage(m.conn),
	)
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 2
		inputHeight := 5
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(msg.Height - headerHeight - inputHeight)
		m.textarea.SetWidth(msg.Width - 2)
		m.updateViewportContent()

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+d":
			m.quitting = true
			return m, tea.Quit
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "up":
			if len(m.sentHistory) == 0 {
				return m, nil
			}
			if m.historyIdx == len(m.sentHistory) {
				m.savedInput = m.textarea.Value()
			}
			if m.historyIdx > 0 {
				m.historyIdx--
				m.textarea.SetValue(m.sentHistory[m.historyIdx])
			}
			return m, nil
		case "down":
			if m.historyIdx >= len(m.sentHistory) {
				return m, nil
			}
			m.historyIdx++
			if m.historyIdx == len(m.sentHistory) {
				m.textarea.SetValue(m.savedInput)
			} else {
				m.textarea.SetValue(m.sentHistory[m.historyIdx])
			}
			return m, nil
		case "enter":
			text := strings.TrimSpace(m.textarea.Value())
			if text == "" {
				return m, nil
			}
			m.textarea.Reset()

			// Record in history.
			m.sentHistory = append(m.sentHistory, text)
			m.historyIdx = len(m.sentHistory)
			m.savedInput = ""

			// Handle in-chat commands.
			if strings.HasPrefix(text, "/") {
				return m.handleCommand(text)
			}

			// Send message.
			m.messages = append(m.messages, chatMessage{Role: "user", Content: text})
			m.updateViewportContent()
			m.viewport.GotoBottom()

			if err := m.conn.Send(text); err != nil {
				m.err = err
			}
			return m, waitForMessage(m.conn)
		}

	case wsMessageMsg:
		apiMsg := client.APIMessage(msg)
		switch apiMsg.Type {
		case client.TypeTypingStart:
			m.typing = true
			cmds = append(cmds, m.spinner.Tick)
		case client.TypeTypingStop:
			m.typing = false
		case client.TypeMessageCreate:
			m.typing = false
			content, _ := apiMsg.Payload["content"].(string)
			if content != "" {
				m.messages = append(m.messages, chatMessage{Role: "assistant", Content: content})
			}
			if apiMsg.SessionID != "" && m.sessionID == "" {
				m.sessionID = apiMsg.SessionID
			}
		case client.TypeMessageUpdate:
			content, _ := apiMsg.Payload["content"].(string)
			if content != "" && len(m.messages) > 0 {
				last := &m.messages[len(m.messages)-1]
				if last.Role == "assistant" {
					last.Content = content
				}
			}
		case client.TypeError:
			errMsg, _ := apiMsg.Payload["message"].(string)
			if errMsg != "" {
				m.messages = append(m.messages, chatMessage{
					Role:    "system",
					Content: "Error: " + errMsg,
				})
			}
		case "reconnected":
			m.messages = append(m.messages, chatMessage{
				Role:    "system",
				Content: "Reconnected to gateway.",
			})
		}
		m.updateViewportContent()
		m.viewport.GotoBottom()
		cmds = append(cmds, waitForMessage(m.conn))

	case wsDisconnectedMsg:
		m.messages = append(m.messages, chatMessage{
			Role:    "system",
			Content: "Disconnected from gateway.",
		})
		m.updateViewportContent()
		m.viewport.GotoBottom()

	case spinner.TickMsg:
		if m.typing {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case errMsg:
		m.err = msg.err
	}

	// Update sub-models.
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m chatModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	header := styles.TitleStyle.Render("PicoClaw Chat")
	if m.sessionID != "" {
		sid := m.sessionID
		if len(sid) > 8 {
			sid = sid[:8]
		}
		header += styles.MutedStyle.Render(" — Session: " + sid)
	}
	header += "\n"

	separator := styles.MutedStyle.Render(strings.Repeat("─", m.width))

	input := m.textarea.View()
	helpBar := styles.MutedStyle.Render(
		"  ctrl+d: quit  /help: commands  ↑↓: history  pgup/dn: scroll  ctrl+j: newline",
	)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		separator,
		m.viewport.View(),
		separator,
		input,
		helpBar,
	)

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m *chatModel) updateViewportContent() {
	var sb strings.Builder
	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			sb.WriteString(styles.UserBubble.Render("you: "))
			sb.WriteString(msg.Content)
			sb.WriteString("\n\n")
		case "assistant":
			sb.WriteString(styles.AgentBubble.Render("assistant: "))
			sb.WriteString(msg.Content)
			sb.WriteString("\n\n")
		case "system":
			sb.WriteString(styles.MutedStyle.Render("• " + msg.Content))
			sb.WriteString("\n\n")
		}
	}
	if m.typing {
		sb.WriteString(styles.MutedStyle.Render(m.spinner.View() + " typing..."))
		sb.WriteString("\n")
	}
	m.viewport.SetContent(sb.String())
}

func (m chatModel) handleCommand(text string) (tea.Model, tea.Cmd) {
	switch text {
	case "/quit", "/exit":
		m.quitting = true
		return m, tea.Quit
	case "/session":
		sid := m.sessionID
		if sid == "" {
			sid = "(no session)"
		}
		m.messages = append(m.messages, chatMessage{
			Role:    "system",
			Content: "Session ID: " + sid,
		})
	case "/new":
		m.messages = nil
		m.sessionID = ""
		m.conn.Close()
		gwURL := m.apiClient.BaseURL
		conn, err := client.DialChat(gwURL, "")
		if err != nil {
			m.messages = append(m.messages, chatMessage{
				Role:    "system",
				Content: fmt.Sprintf("Failed to start new session: %v", err),
			})
		} else {
			m.conn = conn
			m.messages = append(m.messages, chatMessage{
				Role:    "system",
				Content: "Started new session.",
			})
		}
	case "/clear":
		m.messages = nil
	case "/help":
		m.messages = append(m.messages, chatMessage{
			Role: "system",
			Content: "Commands:\n" +
				"  /quit, /exit  — quit chat\n" +
				"  /session      — show session ID\n" +
				"  /new          — start new session\n" +
				"  /clear        — clear messages\n" +
				"  /help         — show this help",
		})
	default:
		m.messages = append(m.messages, chatMessage{
			Role:    "system",
			Content: "Unknown command: " + text,
		})
	}
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return m, waitForMessage(m.conn)
}

func waitForMessage(conn *client.ChatConn) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-conn.Messages()
		if !ok {
			return wsDisconnectedMsg{}
		}
		if msg.Type == "reconnected" {
			return wsMessageMsg(msg)
		}
		return wsMessageMsg(msg)
	}
}
