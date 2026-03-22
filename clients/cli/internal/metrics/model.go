package metrics

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/sipeed/picoclaw/clients/cli/internal/client"
	"github.com/sipeed/picoclaw/clients/cli/internal/styles"
)

// Bubble Tea messages.
type (
	tickMsg       struct{}
	metricsMsg    *MetricsSnapshot
	metricsErrMsg struct{ error }
)

type model struct {
	client   *client.Client
	interval time.Duration

	current *MetricsSnapshot
	prev    *MetricsSnapshot

	activeTab int
	panels    [4]bool

	settingsOpen     bool
	settingsForm     *huh.Form
	selectedPanels   *[]int
	selectedInterval *time.Duration

	width    int
	height   int
	err      error
	quitting bool
}

func newModel(c *client.Client, interval time.Duration) model {
	return model{
		client:   c,
		interval: interval,
		panels:   [4]bool{true, true, true, true},
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		fetchMetrics(m.client),
		tickCmd(m.interval),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		if m.settingsOpen {
			return m.updateSettings(msg)
		}
		return m.updateKeys(msg)

	case tickMsg:
		return m, tea.Batch(fetchMetrics(m.client), tickCmd(m.interval))

	case metricsMsg:
		m.prev = m.current
		m.current = (*MetricsSnapshot)(msg)
		m.err = nil
		return m, nil

	case metricsErrMsg:
		m.err = msg.error
		return m, nil
	}

	return m, nil
}

func (m model) updateKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "tab":
		m.activeTab = (m.activeTab + 1) % 4
	case "shift+tab":
		m.activeTab = (m.activeTab + 3) % 4
	case "s":
		m.settingsOpen = true
		form, sp, si := newSettingsForm(m.panels, m.interval)
		m.settingsForm = form
		m.selectedPanels = sp
		m.selectedInterval = si
		return m, m.settingsForm.Init()
	case "1":
		m.panels[0] = !m.panels[0]
	case "2":
		m.panels[1] = !m.panels[1]
	case "3":
		m.panels[2] = !m.panels[2]
	case "4":
		m.panels[3] = !m.panels[3]
	}
	return m, nil
}

func (m model) updateSettings(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.settingsOpen = false
		return m, nil
	}

	form, cmd := m.settingsForm.Update(msg)
	m.settingsForm = form.(*huh.Form)

	if m.settingsForm.State == huh.StateCompleted {
		result := applySettings(*m.selectedPanels, *m.selectedInterval)
		m.panels = result.panels
		m.interval = result.interval
		m.settingsOpen = false
	}

	return m, cmd
}

func (m model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	// Header
	header := styles.TitleStyle.Render("PicoClaw Metrics Dashboard")
	if m.current != nil {
		header += styles.MutedStyle.Render(
			fmt.Sprintf("  (updated %s)", m.current.Timestamp.Format("15:04:05")))
	}
	if m.err != nil {
		header += "  " + styles.ErrorStyle.Render(fmt.Sprintf("error: %v", m.err))
	}

	separator := styles.MutedStyle.Render(strings.Repeat("─", m.width))

	// Help bar
	help := styles.MutedStyle.Render("  q: quit  tab: cycle panels  1-4: toggle  s: settings")

	// Calculate panel dimensions
	contentHeight := m.height - 4 // header + separator + help + padding
	if contentHeight < 4 {
		contentHeight = 4
	}

	var content string
	if m.width < 80 {
		content = m.renderVertical(contentHeight)
	} else {
		content = m.renderGrid(contentHeight)
	}

	// Settings overlay
	if m.settingsOpen {
		overlay := lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(styles.Secondary).
			Padding(1, 2).
			Render(m.settingsForm.View())
		content = lipgloss.Place(m.width, contentHeight, lipgloss.Center, lipgloss.Center, overlay)
	}

	result := lipgloss.JoinVertical(lipgloss.Left, header, separator, content, help)

	v := tea.NewView(result)
	v.AltScreen = true
	return v
}

func (m model) renderGrid(totalHeight int) string {
	panelW := (m.width - 2) / 2 // account for border padding
	if panelW < 20 {
		panelW = 20
	}
	panelH := (totalHeight - 1) / 2
	if panelH < 4 {
		panelH = 4
	}

	var topPanels, bottomPanels []string

	if m.panels[0] {
		topPanels = append(topPanels, renderLLMPanel(m.current, m.prev, panelW, panelH, m.activeTab == 0))
	}
	if m.panels[1] {
		topPanels = append(topPanels, renderToolPanel(m.current, m.prev, panelW, panelH, m.activeTab == 1))
	}
	if m.panels[2] {
		bottomPanels = append(bottomPanels,
			renderMessagePanel(m.current, m.prev, panelW, panelH, m.activeTab == 2))
	}
	if m.panels[3] {
		bottomPanels = append(bottomPanels, renderSystemPanel(m.current, panelW, panelH, m.activeTab == 3))
	}

	var rows []string
	if len(topPanels) > 0 {
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, topPanels...))
	}
	if len(bottomPanels) > 0 {
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, bottomPanels...))
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m model) renderVertical(totalHeight int) string {
	var visible int
	for _, v := range m.panels {
		if v {
			visible++
		}
	}
	if visible == 0 {
		return styles.MutedStyle.Render("  All panels hidden. Press 1-4 to toggle.")
	}

	panelH := totalHeight / visible
	if panelH < 4 {
		panelH = 4
	}
	panelW := m.width - 2

	var panels []string
	if m.panels[0] {
		panels = append(panels, renderLLMPanel(m.current, m.prev, panelW, panelH, m.activeTab == 0))
	}
	if m.panels[1] {
		panels = append(panels, renderToolPanel(m.current, m.prev, panelW, panelH, m.activeTab == 1))
	}
	if m.panels[2] {
		panels = append(panels, renderMessagePanel(m.current, m.prev, panelW, panelH, m.activeTab == 2))
	}
	if m.panels[3] {
		panels = append(panels, renderSystemPanel(m.current, panelW, panelH, m.activeTab == 3))
	}

	return lipgloss.JoinVertical(lipgloss.Left, panels...)
}

func fetchMetrics(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		raw, err := c.GetMetricsRaw()
		if err != nil {
			return metricsErrMsg{err}
		}
		snap, err := ParseMetrics(bytes.NewReader(raw))
		if err != nil {
			return metricsErrMsg{err}
		}
		return metricsMsg(snap)
	}
}

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}
