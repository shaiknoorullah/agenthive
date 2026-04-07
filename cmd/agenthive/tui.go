package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shaiknoorullah/agenthive/internal/tui"
)

// runTUI launches the interactive TUI management interface.
// Called when the user runs `agenthive tui`.
func runTUI() error {
	m := tui.NewAppModel()

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}
