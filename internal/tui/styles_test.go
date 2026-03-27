package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
)

func init() {
	lipgloss.DefaultRenderer().SetColorProfile(termenv.Ascii)
}

func TestNewStyles_ReturnsNonNil(t *testing.T) {
	s := NewStyles()
	assert.NotNil(t, s)
}

func TestStyles_TabActive_RendersWithBrackets(t *testing.T) {
	s := NewStyles()
	out := s.TabActive.Render("peers")
	assert.Contains(t, out, "peers")
}

func TestStyles_TabInactive_RendersWithBrackets(t *testing.T) {
	s := NewStyles()
	out := s.TabInactive.Render("routes")
	assert.Contains(t, out, "routes")
}

func TestStyles_StatusOnline_RendersText(t *testing.T) {
	s := NewStyles()
	out := s.StatusOnline.Render("online")
	assert.Contains(t, out, "online")
}

func TestStyles_StatusOffline_RendersText(t *testing.T) {
	s := NewStyles()
	out := s.StatusOffline.Render("offline")
	assert.Contains(t, out, "offline")
}

func TestStyles_Title_RendersText(t *testing.T) {
	s := NewStyles()
	out := s.Title.Render("agenthive")
	assert.Contains(t, out, "agenthive")
}

func TestStyles_SelectedRow_RendersText(t *testing.T) {
	s := NewStyles()
	out := s.SelectedRow.Render("row content")
	assert.Contains(t, out, "row content")
}

func TestStyles_Help_RendersText(t *testing.T) {
	s := NewStyles()
	out := s.Help.Render("[q]uit")
	assert.Contains(t, out, "[q]uit")
}

func TestStyles_ActionApprove_RendersText(t *testing.T) {
	s := NewStyles()
	out := s.ActionApprove.Render("[a]pprove")
	assert.Contains(t, out, "[a]pprove")
}

func TestStyles_ActionDeny_RendersText(t *testing.T) {
	s := NewStyles()
	out := s.ActionDeny.Render("[d]eny")
	assert.Contains(t, out, "[d]eny")
}

func TestStyles_PriorityColors(t *testing.T) {
	s := NewStyles()

	critical := s.PriorityCritical.Render("critical")
	assert.Contains(t, critical, "critical")

	warning := s.PriorityWarning.Render("warning")
	assert.Contains(t, warning, "warning")

	info := s.PriorityInfo.Render("info")
	assert.Contains(t, info, "info")
}
