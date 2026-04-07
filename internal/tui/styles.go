package tui

import "github.com/charmbracelet/lipgloss"

// TokyoNight color palette.
const (
	colorBg      = "#1a1b26"
	colorFg      = "#c0caf5"
	colorBlue    = "#7aa2f7"
	colorGreen   = "#9ece6a"
	colorYellow  = "#e0af68"
	colorRed     = "#f7768e"
	colorMagenta = "#bb9af7"
	colorCyan    = "#7dcfff"
	colorMuted   = "#565f89"
	colorBgLight = "#24283b"
)

// Styles holds all lipgloss styles for the TUI.
type Styles struct {
	TabActive        lipgloss.Style
	TabInactive      lipgloss.Style
	Title            lipgloss.Style
	StatusOnline     lipgloss.Style
	StatusOffline    lipgloss.Style
	SelectedRow      lipgloss.Style
	NormalRow        lipgloss.Style
	Help             lipgloss.Style
	ActionApprove    lipgloss.Style
	ActionDeny       lipgloss.Style
	PriorityCritical lipgloss.Style
	PriorityWarning  lipgloss.Style
	PriorityInfo     lipgloss.Style
	Border           lipgloss.Style
}

// NewStyles creates a new Styles with the TokyoNight color theme.
func NewStyles() *Styles {
	return &Styles{
		TabActive: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorBlue)).
			Underline(true).
			Padding(0, 1),

		TabInactive: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMuted)).
			Padding(0, 1),

		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorCyan)).
			Padding(0, 1),

		StatusOnline: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorGreen)).
			Bold(true),

		StatusOffline: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMuted)),

		SelectedRow: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFg)).
			Background(lipgloss.Color(colorBgLight)).
			Bold(true),

		NormalRow: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFg)),

		Help: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMuted)),

		ActionApprove: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorGreen)).
			Bold(true),

		ActionDeny: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorRed)).
			Bold(true),

		PriorityCritical: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorRed)).
			Bold(true),

		PriorityWarning: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorYellow)),

		PriorityInfo: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFg)),

		Border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorBlue)),
	}
}
