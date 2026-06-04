package daemon

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// dialAndQuery is a convenience used by every query test: it dials the socket
// at path, writes a request envelope of kind k with optional payload, and reads
// back a single envelope.
func dialAndQuery(t *testing.T, path, kind string, payload any) SocketEnvelope {
	t.Helper()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal request payload: %v", err)
		}
		raw = b
	} else {
		raw = json.RawMessage(`{}`)
	}

	if err := protocols.WriteFramed(conn, SocketEnvelope{Kind: kind, Payload: raw}); err != nil {
		t.Fatalf("write request: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var env SocketEnvelope
	if err := protocols.ReadFramed(conn, &env); err != nil {
		t.Fatalf("read response: %v", err)
	}
	return env
}

// startDaemonForQueryTest boots a single daemon with a known SocketPath under
// a freshly-allocated config dir; returns the daemon, the socket path, the
// cancel func, and the error channel. Tests should cancel the ctx and drain
// errCh.
func startDaemonForQueryTest(t *testing.T) (*Daemon, string, context.CancelFunc, <-chan error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}

	cfg := newTestConfig(t)
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- d.Run(ctx) }()

	// Wait for the socket file and host to come up.
	if !waitForCondition(t, func() bool {
		if d.Host() == nil {
			return false
		}
		info, err := os.Stat(cfg.SocketPath)
		return err == nil && info.Mode()&os.ModeSocket != 0
	}, 5*time.Second) {
		cancel()
		<-errCh
		t.Fatalf("daemon never came up (socket at %s)", cfg.SocketPath)
	}

	return d, cfg.SocketPath, cancel, errCh
}

// TestSocket_ListPeers verifies the daemon exposes the CRDT peer set via the
// socket query API.
func TestSocket_ListPeers(t *testing.T) {
	d, sockPath, cancel, errCh := startDaemonForQueryTest(t)
	defer func() {
		cancel()
		<-errCh
	}()

	d.State().SetPeer("peer-a", crdt.PeerInfo{Name: "alpha", Status: "online"})
	d.State().SetPeer("peer-b", crdt.PeerInfo{Name: "bravo", Status: "offline"})

	env := dialAndQuery(t, sockPath, KindListPeers, nil)
	if env.Kind != KindListPeersResponse {
		t.Fatalf("kind: got %q want %q (payload=%s)", env.Kind, KindListPeersResponse, string(env.Payload))
	}

	var resp ListPeersResponse
	if err := json.Unmarshal(env.Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	got := map[string]crdt.PeerInfo{}
	for _, e := range resp.Peers {
		got[e.ID] = e.Info
	}
	if a, ok := got["peer-a"]; !ok || a.Name != "alpha" {
		t.Fatalf("expected peer-a alpha, got %+v", resp.Peers)
	}
	if b, ok := got["peer-b"]; !ok || b.Status != "offline" {
		t.Fatalf("expected peer-b offline, got %+v", resp.Peers)
	}
}

// TestSocket_ListRoutes verifies the daemon exposes the CRDT routes table via
// the socket query API.
func TestSocket_ListRoutes(t *testing.T) {
	d, sockPath, cancel, errCh := startDaemonForQueryTest(t)
	defer func() {
		cancel()
		<-errCh
	}()

	rule := crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "demo", Priority: "critical"},
		Targets: []string{"phone", "laptop"},
		Action:  "notify",
	}
	d.State().SetRoute("critical-demo", rule)

	env := dialAndQuery(t, sockPath, KindListRoutes, nil)
	if env.Kind != KindListRoutesResponse {
		t.Fatalf("kind: got %q want %q", env.Kind, KindListRoutesResponse)
	}

	var resp ListRoutesResponse
	if err := json.Unmarshal(env.Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Routes) != 1 {
		t.Fatalf("expected one route entry, got %d", len(resp.Routes))
	}
	got := resp.Routes[0]
	if got.ID != "critical-demo" || got.Rule.Match.Project != "demo" || got.Rule.Match.Priority != "critical" {
		t.Fatalf("unexpected route: %+v", got)
	}
	if len(got.Rule.Targets) != 2 || got.Rule.Targets[0] != "phone" || got.Rule.Targets[1] != "laptop" {
		t.Fatalf("unexpected targets: %+v", got.Rule.Targets)
	}
}

// TestSocket_ListActions verifies the daemon reports any pending action files
// from the queue directory.
func TestSocket_ListActions(t *testing.T) {
	d, sockPath, cancel, errCh := startDaemonForQueryTest(t)
	defer func() {
		cancel()
		<-errCh
	}()

	// Seed a pending action by writing directly into the queue dir, matching
	// the on-disk shape hooks.Queue produces.
	queueDir := filepath.Join(d.cfg.ConfigDir, "queue")
	if err := os.MkdirAll(queueDir, 0o700); err != nil {
		t.Fatalf("mkdir queue: %v", err)
	}
	req := protocols.ActionRequest{
		ActionID:  "act-99",
		SessionID: "sess",
		ToolUseID: "tu",
		ToolName:  "Bash",
		ToolInput: "rm -rf /",
		Timestamp: time.Unix(1_700_000_000, 0).UTC(),
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(queueDir, "act-99.pending"), body, 0o600); err != nil {
		t.Fatalf("write pending: %v", err)
	}

	env := dialAndQuery(t, sockPath, KindListActions, nil)
	if env.Kind != KindListActionsResponse {
		t.Fatalf("kind: got %q want %q (payload=%s)", env.Kind, KindListActionsResponse, string(env.Payload))
	}
	var resp ListActionsResponse
	if err := json.Unmarshal(env.Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	found := false
	for _, a := range resp.Actions {
		if a.Action.ActionID == "act-99" && a.Action.ToolName == "Bash" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("act-99 not reported by list_actions: %+v", resp.Actions)
	}
}

// TestSocket_ListLogs verifies the daemon tails its log file and surfaces the
// most recent N lines via the query API.
func TestSocket_ListLogs(t *testing.T) {
	d, sockPath, cancel, errCh := startDaemonForQueryTest(t)
	defer func() {
		cancel()
		<-errCh
	}()

	// Dispatch a notification through the daemon's local dispatcher so a log
	// line lands on disk.
	d.dispatcher.Dispatch(context.Background(), protocols.Notification{
		SessionID: "s1",
		Source:    "claude-code",
		Project:   "demo",
		Priority:  "normal",
		Message:   "hello",
		Timestamp: time.Unix(1_700_000_000, 0).UTC(),
	})

	// Allow the log surface a beat to flush.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		info, err := os.Stat(d.cfg.LogPath)
		if err == nil && info.Size() > 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	env := dialAndQuery(t, sockPath, KindListLogs, ListLogsRequest{Limit: 50})
	if env.Kind != KindListLogsResponse {
		t.Fatalf("kind: got %q want %q (payload=%s)", env.Kind, KindListLogsResponse, string(env.Payload))
	}
	var resp ListLogsResponse
	if err := json.Unmarshal(env.Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Lines) == 0 {
		t.Fatalf("expected at least one log line, got 0")
	}
	if !strings.Contains(resp.Lines[len(resp.Lines)-1], "hello") {
		t.Fatalf("expected last log line to contain hello: %q", resp.Lines[len(resp.Lines)-1])
	}
}

// TestSocket_ListLogs_RespectsLimit verifies the Limit field truncates the
// response to the most-recent N entries.
func TestSocket_ListLogs_RespectsLimit(t *testing.T) {
	d, sockPath, cancel, errCh := startDaemonForQueryTest(t)
	defer func() {
		cancel()
		<-errCh
	}()

	for i := 0; i < 10; i++ {
		d.dispatcher.Dispatch(context.Background(), protocols.Notification{
			SessionID: "s",
			Source:    "claude-code",
			Project:   "demo",
			Priority:  "low",
			Message:   "line",
			Timestamp: time.Now(),
		})
	}
	// Let the writes settle.
	time.Sleep(150 * time.Millisecond)

	env := dialAndQuery(t, sockPath, KindListLogs, ListLogsRequest{Limit: 3})
	if env.Kind != KindListLogsResponse {
		t.Fatalf("kind: got %q want %q", env.Kind, KindListLogsResponse)
	}
	var resp ListLogsResponse
	if err := json.Unmarshal(env.Payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Lines) > 3 {
		t.Fatalf("expected <=3 lines, got %d", len(resp.Lines))
	}
}
