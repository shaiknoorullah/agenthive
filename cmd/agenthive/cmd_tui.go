package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/daemon"
	"github.com/shaiknoorullah/agenthive/internal/protocols"
	"github.com/shaiknoorullah/agenthive/internal/tui"
)

// errDaemonDown is the canonical error returned when the local daemon's
// Unix socket cannot be reached or the initial snapshot fails. The
// wording is fixed: tests assert on the substring and operators see it
// directly on stderr.
var errDaemonDown = errors.New(
	"agenthive daemon is not running. Start it with: agenthive start",
)

// Default polling and timeout knobs. Both are overridable via tuiOptions
// so tests can drive the runTUI lifecycle without waiting two real
// seconds between poll rounds.
const (
	defaultTUIPollInterval = 2 * time.Second
	defaultTUIDialTimeout  = 1 * time.Second
	// queryRoundTimeout caps how long a single round of the four-query
	// snapshot takes. A daemon that accepts but never replies must not
	// pin the poll loop forever.
	queryRoundTimeout = 3 * time.Second
	// defaultLogQueryLimit is how many recent log lines we ask the
	// daemon for on each round. The daemon clamps this to its own cap,
	// so over-asking is safe and lets us scroll through history in the
	// logs tab without round-tripping again.
	defaultLogQueryLimit = 500
)

// tuiSnapshot bundles one round of the four queries so the poll loop can
// hand a complete picture to the bubbletea Program in one call. The TUI
// child Models all expect map-keyed or chronologically-ordered inputs
// (PeersUpdateMsg.Peers, RoutesUpdateMsg.Routes, ActionsUpdateMsg.Actions,
// LogsUpdateMsg.Entries) so the snapshot is materialised into the same
// shapes the App.Update branches consume.
type tuiSnapshot struct {
	Peers   map[string]crdt.PeerInfo
	Routes  map[string]crdt.RouteRule
	Actions []protocols.ActionRequest
	Logs    []tui.LogEntry
}

// tuiProgramRunner is the slice of the bubbletea Program API that runTUI
// needs. tea.Program already satisfies this implicitly via Run, Send,
// Quit, so the production newProgram closure can return a *tea.Program
// directly. Tests supply a recording stub that captures the messages
// without ever drawing a frame.
type tuiProgramRunner interface {
	Run() (tea.Model, error)
	Send(msg tea.Msg)
	Quit()
}

// tuiOptions wires the test-overridable knobs into runTUI. Production
// callers fill it via defaultTUIOptions; tests construct it inline.
type tuiOptions struct {
	socketPath   string
	pollInterval time.Duration
	dialTimeout  time.Duration
	// newProgram constructs the tuiProgramRunner around the bubbletea
	// Model. The production wiring uses tea.NewProgram(model); tests
	// substitute a recording stub.
	newProgram func(tea.Model) tuiProgramRunner
}

// defaultTUIOptions returns the production knob set for the supplied
// configDir. The bubbletea program is wired to read keyboard input and
// write frames to the user's terminal.
func defaultTUIOptions(cfgDir string, in io.Reader, out io.Writer) tuiOptions {
	return tuiOptions{
		socketPath:   filepath.Join(cfgDir, "agenthive.sock"),
		pollInterval: defaultTUIPollInterval,
		dialTimeout:  defaultTUIDialTimeout,
		newProgram: func(m tea.Model) tuiProgramRunner {
			return tea.NewProgram(m,
				tea.WithInput(in),
				tea.WithOutput(out),
				tea.WithAltScreen(),
			)
		},
	}
}

// newTUICmd returns the `agenthive tui` subcommand. The TUI connects to
// the local daemon's Unix socket (default <configDir>/agenthive.sock),
// snapshots peers/routes/actions/logs, launches the bubbletea App, and
// periodically polls for updates. A daemon-down dial produces a clear,
// actionable error message and exit 1.
func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the agenthive terminal UI",
		Long: "Connects to the local daemon socket, snapshots state, and " +
			"launches the bubbletea terminal UI. Exits 1 if the daemon is " +
			"not running.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			opts := defaultTUIOptions(configDir, cmd.InOrStdin(), cmd.OutOrStdout())
			return runTUI(ctx, cmd.OutOrStdout(), opts)
		},
	}
}

// runTUI is the testable core of the tui subcommand. It dials the daemon
// socket, fetches one snapshot, launches the bubbletea Program, and runs
// a poll loop that re-queries the daemon every opts.pollInterval and
// dispatches the resulting update messages.
//
// Lifecycle:
//   - A failed dial of the initial snapshot returns errDaemonDown.
//   - Once Run is called on the Program, runTUI blocks on it. Two
//     external signals can end the run: ctx.Done() (caller cancelled)
//     and the Program returning naturally (user pressed q / ctrl+c).
//   - On ctx.Done() runTUI calls Program.Quit so Run unblocks promptly.
//   - The poll-loop goroutine always exits before runTUI returns so no
//     goroutine leaks past the call.
func runTUI(ctx context.Context, _ io.Writer, opts tuiOptions) error {
	// Snapshot once up-front. The user sees the daemon-down error
	// before any TUI frame is rendered, which is what they want — a
	// blank screen followed by a crash is far worse UX than a one-line
	// "start the daemon" instruction.
	first, err := fetchSnapshot(ctx, opts.socketPath, opts.dialTimeout)
	if err != nil {
		return errDaemonDown
	}

	app := tui.NewApp(tui.NewStyles())
	prog := opts.newProgram(app)

	// Feed the initial snapshot into the program before Run wakes the
	// event loop so the user lands on a populated peers tab rather than
	// an empty placeholder.
	sendSnapshot(prog, first)

	pollCtx, cancelPoll := context.WithCancel(ctx)
	defer cancelPoll()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runPollLoop(pollCtx, prog, opts)
	}()

	// If the caller's ctx fires while we are inside Program.Run, ask
	// the program to quit so Run returns promptly and the poll loop
	// can shut down with us.
	stopWatch := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			prog.Quit()
		case <-stopWatch:
		}
	}()

	_, runErr := prog.Run()
	close(stopWatch)

	cancelPoll()
	wg.Wait()

	// Treat caller-cancelled and program-returned-cleanly as success;
	// only a genuine bubbletea error bubbles out.
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return runErr
	}
	return nil
}

// runPollLoop re-queries the daemon every opts.pollInterval and pushes
// the resulting update messages into the program. A failing round logs
// nothing and keeps the loop alive — a momentary disconnect must not
// kill the TUI mid-render. The loop exits on ctx.Done().
func runPollLoop(ctx context.Context, prog tuiProgramRunner, opts tuiOptions) {
	ticker := time.NewTicker(opts.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snap, err := fetchSnapshot(ctx, opts.socketPath, opts.dialTimeout)
			if err != nil {
				// Skip this round; the next tick will retry.
				continue
			}
			sendSnapshot(prog, snap)
		}
	}
}

// sendSnapshot dispatches the four typed update messages corresponding
// to a snapshot. The bubbletea App routes each message to the relevant
// child tab Model.
func sendSnapshot(prog tuiProgramRunner, snap tuiSnapshot) {
	prog.Send(tui.PeersUpdateMsg{Peers: snap.Peers})
	prog.Send(tui.RoutesUpdateMsg{Routes: snap.Routes})
	prog.Send(tui.ActionsUpdateMsg{Actions: snap.Actions})
	prog.Send(tui.LogsUpdateMsg{Entries: snap.Logs})
}

// fetchSnapshot dials the daemon socket once for each of the four query
// kinds and assembles a tuiSnapshot. Any per-query failure shortcircuits
// the round: callers either get the full snapshot or an error.
//
// Each query runs on a fresh connection because the daemon's socket
// protocol is one-request-one-response (see daemon.SocketServer
// handleConn). Keeping the connections tight is also the safest way to
// avoid pinning a goroutine on the daemon side if the TUI process is
// killed mid-poll.
func fetchSnapshot(ctx context.Context, socketPath string, dialTimeout time.Duration) (tuiSnapshot, error) {
	roundCtx, cancel := context.WithTimeout(ctx, queryRoundTimeout)
	defer cancel()

	peers, err := queryPeers(roundCtx, socketPath, dialTimeout)
	if err != nil {
		return tuiSnapshot{}, err
	}
	routes, err := queryRoutes(roundCtx, socketPath, dialTimeout)
	if err != nil {
		return tuiSnapshot{}, err
	}
	actions, err := queryActions(roundCtx, socketPath, dialTimeout)
	if err != nil {
		return tuiSnapshot{}, err
	}
	logs, err := queryLogs(roundCtx, socketPath, dialTimeout)
	if err != nil {
		return tuiSnapshot{}, err
	}
	return tuiSnapshot{
		Peers:   peers,
		Routes:  routes,
		Actions: actions,
		Logs:    logs,
	}, nil
}

// queryPeers sends a list_peers envelope and decodes the response into
// the map shape the TUI's PeersUpdateMsg expects.
func queryPeers(ctx context.Context, socketPath string, dialTimeout time.Duration) (map[string]crdt.PeerInfo, error) {
	var resp daemon.ListPeersResponse
	if err := roundTrip(ctx, socketPath, dialTimeout, daemon.KindListPeers, daemon.KindListPeersResponse, nil, &resp); err != nil {
		return nil, err
	}
	out := make(map[string]crdt.PeerInfo, len(resp.Peers))
	for _, entry := range resp.Peers {
		out[entry.ID] = entry.Info
	}
	return out, nil
}

// queryRoutes sends a list_routes envelope and decodes the response into
// the map shape the TUI's RoutesUpdateMsg expects.
func queryRoutes(ctx context.Context, socketPath string, dialTimeout time.Duration) (map[string]crdt.RouteRule, error) {
	var resp daemon.ListRoutesResponse
	if err := roundTrip(ctx, socketPath, dialTimeout, daemon.KindListRoutes, daemon.KindListRoutesResponse, nil, &resp); err != nil {
		return nil, err
	}
	out := make(map[string]crdt.RouteRule, len(resp.Routes))
	for _, entry := range resp.Routes {
		out[entry.ID] = entry.Rule
	}
	return out, nil
}

// queryActions sends a list_actions envelope and decodes the response
// into the slice shape ActionsUpdateMsg expects.
func queryActions(ctx context.Context, socketPath string, dialTimeout time.Duration) ([]protocols.ActionRequest, error) {
	var resp daemon.ListActionsResponse
	if err := roundTrip(ctx, socketPath, dialTimeout, daemon.KindListActions, daemon.KindListActionsResponse, nil, &resp); err != nil {
		return nil, err
	}
	out := make([]protocols.ActionRequest, 0, len(resp.Actions))
	for _, entry := range resp.Actions {
		out = append(out, entry.Action)
	}
	return out, nil
}

// queryLogs sends a list_logs envelope and decodes the resulting JSONL
// lines into the tui.LogEntry shape. Malformed lines are skipped so a
// single bad row cannot blank the entire logs tab.
func queryLogs(ctx context.Context, socketPath string, dialTimeout time.Duration) ([]tui.LogEntry, error) {
	req := daemon.ListLogsRequest{Limit: defaultLogQueryLimit}
	var resp daemon.ListLogsResponse
	if err := roundTrip(ctx, socketPath, dialTimeout, daemon.KindListLogs, daemon.KindListLogsResponse, req, &resp); err != nil {
		return nil, err
	}
	out := make([]tui.LogEntry, 0, len(resp.Lines))
	for _, line := range resp.Lines {
		entry, ok := parseLogLine(line)
		if !ok {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

// parseLogLine decodes one JSONL log line emitted by LogSurface into the
// tui.LogEntry shape the logs tab renders. Returns ok=false if the line
// is empty or fails to unmarshal; the caller skips it silently.
//
// The line schema is intentionally permissive: only the fields the TUI
// needs (ts, level, source, message) are read, and unknown keys are
// ignored. Unrecognised timestamp formats fall through with a zero time.
func parseLogLine(line string) (tui.LogEntry, bool) {
	if len(line) == 0 {
		return tui.LogEntry{}, false
	}
	var raw struct {
		Timestamp string `json:"ts"`
		Level     string `json:"level"`
		Source    string `json:"source"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return tui.LogEntry{}, false
	}
	entry := tui.LogEntry{
		Level:   raw.Level,
		Source:  raw.Source,
		Message: raw.Message,
	}
	if raw.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, raw.Timestamp); err == nil {
			entry.Timestamp = t
		} else if t, err := time.Parse(time.RFC3339, raw.Timestamp); err == nil {
			entry.Timestamp = t
		}
	}
	return entry, true
}

// roundTrip dials the socket, writes a framed envelope with reqKind and
// the JSON-encoded reqBody (nil → an empty object), reads one framed
// envelope back, validates the kind matches wantKind, and decodes the
// payload into out.
//
// Any deviation — failed dial, unexpected envelope kind, decode error —
// returns an error so the caller can decide whether to skip this poll
// round (background loop) or surface a daemon-down message (initial
// snapshot).
func roundTrip(ctx context.Context, socketPath string, dialTimeout time.Duration, reqKind, wantKind string, reqBody any, out any) error {
	dialer := net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return fmt.Errorf("dial daemon socket %s: %w", socketPath, err)
	}
	defer func() { _ = conn.Close() }()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(queryRoundTimeout))
	}

	var payload json.RawMessage
	if reqBody == nil {
		payload = json.RawMessage("{}")
	} else {
		body, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("encode %s payload: %w", reqKind, err)
		}
		payload = body
	}

	if err := protocols.WriteFramed(conn, daemon.SocketEnvelope{
		Kind:    reqKind,
		Payload: payload,
	}); err != nil {
		return fmt.Errorf("write %s envelope: %w", reqKind, err)
	}

	var env daemon.SocketEnvelope
	if err := protocols.ReadFramed(conn, &env); err != nil {
		return fmt.Errorf("read %s response: %w", reqKind, err)
	}
	if env.Kind != wantKind {
		return fmt.Errorf("expected envelope kind %q, got %q", wantKind, env.Kind)
	}
	if err := json.Unmarshal(env.Payload, out); err != nil {
		return fmt.Errorf("decode %s payload: %w", wantKind, err)
	}
	return nil
}
