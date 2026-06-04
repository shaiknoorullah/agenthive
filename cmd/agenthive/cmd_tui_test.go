package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/daemon"
	"github.com/shaiknoorullah/agenthive/internal/protocols"
	"github.com/shaiknoorullah/agenthive/internal/tui"
)

// startStubDaemonSocket spins up a Unix-socket server at path that
// responds to the four TUI query envelope kinds (list_peers, list_routes,
// list_actions, list_logs) with canned responses derived from snap. Every
// other envelope kind is replied to with a kind:"error" envelope so the
// TUI's contract violations surface in tests as errors rather than hangs.
//
// The returned counter tracks how many envelopes the server has fielded
// so poll-loop tests can assert the periodic re-query actually happens.
func startStubDaemonSocket(t *testing.T, path string, snap tuiSnapshot) (*atomic.Int64, func()) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir socket parent: %v", err)
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}

	var calls atomic.Int64
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer func() { _ = c.Close() }()
				_ = c.SetDeadline(time.Now().Add(5 * time.Second))

				var env daemon.SocketEnvelope
				if err := protocols.ReadFramed(c, &env); err != nil {
					return
				}
				calls.Add(1)

				switch env.Kind {
				case daemon.KindListPeers:
					peers := make([]daemon.PeerEntry, 0, len(snap.Peers))
					for id, info := range snap.Peers {
						peers = append(peers, daemon.PeerEntry{ID: id, Info: info})
					}
					body, _ := json.Marshal(daemon.ListPeersResponse{Peers: peers})
					_ = protocols.WriteFramed(c, daemon.SocketEnvelope{
						Kind: daemon.KindListPeersResponse, Payload: body,
					})
				case daemon.KindListRoutes:
					routes := make([]daemon.RouteEntry, 0, len(snap.Routes))
					for id, rule := range snap.Routes {
						routes = append(routes, daemon.RouteEntry{ID: id, Rule: rule})
					}
					body, _ := json.Marshal(daemon.ListRoutesResponse{Routes: routes})
					_ = protocols.WriteFramed(c, daemon.SocketEnvelope{
						Kind: daemon.KindListRoutesResponse, Payload: body,
					})
				case daemon.KindListActions:
					actions := make([]daemon.ActionEntry, 0, len(snap.Actions))
					for _, a := range snap.Actions {
						actions = append(actions, daemon.ActionEntry{Action: a})
					}
					body, _ := json.Marshal(daemon.ListActionsResponse{Actions: actions})
					_ = protocols.WriteFramed(c, daemon.SocketEnvelope{
						Kind: daemon.KindListActionsResponse, Payload: body,
					})
				case daemon.KindListLogs:
					lines := make([]string, 0, len(snap.Logs))
					for _, e := range snap.Logs {
						line, _ := json.Marshal(logLineWire{
							Timestamp: e.Timestamp.Format(time.RFC3339Nano),
							Level:     e.Level,
							Source:    e.Source,
							Message:   e.Message,
						})
						lines = append(lines, string(line))
					}
					body, _ := json.Marshal(daemon.ListLogsResponse{Lines: lines})
					_ = protocols.WriteFramed(c, daemon.SocketEnvelope{
						Kind: daemon.KindListLogsResponse, Payload: body,
					})
				default:
					body, _ := json.Marshal(daemon.SocketError{Message: "unknown kind"})
					_ = protocols.WriteFramed(c, daemon.SocketEnvelope{
						Kind: daemon.KindError, Payload: body,
					})
				}
			}(conn)
		}
	}()

	cleanup := func() {
		_ = ln.Close()
		wg.Wait()
		_ = os.Remove(path)
	}
	return &calls, cleanup
}

// logLineWire mirrors the JSONL schema used by dispatch.LogSurface and
// expected by cmd_tui's parseLogLine. Keeping it package-local rather
// than reaching into internal/dispatch keeps this test file self-
// sufficient.
type logLineWire struct {
	Timestamp string `json:"ts"`
	Level     string `json:"level"`
	Source    string `json:"source"`
	Message   string `json:"message"`
}

// shortTempDir mints a short tmpdir under /tmp so the resulting Unix
// socket path stays under the 104-byte kernel limit on macOS. t.TempDir()
// often returns paths near or over that limit.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ah-tui-*")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// recordingProgram is a tuiProgramRunner stub that captures the initial
// model state and every tea.Msg the runTUI loop sends after Init. The
// stub returns from Run as soon as the test calls finishImmediately, so
// tests can drive the runTUI lifecycle without ever spinning a real
// bubbletea Program.
type recordingProgram struct {
	mu        sync.Mutex
	model     tea.Model
	msgs      []tea.Msg
	runCalled bool
	// finishOnReady lets the test trigger Run's return without waiting
	// for the poll loop to fire — handy for the snapshot-only tests.
	finishOnReady chan struct{}
}

func newRecordingProgram() *recordingProgram {
	return &recordingProgram{
		finishOnReady: make(chan struct{}),
	}
}

func (p *recordingProgram) Run() (tea.Model, error) {
	p.mu.Lock()
	p.runCalled = true
	p.mu.Unlock()
	<-p.finishOnReady
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.model, nil
}

func (p *recordingProgram) Send(msg tea.Msg) {
	p.mu.Lock()
	p.msgs = append(p.msgs, msg)
	p.mu.Unlock()
}

func (p *recordingProgram) Quit() {
	p.finishImmediately()
}

func (p *recordingProgram) snapshotMsgs() []tea.Msg {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]tea.Msg, len(p.msgs))
	copy(out, p.msgs)
	return out
}

func (p *recordingProgram) runCount() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.runCalled
}

// finishImmediately tells the recordingProgram's Run() to return now.
// Safe to call multiple times concurrently.
func (p *recordingProgram) finishImmediately() {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case <-p.finishOnReady:
	default:
		close(p.finishOnReady)
	}
}

// TestTUI_ErrorsWhenDaemonAbsent asserts the user-facing error message
// when the daemon socket cannot be dialled.
func TestTUI_ErrorsWhenDaemonAbsent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	dir := shortTempDir(t)

	_, _, err := runRoot(t, []string{"--config-dir", dir, "tui"}, "")
	if err == nil {
		t.Fatalf("tui: expected error when daemon unreachable, got nil")
	}
	if msg := err.Error(); !contains(msg, "agenthive daemon is not running") ||
		!contains(msg, "agenthive start") {
		t.Fatalf("tui error message should guide the user; got: %q", msg)
	}
}

// TestTUI_SnapshotsInitialStateOverSocket verifies that runTUI dials the
// stub daemon, fans out the four query envelopes, and feeds a
// PeersUpdateMsg / RoutesUpdateMsg / ActionsUpdateMsg / LogsUpdateMsg
// into the bubbletea Program before Run returns.
func TestTUI_SnapshotsInitialStateOverSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	dir := shortTempDir(t)
	socketPath := filepath.Join(dir, "agenthive.sock")

	now := time.Now().UTC().Truncate(time.Second)
	snap := tuiSnapshot{
		Peers: map[string]crdt.PeerInfo{
			"peer-1": {Name: "alpha", Status: "online"},
		},
		Routes: map[string]crdt.RouteRule{
			"r-1": {
				Match:   crdt.RouteMatch{Priority: "critical"},
				Targets: []string{"phone"},
			},
		},
		Actions: []protocols.ActionRequest{{ActionID: "a-1", ToolName: "Bash"}},
		Logs:    []tui.LogEntry{{Timestamp: now, Level: "info", Source: "test", Message: "hello"}},
	}
	_, cleanup := startStubDaemonSocket(t, socketPath, snap)
	defer cleanup()

	rec := newRecordingProgram()
	opts := tuiOptions{
		socketPath:   socketPath,
		pollInterval: 50 * time.Millisecond,
		newProgram: func(m tea.Model) tuiProgramRunner {
			rec.model = m
			return rec
		},
		dialTimeout: time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- runTUI(ctx, io.Discard, opts) }()

	// Wait for the four initial messages to land.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if hasAllSnapshotMsgs(rec.snapshotMsgs()) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	msgs := rec.snapshotMsgs()
	if !hasAllSnapshotMsgs(msgs) {
		t.Fatalf("expected all four snapshot messages, got: %s", msgKinds(msgs))
	}
	if !containsPeerMsg(msgs, "peer-1") {
		t.Fatalf("PeersUpdateMsg missing expected peer; got: %#v", msgs)
	}
	if !containsRouteMsg(msgs, "r-1") {
		t.Fatalf("RoutesUpdateMsg missing expected route; got: %#v", msgs)
	}
	if !containsActionMsg(msgs, "a-1") {
		t.Fatalf("ActionsUpdateMsg missing expected action; got: %#v", msgs)
	}
	if !containsLogMsg(msgs, "hello") {
		t.Fatalf("LogsUpdateMsg missing expected log; got: %#v", msgs)
	}

	rec.finishImmediately()
	if err := <-done; err != nil {
		t.Fatalf("runTUI returned error: %v", err)
	}
	if !rec.runCount() {
		t.Fatalf("expected Program.Run() to be called")
	}
}

// TestTUI_PollsPeriodically verifies the 2s poll loop actually re-queries
// the daemon. With pollInterval shortened to 25ms the stub should see at
// least a few rounds of queries before we cancel.
func TestTUI_PollsPeriodically(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	dir := shortTempDir(t)
	socketPath := filepath.Join(dir, "agenthive.sock")

	calls, cleanup := startStubDaemonSocket(t, socketPath, tuiSnapshot{
		Peers:   map[string]crdt.PeerInfo{},
		Routes:  map[string]crdt.RouteRule{},
		Actions: []protocols.ActionRequest{},
		Logs:    []tui.LogEntry{},
	})
	defer cleanup()

	rec := newRecordingProgram()
	opts := tuiOptions{
		socketPath:   socketPath,
		pollInterval: 25 * time.Millisecond,
		newProgram: func(m tea.Model) tuiProgramRunner {
			rec.model = m
			return rec
		},
		dialTimeout: time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- runTUI(ctx, io.Discard, opts) }()

	// Each round of polling sends four envelopes (peers/routes/actions/
	// logs). After ~5 rounds we expect ≥ 12 envelopes received by the
	// stub (4 initial + at least 2 poll rounds × 4).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if calls.Load() >= 12 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	got := calls.Load()
	if got < 12 {
		t.Fatalf("expected at least 12 envelopes after polling; got %d", got)
	}

	rec.finishImmediately()
	if err := <-done; err != nil {
		t.Fatalf("runTUI returned error: %v", err)
	}
}

// TestTUI_DialDeadlineRespected ensures the dial timeout actually fires
// when the socket does not exist. A 50ms timeout against an absent
// socket must surface the standard daemon-down error.
func TestTUI_DialDeadlineRespected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	dir := shortTempDir(t)
	socketPath := filepath.Join(dir, "agenthive.sock")
	// Don't start a server.

	rec := newRecordingProgram()
	opts := tuiOptions{
		socketPath:   socketPath,
		pollInterval: 100 * time.Millisecond,
		newProgram: func(m tea.Model) tuiProgramRunner {
			rec.model = m
			return rec
		},
		dialTimeout: 50 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := runTUI(ctx, io.Discard, opts)
	if err == nil {
		t.Fatalf("expected error when daemon socket cannot be dialled")
	}
	if !contains(err.Error(), "agenthive daemon is not running") {
		t.Fatalf("expected daemon-down error; got: %v", err)
	}
}

// TestTUI_ContextCancelEndsRun verifies the cancel-context path: when
// the caller's ctx fires, runTUI stops the poll loop and asks the
// program to quit. Run returns nil.
func TestTUI_ContextCancelEndsRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	dir := shortTempDir(t)
	socketPath := filepath.Join(dir, "agenthive.sock")

	_, cleanup := startStubDaemonSocket(t, socketPath, tuiSnapshot{
		Peers:   map[string]crdt.PeerInfo{},
		Routes:  map[string]crdt.RouteRule{},
		Actions: []protocols.ActionRequest{},
		Logs:    []tui.LogEntry{},
	})
	defer cleanup()

	rec := newRecordingProgram()
	opts := tuiOptions{
		socketPath:   socketPath,
		pollInterval: 50 * time.Millisecond,
		newProgram: func(m tea.Model) tuiProgramRunner {
			rec.model = m
			return rec
		},
		dialTimeout: time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- runTUI(ctx, io.Discard, opts) }()

	// Let the initial snapshot land.
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("runTUI on cancel: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("runTUI did not return after ctx cancel")
	}
}

// hasAllSnapshotMsgs reports whether the captured messages include one
// of each of the four expected initial update messages.
func hasAllSnapshotMsgs(msgs []tea.Msg) bool {
	var p, r, a, l bool
	for _, m := range msgs {
		switch m.(type) {
		case tui.PeersUpdateMsg:
			p = true
		case tui.RoutesUpdateMsg:
			r = true
		case tui.ActionsUpdateMsg:
			a = true
		case tui.LogsUpdateMsg:
			l = true
		}
	}
	return p && r && a && l
}

// msgKinds prints a tabular summary of what message types showed up in
// msgs, used to make Fatalf output legible.
func msgKinds(msgs []tea.Msg) string {
	out := "["
	for i, m := range msgs {
		if i > 0 {
			out += " "
		}
		out += fmt.Sprintf("%T", m)
	}
	out += "]"
	return out
}

// containsPeerMsg returns true if any PeersUpdateMsg in msgs holds id.
func containsPeerMsg(msgs []tea.Msg, id string) bool {
	for _, m := range msgs {
		pm, ok := m.(tui.PeersUpdateMsg)
		if !ok {
			continue
		}
		if _, present := pm.Peers[id]; present {
			return true
		}
	}
	return false
}

// containsRouteMsg returns true if any RoutesUpdateMsg in msgs holds id.
func containsRouteMsg(msgs []tea.Msg, id string) bool {
	for _, m := range msgs {
		rm, ok := m.(tui.RoutesUpdateMsg)
		if !ok {
			continue
		}
		if _, present := rm.Routes[id]; present {
			return true
		}
	}
	return false
}

// containsActionMsg returns true if any ActionsUpdateMsg in msgs has an
// action with the given ID.
func containsActionMsg(msgs []tea.Msg, id string) bool {
	for _, m := range msgs {
		am, ok := m.(tui.ActionsUpdateMsg)
		if !ok {
			continue
		}
		for _, a := range am.Actions {
			if a.ActionID == id {
				return true
			}
		}
	}
	return false
}

// containsLogMsg returns true if any LogsUpdateMsg in msgs has a log
// entry whose Message equals msg.
func containsLogMsg(msgs []tea.Msg, msg string) bool {
	for _, m := range msgs {
		lm, ok := m.(tui.LogsUpdateMsg)
		if !ok {
			continue
		}
		for _, e := range lm.Entries {
			if e.Message == msg {
				return true
			}
		}
	}
	return false
}

// contains is a thin substring check that keeps the test file free of
// an import on strings just for this one helper, mirroring the helper
// style used elsewhere in this package's tests.
func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
