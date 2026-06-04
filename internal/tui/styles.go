// Package tui implements the bubbletea-powered terminal UI that fronts the
// running agenthive daemon.
//
// styles.go owns the lipgloss style palette. The colours are loosely inspired
// by TokyoNight so the TUI feels at home in the same terminals that host the
// editor — but every Style is constructed up-front and exposed by name so
// callers never have to repeat colour literals.
package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Styles is the palette used by every TUI Model. A single Styles value is
// constructed by NewStyles and threaded into each tab Model at startup so
// the App can swap palettes by handing out a different value.
type Styles struct {
	// Base is the catch-all default style; every other Style derives from it.
	Base lipgloss.Style
	// Title styles the top banner of each tab.
	Title lipgloss.Style
	// TabActive is the lipgloss style applied to the currently focused tab
	// in the tab bar.
	TabActive lipgloss.Style
	// TabInactive is the style applied to non-focused tabs.
	TabInactive lipgloss.Style
	// Cursor highlights the row the user's cursor is on.
	Cursor lipgloss.Style
	// Header styles the column header row inside a tab.
	Header lipgloss.Style
	// Subtle styles secondary text such as relative timestamps.
	Subtle lipgloss.Style
	// Good / Warn / Crit colour the status icons on the peers and logs tabs.
	Good lipgloss.Style
	Warn lipgloss.Style
	Crit lipgloss.Style
	// Footer styles the bottom hint bar with keybinding tips.
	Footer lipgloss.Style
}

// NewStyles constructs the default Styles palette. Tests can substitute their
// own palette by constructing the Styles struct directly.
func NewStyles() Styles {
	panic("not implemented: tui.NewStyles")
}
