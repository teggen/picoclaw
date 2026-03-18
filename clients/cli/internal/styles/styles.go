package styles

import "charm.land/lipgloss/v2"

// Color palette.
var (
	Primary    = lipgloss.Color("#7D56F4")
	Secondary  = lipgloss.Color("#4ECDC4")
	Success    = lipgloss.Color("#04B575")
	Warning    = lipgloss.Color("#FF6B6B")
	Muted      = lipgloss.Color("241")
	UserColor  = lipgloss.Color("#04B575")
	AgentColor = lipgloss.Color("#7D56F4")
)

// Reusable styles.
var (
	TitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(Primary)
	LabelStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	ValueStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	ErrorStyle  = lipgloss.NewStyle().Bold(true).Foreground(Warning)
	MutedStyle  = lipgloss.NewStyle().Foreground(Muted)
	UserBubble  = lipgloss.NewStyle().Foreground(UserColor).Bold(true)
	AgentBubble = lipgloss.NewStyle().Foreground(AgentColor).Bold(true)
	BorderStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(Primary)
)
