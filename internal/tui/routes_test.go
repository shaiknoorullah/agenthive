package tui

import (
	"bytes"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
)

// testStyles returns a Styles palette whose every field is the zero
// lipgloss.Style. Combined with the package-wide TestMain that forces
// termenv.Ascii (declared in actions_test.go), this yields a plain-text
// render with no ANSI escape sequences so golden files are stable across
// hosts and terminal emulators.
//
// This helper is the canonical Styles factory shared by every *_test.go
// in this package; sibling files reference it by name.
func testStyles() Styles {
	z := lipgloss.NewStyle()
	return Styles{
		Base:        z,
		Title:       z,
		TabActive:   z,
		TabInactive: z,
		Cursor:      z,
		Header:      z,
		Subtle:      z,
		Good:        z,
		Warn:        z,
		Crit:        z,
		Footer:      z,
	}
}

// fixedRoutesUpdate returns a deterministic RoutesUpdateMsg with three
// rules whose IDs sort to a stable order: r-critical, r-phone, r-refactor.
func fixedRoutesUpdate() RoutesUpdateMsg {
	return RoutesUpdateMsg{
		Routes: map[string]crdt.RouteRule{
			"r-critical": {
				Match:   crdt.RouteMatch{Priority: "critical"},
				Targets: []string{"phone", "laptop"},
				Action:  "notify",
			},
			"r-phone": {
				Match:   crdt.RouteMatch{Project: "api-server"},
				Targets: []string{"phone"},
				Action:  "notify",
			},
			"r-refactor": {
				Match:   crdt.RouteMatch{Session: "refactor", Source: "claude-code"},
				Targets: []string{"telegram"},
				Action:  "notify+action",
			},
		},
	}
}

// routesHarness wraps a RoutesModel in a tea.Model so that teatest can
// drive it. It is only used by tests.
type routesHarness struct{ inner RoutesModel }

func (h routesHarness) Init() tea.Cmd { return h.inner.Init() }

func (h routesHarness) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := h.inner.Update(msg)
	h.inner = next
	return h, cmd
}

func (h routesHarness) View() string { return h.inner.View() }

// readAllRoutes drains a teatest output reader into a byte slice. It is
// the routes-package analogue of the helper used in peers_test.go (which
// lives under a different name to avoid duplicate symbols).
func readAllRoutes(t *testing.T, r io.Reader) []byte {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	return b
}

func TestRoutesModel_InitialRender_Empty_Golden(t *testing.T) {
	m := NewRoutesModel(testStyles())
	teatest.RequireEqualOutput(t, []byte(m.View()))
}

func TestRoutesModel_PopulatedRender_Golden(t *testing.T) {
	m := NewRoutesModel(testStyles())
	m, _ = m.Update(fixedRoutesUpdate())
	teatest.RequireEqualOutput(t, []byte(m.View()))
}

func TestRoutesModel_CursorNavigationAfterMove_Golden(t *testing.T) {
	m := NewRoutesModel(testStyles())
	m, _ = m.Update(fixedRoutesUpdate())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	teatest.RequireEqualOutput(t, []byte(m.View()))
}

func TestRoutesModel_ViewContainsRuleData(t *testing.T) {
	m := NewRoutesModel(testStyles())
	m, _ = m.Update(fixedRoutesUpdate())
	view := m.View()

	for _, want := range []string{
		"r-critical",
		"r-phone",
		"r-refactor",
		"priority:critical",
		"project:api-server",
		"phone",
		"telegram",
	} {
		if !bytes.Contains([]byte(view), []byte(want)) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func TestRoutesModel_EmptyViewMentionsAddHint(t *testing.T) {
	m := NewRoutesModel(testStyles())
	view := m.View()
	if !bytes.Contains([]byte(view), []byte("No routes")) {
		t.Errorf("expected empty-state copy 'No routes' in view, got:\n%s", view)
	}
	if !bytes.Contains([]byte(view), []byte("a")) {
		t.Errorf("expected 'a' add hint in empty view, got:\n%s", view)
	}
}

func TestRoutesModel_CursorNavigation_JK(t *testing.T) {
	m := NewRoutesModel(testStyles())
	m, _ = m.Update(fixedRoutesUpdate())

	if got := m.Cursor(); got != 0 {
		t.Fatalf("initial cursor: want 0, got %d", got)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if got := m.Cursor(); got != 1 {
		t.Fatalf("after j: want 1, got %d", got)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if got := m.Cursor(); got != 2 {
		t.Fatalf("after second j: want 2, got %d", got)
	}
	// Clamp at end.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if got := m.Cursor(); got != 2 {
		t.Fatalf("after j at end: want 2, got %d", got)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if got := m.Cursor(); got != 1 {
		t.Fatalf("after k: want 1, got %d", got)
	}
	// Clamp at start.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if got := m.Cursor(); got != 0 {
		t.Fatalf("after k at start: want 0, got %d", got)
	}
}

func TestRoutesModel_CursorNavigation_Arrows(t *testing.T) {
	m := NewRoutesModel(testStyles())
	m, _ = m.Update(fixedRoutesUpdate())

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := m.Cursor(); got != 1 {
		t.Fatalf("after down arrow: want 1, got %d", got)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := m.Cursor(); got != 0 {
		t.Fatalf("after up arrow: want 0, got %d", got)
	}
}

func TestRoutesModel_DeleteEmitsRequest(t *testing.T) {
	m := NewRoutesModel(testStyles())
	m, _ = m.Update(fixedRoutesUpdate())
	// Move to the second row (r-phone) then press d.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatal("d on populated routes: expected a tea.Cmd, got nil")
	}
	msg := cmd()
	req, ok := msg.(RouteDeleteRequestMsg)
	if !ok {
		t.Fatalf("d cmd: expected RouteDeleteRequestMsg, got %T (%+v)", msg, msg)
	}
	if req.ID != "r-phone" {
		t.Fatalf("RouteDeleteRequestMsg.ID: want r-phone, got %q", req.ID)
	}
}

func TestRoutesModel_DeleteOnEmptyDoesNothing(t *testing.T) {
	m := NewRoutesModel(testStyles())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("d on empty routes: expected nil cmd, got %T", cmd())
	}
}

func TestRoutesModel_AddEmitsRequest(t *testing.T) {
	m := NewRoutesModel(testStyles())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if cmd == nil {
		t.Fatal("a key: expected a tea.Cmd to surface add intent, got nil")
	}
	msg := cmd()
	if _, ok := msg.(RouteAddRequestMsg); !ok {
		t.Fatalf("a cmd: expected RouteAddRequestMsg, got %T", msg)
	}
}

func TestRoutesModel_WindowResize(t *testing.T) {
	m := NewRoutesModel(testStyles())
	m, _ = m.Update(fixedRoutesUpdate())

	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	if w := m.Width(); w != 40 {
		t.Fatalf("after resize width: want 40, got %d", w)
	}
	if h := m.Height(); h != 10 {
		t.Fatalf("after resize height: want 10, got %d", h)
	}

	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	view := m.View()
	if !bytes.Contains([]byte(view), []byte("r-critical")) {
		t.Errorf("expected route id 'r-critical' in resized view, got:\n%s", view)
	}
}

func TestRoutesModel_UpdateClampsCursor(t *testing.T) {
	m := NewRoutesModel(testStyles())
	m, _ = m.Update(fixedRoutesUpdate())
	// Move cursor to last row.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if got := m.Cursor(); got != 2 {
		t.Fatalf("setup cursor: want 2, got %d", got)
	}

	// Shrink to one route; cursor must clamp to 0.
	shrunk := RoutesUpdateMsg{
		Routes: map[string]crdt.RouteRule{
			"r-only": {Match: crdt.RouteMatch{Project: "only"}, Targets: []string{"phone"}},
		},
	}
	m, _ = m.Update(shrunk)
	if got := m.Cursor(); got != 0 {
		t.Fatalf("after shrink: want cursor 0, got %d", got)
	}
}

func TestRoutesModel_TeatestDriver(t *testing.T) {
	app := routesHarness{inner: NewRoutesModel(testStyles())}

	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(80, 24))
	tm.Send(fixedRoutesUpdate())
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	if err := tm.Quit(); err != nil {
		t.Fatalf("quit: %v", err)
	}
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	out := readAllRoutes(t, tm.FinalOutput(t))
	for _, want := range []string{"r-critical", "r-phone", "r-refactor"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("teatest output missing %q:\n%s", want, out)
		}
	}
}
