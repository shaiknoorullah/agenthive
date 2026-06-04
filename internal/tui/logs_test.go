package tui

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"
)

// TestMain forces lipgloss into termenv.Ascii so the teatest golden
// snapshots in this package do not vary by host terminal colour
// support. Sibling _test.go files may also declare TestMain; the
// integrator deduplicates once all L3 tabs are merged.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

// logsTestStyles returns a Styles palette of blank lipgloss styles so,
// combined with the ASCII colour profile pinned by TestMain, the
// rendered output is plain text and golden files stay deterministic.
// The helper is unique-named so this test file is self-sufficient even
// when sibling tab tests have not yet defined a shared testStyles
// helper.
func logsTestStyles() Styles {
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

// fixedTime is used in every test so the rendered timestamps are stable.
func fixedTime(offsetSec int) time.Time {
	return time.Date(2026, 6, 4, 12, 0, offsetSec, 0, time.UTC)
}

func sampleEntries() []LogEntry {
	return []LogEntry{
		{Timestamp: fixedTime(0), Level: "info", Source: "daemon", Message: "started libp2p host"},
		{Timestamp: fixedTime(1), Level: "warn", Source: "transport", Message: "relay reservation expired"},
		{Timestamp: fixedTime(2), Level: "crit", Source: "router", Message: "pubsub topic dropped"},
		{Timestamp: fixedTime(3), Level: "info", Source: "hooks", Message: "stop-hook delivered"},
		{Timestamp: fixedTime(4), Level: "warn", Source: "dispatch", Message: "notify-send not found"},
	}
}

// TestNewLogsModel_StartsEmpty asserts the freshly constructed model has
// no entries, no cursor offset, and an empty filter.
func TestNewLogsModel_StartsEmpty(t *testing.T) {
	t.Parallel()
	m := NewLogsModel(logsTestStyles())
	if got := len(m.entries); got != 0 {
		t.Fatalf("entries: want 0, got %d", got)
	}
	if m.cursor != 0 {
		t.Fatalf("cursor: want 0, got %d", m.cursor)
	}
	if m.filter != "" {
		t.Fatalf("filter: want empty, got %q", m.filter)
	}
}

// TestLogsModel_LogsUpdateMsg_ReplacesEntries asserts that ingesting a
// LogsUpdateMsg overwrites the entries slice and clamps the cursor to
// the new bounds.
func TestLogsModel_LogsUpdateMsg_ReplacesEntries(t *testing.T) {
	t.Parallel()
	m := NewLogsModel(logsTestStyles())
	m, _ = m.Update(LogsUpdateMsg{Entries: sampleEntries()})
	if got := len(m.entries); got != 5 {
		t.Fatalf("entries: want 5, got %d", got)
	}

	// Move cursor to bottom, then send a smaller batch; cursor must
	// clamp to len-1.
	m.cursor = 4
	m, _ = m.Update(LogsUpdateMsg{Entries: sampleEntries()[:2]})
	if m.cursor != 1 {
		t.Fatalf("cursor after shrink: want 1, got %d", m.cursor)
	}
}

// TestLogsModel_FilterKeys checks the 1 / 2 / 3 / 0 filter shortcuts.
// 1 = info, 2 = warn, 3 = crit, 0 = clear (show all).
func TestLogsModel_FilterKeys(t *testing.T) {
	t.Parallel()
	cases := []struct {
		key      string
		want     string
		expected int
	}{
		{"1", "info", 2},
		{"2", "warn", 2},
		{"3", "crit", 1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()
			m := NewLogsModel(logsTestStyles())
			m, _ = m.Update(LogsUpdateMsg{Entries: sampleEntries()})
			m, _ = m.Update(keyRune(tc.key))
			if m.filter != tc.want {
				t.Fatalf("filter: want %q, got %q", tc.want, m.filter)
			}
			if got := len(m.visible()); got != tc.expected {
				t.Fatalf("visible count: want %d, got %d", tc.expected, got)
			}
		})
	}

	// 0 clears the filter.
	m := NewLogsModel(logsTestStyles())
	m, _ = m.Update(LogsUpdateMsg{Entries: sampleEntries()})
	m, _ = m.Update(keyRune("2"))
	if m.filter != "warn" {
		t.Fatalf("setup failed: filter %q", m.filter)
	}
	m, _ = m.Update(keyRune("0"))
	if m.filter != "" {
		t.Fatalf("filter after 0: want empty, got %q", m.filter)
	}
}

// TestLogsModel_CursorJK moves the cursor with j and k and asserts it
// stays within visible bounds.
func TestLogsModel_CursorJK(t *testing.T) {
	t.Parallel()
	m := NewLogsModel(logsTestStyles())
	m, _ = m.Update(LogsUpdateMsg{Entries: sampleEntries()})

	// j moves down, k moves up.
	m, _ = m.Update(keyRune("j"))
	if m.cursor != 1 {
		t.Fatalf("after j: want 1, got %d", m.cursor)
	}
	m, _ = m.Update(keyRune("j"))
	m, _ = m.Update(keyRune("j"))
	if m.cursor != 3 {
		t.Fatalf("after jjj: want 3, got %d", m.cursor)
	}
	m, _ = m.Update(keyRune("k"))
	if m.cursor != 2 {
		t.Fatalf("after k: want 2, got %d", m.cursor)
	}

	// Cursor cannot move past the end.
	for i := 0; i < 20; i++ {
		m, _ = m.Update(keyRune("j"))
	}
	if m.cursor != 4 {
		t.Fatalf("cursor saturate down: want 4, got %d", m.cursor)
	}

	// Cursor cannot move above 0.
	for i := 0; i < 20; i++ {
		m, _ = m.Update(keyRune("k"))
	}
	if m.cursor != 0 {
		t.Fatalf("cursor saturate up: want 0, got %d", m.cursor)
	}
}

// TestLogsModel_CursorClampsToFiltered ensures that filtering trims the
// cursor when the visible slice becomes shorter than the cursor index.
func TestLogsModel_CursorClampsToFiltered(t *testing.T) {
	t.Parallel()
	m := NewLogsModel(logsTestStyles())
	m, _ = m.Update(LogsUpdateMsg{Entries: sampleEntries()})
	m.cursor = 4
	m, _ = m.Update(keyRune("3")) // only 1 crit row -> cursor must clamp.
	if m.cursor != 0 {
		t.Fatalf("cursor after filter: want 0, got %d", m.cursor)
	}
}

// TestLogsModel_WindowResize keeps the model bounded after a tiny
// terminal is reported. The view should not panic and should produce a
// non-empty render even with very small geometry.
func TestLogsModel_WindowResize(t *testing.T) {
	t.Parallel()
	m := NewLogsModel(logsTestStyles())
	m, _ = m.Update(LogsUpdateMsg{Entries: sampleEntries()})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 30, Height: 8})
	if m.width != 30 || m.height != 8 {
		t.Fatalf("size: want 30x8, got %dx%d", m.width, m.height)
	}
	v := m.View()
	if v == "" {
		t.Fatal("View() returned empty string on small size")
	}
}

// TestLogsModel_View_GoldenAllLevels is the canonical golden-file test
// driven through teatest. It feeds entries, asserts the program quits
// cleanly, and compares the captured final output against the golden.
func TestLogsModel_View_GoldenAllLevels(t *testing.T) {
	tm := teatest.NewTestModel(
		t,
		logsHarness{inner: NewLogsModel(logsTestStyles())},
		teatest.WithInitialTermSize(80, 16),
	)
	tm.Send(LogsUpdateMsg{Entries: sampleEntries()})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // move cursor to row 1
	tm.Quit()
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	out, err := io.ReadAll(tm.FinalOutput(t))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	teatest.RequireEqualOutput(t, lastFrame(out))
}

// TestLogsModel_View_GoldenCritFilter renders with the crit filter
// active so the golden documents the filtered render.
func TestLogsModel_View_GoldenCritFilter(t *testing.T) {
	tm := teatest.NewTestModel(
		t,
		logsHarness{inner: NewLogsModel(logsTestStyles())},
		teatest.WithInitialTermSize(80, 16),
	)
	tm.Send(LogsUpdateMsg{Entries: sampleEntries()})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	tm.Quit()
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	out, err := io.ReadAll(tm.FinalOutput(t))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	teatest.RequireEqualOutput(t, lastFrame(out))
}

// logsHarness wraps LogsModel to satisfy tea.Model (which needs
// (tea.Model, tea.Cmd) from Update, not the (LogsModel, tea.Cmd) shape).
type logsHarness struct{ inner LogsModel }

func (h logsHarness) Init() tea.Cmd { return h.inner.Init() }
func (h logsHarness) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		if k.Type == tea.KeyCtrlC || (k.Type == tea.KeyRunes && string(k.Runes) == "q") {
			return h, tea.Quit
		}
	}
	next, cmd := h.inner.Update(msg)
	h.inner = next
	return h, cmd
}
func (h logsHarness) View() string { return h.inner.View() }

// keyRune builds a tea.KeyMsg from a single character -- the common
// case for filter / movement keys in this tab.
func keyRune(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// lastFrame extracts the final rendered frame from a teatest output
// stream. bubbletea emits each frame separated by a clear-screen
// escape; we keep only the trailing one and strip the cursor-control
// bookends so the golden is stable across bubbletea minor releases.
func lastFrame(b []byte) []byte {
	s := string(b)
	if i := strings.LastIndex(s, "\x1b[?1049l"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, "\x1b[?1049h"); i >= 0 {
		s = s[i+len("\x1b[?1049h"):]
	}
	s = strings.ReplaceAll(s, "\x1b[?25l", "")
	s = strings.ReplaceAll(s, "\x1b[?25h", "")
	if i := strings.LastIndex(s, "\x1b[H"); i >= 0 {
		s = s[i+len("\x1b[H"):]
	}
	return bytes.TrimRight([]byte(s), "\n")
}
