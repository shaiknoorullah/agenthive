package tui

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// NB: TestMain — which forces lipgloss into termenv.Ascii so golden files
// stay deterministic — is declared by logs_test.go. The shared helper
// testStyles() lives in routes_test.go. This file deliberately uses a
// tab-scoped helper (actionsTestStyles) for the actions tab so it never
// depends on sibling files compiling.

// actionsTestStyles returns a Styles palette whose every field is the zero
// lipgloss.Style. Combined with the package-wide Ascii profile this yields a
// plain-text render with no ANSI escape sequences so the actions golden files
// are stable across hosts.
//
// A dedicated helper is used (rather than the shared testStyles() defined by
// routes_test.go) so this file remains self-contained: re-ordering or
// removing sibling test files cannot make the actions tests fail to compile.
func actionsTestStyles() Styles {
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

// actionsReadAll drains a teatest output reader. Kept package-private with an
// actions-scoped name so it does not collide with helpers defined by sibling
// test files (the peers tests use peersReadAll, the routes tests use
// readAllRoutes).
func actionsReadAll(t *testing.T, r io.Reader) []byte {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	return b
}

// twoActionsUpdate returns an ActionsUpdateMsg containing a deterministic set
// of two pending actions so cursor / golden tests have stable ordering.
func twoActionsUpdate() ActionsUpdateMsg {
	ts := time.Date(2026, time.June, 4, 10, 0, 0, 0, time.UTC)
	return ActionsUpdateMsg{
		Actions: []protocols.ActionRequest{
			{
				ActionID:  "act-001",
				SessionID: "sess-a",
				ToolName:  "Bash",
				ToolInput: "rm -rf /tmp/foo",
				Project:   "agenthive",
				Timestamp: ts,
				ExpiresAt: ts.Add(2 * time.Minute),
			},
			{
				ActionID:  "act-002",
				SessionID: "sess-b",
				ToolName:  "Write",
				ToolInput: "echo hi > /tmp/bar",
				Project:   "agenthive",
				Timestamp: ts,
				ExpiresAt: ts.Add(2 * time.Minute),
			},
		},
	}
}

// recordingQueue captures decisions written through it and signals each write
// on a buffered channel so tests can wait deterministically without sleeps.
type recordingQueue struct {
	mu       sync.Mutex
	written  []protocols.ActionResponse
	notifyCh chan protocols.ActionResponse
}

func newRecordingQueue() *recordingQueue {
	return &recordingQueue{notifyCh: make(chan protocols.ActionResponse, 16)}
}

func (q *recordingQueue) WriteResponse(resp protocols.ActionResponse) error {
	q.mu.Lock()
	q.written = append(q.written, resp)
	q.mu.Unlock()
	select {
	case q.notifyCh <- resp:
	default:
	}
	return nil
}

func (q *recordingQueue) snapshot() []protocols.ActionResponse {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]protocols.ActionResponse, len(q.written))
	copy(out, q.written)
	return out
}

// containsAllSubs asserts that out contains every substring in needles. The
// helper is renamed away from the obvious "containsAll" to avoid colliding
// with helpers a sibling file might define.
func containsAllSubs(t *testing.T, out []byte, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !bytes.Contains(out, []byte(n)) {
			t.Fatalf("expected output to contain %q\n--- output ---\n%s", n, out)
		}
	}
}

// containsNoneSubs asserts that out contains none of the supplied substrings.
func containsNoneSubs(t *testing.T, out []byte, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if bytes.Contains(out, []byte(n)) {
			t.Fatalf("did not expect output to contain %q\n--- output ---\n%s", n, out)
		}
	}
}

func TestActionsModel_InitialEmpty_RendersEmptyState(t *testing.T) {
	m := NewActionsModel(actionsTestStyles())
	out := m.View()
	if out == "" {
		t.Fatal("View() must not return an empty string even with no actions")
	}
	containsAllSubs(t, []byte(out), "Pending Actions", "No pending actions")
}

func TestActionsModel_InitialEmpty_Golden(t *testing.T) {
	m := NewActionsModel(actionsTestStyles())
	teatest.RequireEqualOutput(t, []byte(m.View()))
}

func TestActionsModel_UpdateInjectsPending(t *testing.T) {
	m := NewActionsModel(actionsTestStyles())
	upd, _ := m.Update(twoActionsUpdate())
	got := upd.View()
	containsAllSubs(t, []byte(got),
		"act-001",
		"act-002",
		"Bash",
		"Write",
		"rm -rf /tmp/foo",
	)
	containsNoneSubs(t, []byte(got), "No pending actions")
}

func TestActionsModel_PopulatedRender_Golden(t *testing.T) {
	m := NewActionsModel(actionsTestStyles())
	next, _ := m.Update(twoActionsUpdate())
	teatest.RequireEqualOutput(t, []byte(next.View()))
}

func TestActionsModel_CursorNavigation_JK(t *testing.T) {
	m := NewActionsModel(actionsTestStyles())
	m, _ = m.Update(twoActionsUpdate())

	if got := m.Cursor(); got != 0 {
		t.Fatalf("initial cursor: want 0, got %d", got)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if got := m.Cursor(); got != 1 {
		t.Fatalf("after j: want 1, got %d", got)
	}

	// j again clamps at the last row.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if got := m.Cursor(); got != 1 {
		t.Fatalf("after j at end: want 1, got %d", got)
	}

	// k moves back up.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if got := m.Cursor(); got != 0 {
		t.Fatalf("after k: want 0, got %d", got)
	}

	// k at top clamps.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if got := m.Cursor(); got != 0 {
		t.Fatalf("after k at top: want 0, got %d", got)
	}
}

func TestActionsModel_CursorNavigation_Arrows(t *testing.T) {
	m := NewActionsModel(actionsTestStyles())
	m, _ = m.Update(twoActionsUpdate())

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := m.Cursor(); got != 1 {
		t.Fatalf("after down arrow: want 1, got %d", got)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := m.Cursor(); got != 0 {
		t.Fatalf("after up arrow: want 0, got %d", got)
	}
}

func TestActionsModel_WindowResize(t *testing.T) {
	m := NewActionsModel(actionsTestStyles())
	m, _ = m.Update(twoActionsUpdate())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})

	if w := m.Width(); w != 40 {
		t.Fatalf("after resize width: want 40, got %d", w)
	}
	if h := m.Height(); h != 10 {
		t.Fatalf("after resize height: want 10, got %d", h)
	}

	// View() must still render and contain action IDs after a resize.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	view := m.View()
	if !bytes.Contains([]byte(view), []byte("act-001")) {
		t.Errorf("expected action 'act-001' in resized view, got:\n%s", view)
	}
	if !bytes.Contains([]byte(view), []byte("act-002")) {
		t.Errorf("expected action 'act-002' in resized view, got:\n%s", view)
	}
}

func TestActionsModel_ApproveYWritesAllowAndRemovesRow(t *testing.T) {
	q := newRecordingQueue()
	m := NewActionsModel(actionsTestStyles()).WithQueue(q)
	m, _ = m.Update(twoActionsUpdate())

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	if cmd == nil {
		t.Fatal("expected a tea.Cmd from approve key, got nil")
	}
	msg := cmd()
	dec, ok := msg.(ActionDecisionMsg)
	if !ok {
		t.Fatalf("expected ActionDecisionMsg, got %T", msg)
	}
	if dec.ActionID != "act-001" || dec.Decision != "allow" {
		t.Fatalf("unexpected decision: %+v", dec)
	}

	// The queue should have received exactly one allow record.
	select {
	case resp := <-q.notifyCh:
		if resp.ActionID != "act-001" || resp.Decision != "allow" {
			t.Fatalf("queue received %+v", resp)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("queue did not receive a decision in time")
	}

	if got := len(q.snapshot()); got != 1 {
		t.Fatalf("queue should have exactly 1 entry, got %d", got)
	}

	// After approval the action is removed from the visible list.
	out := m.View()
	containsNoneSubs(t, []byte(out), "act-001")
	containsAllSubs(t, []byte(out), "act-002")
}

func TestActionsModel_DenyNWritesDenyAndRemovesRow(t *testing.T) {
	q := newRecordingQueue()
	m := NewActionsModel(actionsTestStyles()).WithQueue(q)
	m, _ = m.Update(twoActionsUpdate())

	// Move cursor to second row then deny it.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if cmd == nil {
		t.Fatal("expected a tea.Cmd from deny key, got nil")
	}
	msg := cmd()
	dec, ok := msg.(ActionDecisionMsg)
	if !ok {
		t.Fatalf("expected ActionDecisionMsg, got %T", msg)
	}
	if dec.ActionID != "act-002" || dec.Decision != "deny" {
		t.Fatalf("unexpected decision: %+v", dec)
	}

	select {
	case resp := <-q.notifyCh:
		if resp.ActionID != "act-002" || resp.Decision != "deny" {
			t.Fatalf("queue received %+v", resp)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("queue did not receive a decision in time")
	}

	out := m.View()
	containsNoneSubs(t, []byte(out), "act-002")
	containsAllSubs(t, []byte(out), "act-001")
}

func TestActionsModel_DecideOnEmptyListIsNoop(t *testing.T) {
	q := newRecordingQueue()
	m := NewActionsModel(actionsTestStyles()).WithQueue(q)

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd != nil {
		// A nil-returning cmd is also acceptable; if non-nil it must produce nil msg.
		if msg := cmd(); msg != nil {
			t.Fatalf("expected nil decision on empty list, got %T", msg)
		}
	}
	if got := len(q.snapshot()); got != 0 {
		t.Fatalf("queue should remain empty, got %d entries", got)
	}
	if m.View() == "" {
		t.Fatal("empty View must still render a non-empty placeholder")
	}
}

func TestActionsModel_WithoutQueue_ApproveStillEmitsMsg(t *testing.T) {
	// No queue wired: ActionsModel should still produce a decision message
	// so the parent App can route it to whatever queue it owns.
	m := NewActionsModel(actionsTestStyles())
	m, _ = m.Update(twoActionsUpdate())

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("expected ActionDecisionMsg cmd even without queue")
	}
	msg := cmd()
	if _, ok := msg.(ActionDecisionMsg); !ok {
		t.Fatalf("expected ActionDecisionMsg, got %T", msg)
	}
}

func TestActionsModel_UpdateClampsCursor(t *testing.T) {
	m := NewActionsModel(actionsTestStyles())
	m, _ = m.Update(twoActionsUpdate())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if got := m.Cursor(); got != 1 {
		t.Fatalf("setup: want 1, got %d", got)
	}

	// Shrink the pending set to one entry; cursor must clamp back to 0.
	shrunk := ActionsUpdateMsg{
		Actions: []protocols.ActionRequest{
			{ActionID: "act-001", ToolName: "Bash", ToolInput: "ls"},
		},
	}
	m, _ = m.Update(shrunk)
	if got := m.Cursor(); got != 0 {
		t.Fatalf("after shrink: want 0, got %d", got)
	}
}

// actionsTestApp is a thin tea.Model wrapper around ActionsModel so it can be
// driven by teatest. It is test-only.
type actionsTestApp struct{ inner ActionsModel }

func (a actionsTestApp) Init() tea.Cmd { return a.inner.Init() }

func (a actionsTestApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := a.inner.Update(msg)
	a.inner = next
	return a, cmd
}

func (a actionsTestApp) View() string { return a.inner.View() }

func TestActionsModel_TeatestDriver(t *testing.T) {
	app := actionsTestApp{inner: NewActionsModel(actionsTestStyles())}

	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(80, 24))
	tm.Send(twoActionsUpdate())
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	if err := tm.Quit(); err != nil {
		t.Fatalf("quit: %v", err)
	}
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	out := actionsReadAll(t, tm.FinalOutput(t))
	if !bytes.Contains(out, []byte("act-001")) {
		t.Errorf("teatest output missing 'act-001':\n%s", out)
	}
	if !bytes.Contains(out, []byte("act-002")) {
		t.Errorf("teatest output missing 'act-002':\n%s", out)
	}
}

func TestActionsModel_CursorNavigationAfterMove_Golden(t *testing.T) {
	m := NewActionsModel(actionsTestStyles())
	m, _ = m.Update(twoActionsUpdate())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	teatest.RequireEqualOutput(t, []byte(m.View()))
}
