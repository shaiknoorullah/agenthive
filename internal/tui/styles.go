package tui

import "github.com/charmbracelet/lipgloss"

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

// NewStyles creates a new Styles with default values.
func NewStyles() *Styles {
	return nil
}
