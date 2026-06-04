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

// TokyoNight palette excerpts used by NewStyles. These are kept package-private
// because callers should construct a Styles via NewStyles (or build their own
// Styles literal in tests) rather than reaching for colour hex codes directly.
const (
	colorBlue   = lipgloss.Color("#7aa2f7") // Title
	colorPurple = lipgloss.Color("#bb9af7") // TabActive
	colorMuted  = lipgloss.Color("#565f89") // TabInactive
	colorCyan   = lipgloss.Color("#7dcfff") // Cursor
	colorYellow = lipgloss.Color("#e0af68") // Header
	colorDim    = lipgloss.Color("#414868") // Subtle
	colorGreen  = lipgloss.Color("#9ece6a") // Good
	colorOrange = lipgloss.Color("#ff9e64") // Warn
	colorRed    = lipgloss.Color("#f7768e") // Crit
	colorLight  = lipgloss.Color("#a9b1d6") // Footer
)

// NewStyles constructs the default Styles palette. Tests can substitute their
// own palette by constructing the Styles struct directly.
//
// Base intentionally carries no foreground so that callers who embed it as a
// container around already-coloured children do not bleed an accent colour over
// the inner styles. Every other field carries a foreground drawn from the
// TokyoNight-inspired colour constants above.
func NewStyles() Styles {
	base := lipgloss.NewStyle()
	return Styles{
		Base:        base,
		Title:       base.Foreground(colorBlue).Bold(true),
		TabActive:   base.Foreground(colorPurple).Bold(true),
		TabInactive: base.Foreground(colorMuted),
		Cursor:      base.Foreground(colorCyan).Bold(true),
		Header:      base.Foreground(colorYellow).Bold(true),
		Subtle:      base.Foreground(colorDim),
		Good:        base.Foreground(colorGreen),
		Warn:        base.Foreground(colorOrange),
		Crit:        base.Foreground(colorRed).Bold(true),
		Footer:      base.Foreground(colorLight),
	}
}
