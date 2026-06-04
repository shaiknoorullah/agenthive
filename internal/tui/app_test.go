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

// NB: TestMain that forces lipgloss into termenv.Ascii is declared by
// logs_test.go. The shared blank-style helper testStyles() lives in
// routes_test.go. This file uses an app-scoped helper so it stays
// self-sufficient if sibling test files are re-ordered.

// appTestStyles returns a Styles palette whose every field is the zero
// lipgloss.Style. Combined with the package-wide Ascii profile this yields a
// plain-text render with no ANSI escape sequences so the app golden files
// are stable across hosts.
func appTestStyles() Styles {
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

// appReadAll drains a teatest output reader.
func appReadAll(t *testing.T, r io.Reader) []byte {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	return b
}

func TestApp_NewApp_StartsOnPeersTab(t *testing.T) {
	a := NewApp(appTestStyles())
	if got := a.Active(); got != TabPeers {
		t.Fatalf("initial active tab: want TabPeers (%d), got %d", TabPeers, got)
	}
}

func TestApp_Init_NoPanic(t *testing.T) {
	a := NewApp(appTestStyles())
	// Init must be safe to call; the value of the returned cmd is unconstrained.
	_ = a.Init()
}

func TestApp_TabSwitchKeys(t *testing.T) {
	cases := []struct {
		key  rune
		want Tab
	}{
		{'p', TabPeers},
		{'r', TabRoutes},
		{'a', TabActions},
		{'l', TabLogs},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.key), func(t *testing.T) {
			a := NewApp(appTestStyles())
			next, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tc.key}})
			app, ok := next.(App)
			if !ok {
				t.Fatalf("Update returned %T, want App", next)
			}
			if got := app.Active(); got != tc.want {
				t.Fatalf("after key %q: want active %d, got %d", tc.key, tc.want, got)
			}
		})
	}
}

func TestApp_QuitKeys(t *testing.T) {
	// 'q' returns tea.Quit
	a := NewApp(appTestStyles())
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q key: expected tea.Quit cmd, got nil")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("q key cmd: expected tea.QuitMsg, got %T", msg)
	}

	// ctrl+c returns tea.Quit
	a = NewApp(appTestStyles())
	_, cmd = a.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c: expected tea.Quit cmd, got nil")
	}
	msg = cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c cmd: expected tea.QuitMsg, got %T", msg)
	}
}

func TestApp_WindowResize_BroadcastsToAllTabs(t *testing.T) {
	a := NewApp(appTestStyles())
	next, _ := a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app, ok := next.(App)
	if !ok {
		t.Fatalf("Update returned %T", next)
	}
	if w := app.Width(); w != 120 {
		t.Fatalf("App.Width(): want 120, got %d", w)
	}
	if h := app.Height(); h != 40 {
		t.Fatalf("App.Height(): want 40, got %d", h)
	}
	if w := app.peers.Width(); w != 120 {
		t.Errorf("peers width: want 120, got %d", w)
	}
	if w := app.routes.Width(); w != 120 {
		t.Errorf("routes width: want 120, got %d", w)
	}
	if w := app.actions.Width(); w != 120 {
		t.Errorf("actions width: want 120, got %d", w)
	}
	if h := app.logs.height; h != 40 {
		t.Errorf("logs height: want 40, got %d", h)
	}
}

func TestApp_View_InitialContainsPeersTabBar(t *testing.T) {
	a := NewApp(appTestStyles())
	view := a.View()

	// Tab bar must mention every tab label.
	for _, want := range []string{"peers", "routes", "actions", "logs"} {
		if !bytes.Contains([]byte(view), []byte(want)) {
			t.Errorf("View missing tab label %q:\n%s", want, view)
		}
	}
	// Initial render must show the Peers tab body.
	if !bytes.Contains([]byte(view), []byte("Peers")) {
		t.Errorf("initial View missing Peers tab body:\n%s", view)
	}
}

func TestApp_View_AfterRSwitchesToRoutesBody(t *testing.T) {
	a := NewApp(appTestStyles())
	next, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	app := next.(App)
	view := app.View()
	// Routes tab renders "ID" + "SELECTOR" header and an empty hint.
	if !bytes.Contains([]byte(view), []byte("SELECTOR")) {
		t.Errorf("after r: expected SELECTOR header in Routes body:\n%s", view)
	}
	if !bytes.Contains([]byte(view), []byte("No routes")) {
		t.Errorf("after r: expected 'No routes' hint in Routes body:\n%s", view)
	}
}

func TestApp_View_AfterASwitchesToActionsBody(t *testing.T) {
	a := NewApp(appTestStyles())
	next, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	app := next.(App)
	view := app.View()
	if !bytes.Contains([]byte(view), []byte("Pending Actions")) {
		t.Errorf("after a: expected 'Pending Actions' title:\n%s", view)
	}
}

func TestApp_View_AfterLSwitchesToLogsBody(t *testing.T) {
	a := NewApp(appTestStyles())
	next, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	app := next.(App)
	view := app.View()
	if !bytes.Contains([]byte(view), []byte("logs")) {
		t.Errorf("after l: expected 'logs' title:\n%s", view)
	}
}

func TestApp_Update_RoutesActiveTabReceivesKeys(t *testing.T) {
	// Switch to the routes tab and populate it. Pressing `d` on a populated
	// routes tab must reach the RoutesModel and surface a
	// RouteDeleteRequestMsg as a tea.Cmd — proving that App delegates
	// non-global keys to the active child Model.
	a := NewApp(appTestStyles())
	a1, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	app := a1.(App)
	a2, _ := app.Update(RoutesUpdateMsg{
		Routes: routeFixtureForApp(),
	})
	app = a2.(App)
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatal("expected RouteDeleteRequestMsg cmd from routes tab, got nil")
	}
	if _, ok := cmd().(RouteDeleteRequestMsg); !ok {
		t.Fatalf("expected RouteDeleteRequestMsg, got %T", cmd())
	}
}

// routeFixtureForApp returns a one-rule route map so the App's routes tab
// has something concrete to delete during integration tests.
func routeFixtureForApp() map[string]crdt.RouteRule {
	return map[string]crdt.RouteRule{
		"r-app-fixture": {
			Match:   crdt.RouteMatch{Project: "agenthive"},
			Targets: []string{"phone"},
			Action:  "notify",
		},
	}
}

func TestApp_Update_PeersTabReceivesPeerData(t *testing.T) {
	a := NewApp(appTestStyles())
	next, _ := a.Update(PeersUpdateMsg{
		Peers: peerFixture(),
	})
	app := next.(App)
	view := app.View()
	if !bytes.Contains([]byte(view), []byte("alice")) {
		t.Errorf("expected peer 'alice' in View:\n%s", view)
	}
	if !bytes.Contains([]byte(view), []byte("bob")) {
		t.Errorf("expected peer 'bob' in View:\n%s", view)
	}
}

func TestApp_FooterContainsHotkeyHints(t *testing.T) {
	a := NewApp(appTestStyles())
	view := a.View()
	for _, want := range []string{"p", "r", "a", "l", "q"} {
		if !bytes.Contains([]byte(view), []byte(want)) {
			t.Errorf("footer missing hint %q:\n%s", want, view)
		}
	}
}

func TestApp_InitialRender_Golden(t *testing.T) {
	a := NewApp(appTestStyles())
	a2, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	teatest.RequireEqualOutput(t, []byte(a2.(App).View()))
}

func TestApp_AfterRSwitchesToRoutes_Golden(t *testing.T) {
	a := NewApp(appTestStyles())
	a1, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a2, _ := a1.(App).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	teatest.RequireEqualOutput(t, []byte(a2.(App).View()))
}

func TestApp_TeatestDriver(t *testing.T) {
	a := NewApp(appTestStyles())
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(80, 24))
	// Switch to routes tab.
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	// Send a routes update.
	tm.Send(RoutesUpdateMsg{Routes: nil})

	if err := tm.Quit(); err != nil {
		t.Fatalf("quit: %v", err)
	}
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	out := appReadAll(t, tm.FinalOutput(t))
	// After switching to routes the SELECTOR header must appear.
	if !bytes.Contains(out, []byte("SELECTOR")) {
		t.Errorf("teatest output missing SELECTOR header after r:\n%s", out)
	}
}

func TestApp_InitialActiveIndicatorIsPeers(t *testing.T) {
	a := NewApp(appTestStyles())
	view := a.View()
	// The active tab indicator must show peers as active. We assert the
	// active-tab marker prefix on "peers" appears somewhere in the
	// rendered tab bar.
	if !bytes.Contains([]byte(view), []byte("[peers]")) {
		t.Errorf("expected active marker '[peers]' in initial View:\n%s", view)
	}
}

func TestApp_PressingRMakesRoutesActive(t *testing.T) {
	a := NewApp(appTestStyles())
	next, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	app := next.(App)
	view := app.View()
	if !bytes.Contains([]byte(view), []byte("[routes]")) {
		t.Errorf("expected active marker '[routes]' after r:\n%s", view)
	}
}

// peerFixture returns a deterministic two-peer map keyed by ID using
// crdt.PeerInfo so PeersUpdateMsg consumers can ingest it.
func peerFixture() map[string]crdt.PeerInfo {
	return map[string]crdt.PeerInfo{
		"peer-alice": {Name: "alice", Status: "online", Addr: "/ip4/1.2.3.4/tcp/4001", LinkType: "tcp"},
		"peer-bob":   {Name: "bob", Status: "online", Addr: "/ip4/5.6.7.8/tcp/4001", LinkType: "tcp"},
	}
}
