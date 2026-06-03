package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/hooks"
	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// testGateRig holds the artefacts a socket test needs to drive a real
// *hooks.Gate from the outside: the queue's on-disk directory (so the test can
// write response files to unblock the gate) and a fake dispatcher (so the test
// can observe which actions reached the surface fan-out).
type testGateRig struct {
	gate     *hooks.Gate
	queueDir string
	disp     *socketFakeDispatcher
}

// newTestGate builds a *hooks.Gate over a freshly created queue directory and
// a recording fake dispatcher. The returned rig exposes the queue dir so tests
// can drop response files directly and unblock the gate's wait.
func newTestGate(t *testing.T) testGateRig {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "queue")
	q, err := hooks.NewQueue(dir)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	disp := &socketFakeDispatcher{}
	return testGateRig{
		gate:     hooks.NewGate(q, disp),
		queueDir: dir,
		disp:     disp,
	}
}

// writeResponse is a convenience for tests that want to drop a response file
// directly into the queue dir (bypassing *hooks.Queue's helper). Both
// approaches produce the same on-disk artefact.
func (r testGateRig) writeResponse(t *testing.T, resp protocols.ActionResponse) {
	t.Helper()
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	path := filepath.Join(r.queueDir, resp.ActionID+".response")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

// pendingExists tells callers whether the gate has already written the
// pending file for actionID. Used by tests that need to write the matching
// response only after the gate is genuinely waiting.
func (r testGateRig) pendingExists(actionID string) bool {
	_, err := os.Stat(filepath.Join(r.queueDir, actionID+".pending"))
	return err == nil
}

// socketFakeDispatcher records DispatchAction invocations so tests can assert
// the gate actually fanned out a request to its surfaces. It implements the
// actionDispatcher interface that hooks.NewGate accepts.
type socketFakeDispatcher struct {
	mu    sync.Mutex
	calls []protocols.ActionRequest
}

func (f *socketFakeDispatcher) DispatchAction(_ context.Context, a protocols.ActionRequest) []error {
	f.mu.Lock()
	f.calls = append(f.calls, a)
	f.mu.Unlock()
	return nil
}

func (f *socketFakeDispatcher) snapshot() []protocols.ActionRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]protocols.ActionRequest, len(f.calls))
	copy(out, f.calls)
	return out
}

// socketPath returns a usable unix socket path inside the test's TempDir. We
// fall back to a shorter mkdtemp prefix on systems whose sun_path limit (104
// bytes on darwin, 108 on linux) would otherwise reject the resulting path.
func socketPath(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if len(p) > 100 {
		shortDir, err := os.MkdirTemp("", "ah-*")
		if err != nil {
			t.Fatalf("mkdtemp: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(shortDir) })
		p = filepath.Join(shortDir, name)
	}
	return p
}

// waitForSocket blocks until the unix-socket file exists at path or the
// deadline elapses. A regular file at the same path is ignored (the seed-stale
// tests put one there to verify the server unlinks it before binding). This
// avoids the false-positive a plain os.Stat would give us when the test
// pre-creates a stale regular file.
func waitForSocket(t *testing.T, path string, deadline time.Duration) {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if info, err := os.Stat(path); err == nil {
			if info.Mode()&os.ModeSocket != 0 {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("socket %s did not appear within %s", path, deadline)
}

// waitForPending blocks until the gate writes a pending file for actionID or
// the deadline elapses.
func waitForPending(t *testing.T, rig testGateRig, actionID string, deadline time.Duration) {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if rig.pendingExists(actionID) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("pending %s did not appear within %s", actionID, deadline)
}

// TestNewSocketServer_StoresPathAndGate verifies the constructor records the
// inputs verbatim. The fields are unexported so we exercise them indirectly
// via reflection-free same-package access.
func TestNewSocketServer_StoresPathAndGate(t *testing.T) {
	path := socketPath(t, "agenthive.sock")
	rig := newTestGate(t)
	s := NewSocketServer(path, rig.gate)
	if s == nil {
		t.Fatalf("NewSocketServer returned nil")
	}
	if s.path != path {
		t.Fatalf("path field: got %q want %q", s.path, path)
	}
	if s.gate != rig.gate {
		t.Fatalf("gate field: got %p want %p", s.gate, rig.gate)
	}
}

// TestRun_CreatesSocketWith0600Permissions confirms the on-disk socket file is
// created with mode 0600 (owner-only) as required by the plan.
func TestRun_CreatesSocketWith0600Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	path := socketPath(t, "agenthive.sock")
	rig := newTestGate(t)
	s := NewSocketServer(path, rig.gate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- s.Run(ctx) }()

	waitForSocket(t, path, 2*time.Second)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != 0o600 {
		t.Fatalf("socket perms: got %o want 0600", mode)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned unexpected err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return after cancel")
	}
}

// TestRun_RemovesStaleSocketBeforeBinding ensures restart-after-crash works:
// if a previous server crashed and left the socket file behind, Run unlinks it
// before binding rather than failing with EADDRINUSE.
func TestRun_RemovesStaleSocketBeforeBinding(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	path := socketPath(t, "agenthive.sock")
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatalf("seed stale: %v", err)
	}

	rig := newTestGate(t)
	s := NewSocketServer(path, rig.gate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- s.Run(ctx) }()

	waitForSocket(t, path, 2*time.Second)

	// Dial to confirm the listener is live (not the stale regular file).
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()

	cancel()
	<-errCh
}

// TestRun_ActionRequest_RoundTrip drives a complete request/response over the
// socket: dial, send a framed action_request, write the matching response into
// the queue from a goroutine, read the framed action_response back.
func TestRun_ActionRequest_RoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	path := socketPath(t, "agenthive.sock")
	rig := newTestGate(t)
	s := NewSocketServer(path, rig.gate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- s.Run(ctx) }()

	waitForSocket(t, path, 2*time.Second)

	// Drop a response in the background once the pending file appears.
	go func() {
		end := time.Now().Add(2 * time.Second)
		for time.Now().Before(end) {
			if rig.pendingExists("act-1") {
				rig.writeResponse(t, protocols.ActionResponse{
					ActionID:  "act-1",
					Decision:  "allow",
					DecidedBy: "test-peer",
				})
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	req := SocketEnvelope{
		Kind: KindActionRequest,
		Payload: mustMarshal(t, protocols.ActionRequest{
			ActionID:  "act-1",
			SessionID: "sess",
			ToolUseID: "tu",
			ToolName:  "Bash",
			ToolInput: "ls",
			Timestamp: time.Unix(1_700_000_000, 0).UTC(),
		}),
	}
	if err := protocols.WriteFramed(conn, req); err != nil {
		t.Fatalf("write request: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var env SocketEnvelope
	if err := protocols.ReadFramed(conn, &env); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if env.Kind != KindActionResponse {
		t.Fatalf("wrong response kind: got %q want %q (envelope=%+v)", env.Kind, KindActionResponse, env)
	}
	var resp protocols.ActionResponse
	if err := json.Unmarshal(env.Payload, &resp); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if resp.Decision != "allow" || resp.DecidedBy != "test-peer" || resp.ActionID != "act-1" {
		t.Fatalf("unexpected decision: %+v", resp)
	}
	if calls := rig.disp.snapshot(); len(calls) != 1 {
		t.Fatalf("dispatcher call count: %d", len(calls))
	}

	cancel()
	<-errCh
}

// TestRun_UnknownKind_RepliesWithError verifies the server responds with a
// kind:"error" envelope (not a raw close) when the client sends an unknown
// envelope kind.
func TestRun_UnknownKind_RepliesWithError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	path := socketPath(t, "agenthive.sock")
	rig := newTestGate(t)
	s := NewSocketServer(path, rig.gate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- s.Run(ctx) }()

	waitForSocket(t, path, 2*time.Second)

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	req := SocketEnvelope{
		Kind:    "totally-bogus",
		Payload: json.RawMessage(`{}`),
	}
	if err := protocols.WriteFramed(conn, req); err != nil {
		t.Fatalf("write request: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var env SocketEnvelope
	if err := protocols.ReadFramed(conn, &env); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if env.Kind != KindError {
		t.Fatalf("expected error kind, got %q", env.Kind)
	}
	var perr SocketError
	if err := json.Unmarshal(env.Payload, &perr); err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	if perr.Message == "" {
		t.Fatalf("expected non-empty error message")
	}

	cancel()
	<-errCh
}

// TestRun_GateUnblockOnServerShutdown verifies that when the server's context
// is cancelled while a request is in-flight (the gate is still waiting for a
// response), the in-flight handler unwinds and the client sees an error
// envelope or a closed connection — never a hung connection.
func TestRun_GateUnblockOnServerShutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	path := socketPath(t, "agenthive.sock")
	rig := newTestGate(t)
	s := NewSocketServer(path, rig.gate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- s.Run(ctx) }()

	waitForSocket(t, path, 2*time.Second)

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	req := SocketEnvelope{
		Kind: KindActionRequest,
		Payload: mustMarshal(t, protocols.ActionRequest{
			ActionID:  "never",
			SessionID: "sess",
			ToolUseID: "tu",
			ToolName:  "Read",
			ToolInput: "/etc/hosts",
		}),
	}
	if err := protocols.WriteFramed(conn, req); err != nil {
		t.Fatalf("write request: %v", err)
	}

	// Wait until the gate genuinely starts waiting so the cancel below
	// races against the wait rather than the bind.
	waitForPending(t, rig, "never", 2*time.Second)

	// Cancel the server. The gate's wait derives from the per-connection
	// context, which derives from the server's, so the goroutine handling
	// this connection must unblock and either send an error envelope or
	// close the connection.
	cancel()

	// Whichever shutdown path the server picks, the read must return
	// promptly — no hang.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var env SocketEnvelope
	readErr := protocols.ReadFramed(conn, &env)
	if readErr == nil {
		// If the server chose to reply with an error envelope, that's also
		// fine — just assert it is the error kind so we don't accidentally
		// swallow a valid action_response that came from nowhere.
		if env.Kind != KindError {
			t.Fatalf("on shutdown expected error kind or read error, got envelope %+v", env)
		}
	}

	<-errCh
}

// TestRun_RejectsActionRequestWithEmptyActionID verifies the server rejects
// malformed payloads at the envelope level: an action_request with no
// ActionID must produce a kind:"error" envelope (because hooks.Gate.Handle
// rejects it before ever waiting on the queue).
func TestRun_RejectsActionRequestWithEmptyActionID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	path := socketPath(t, "agenthive.sock")
	rig := newTestGate(t)
	s := NewSocketServer(path, rig.gate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- s.Run(ctx) }()

	waitForSocket(t, path, 2*time.Second)

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	req := SocketEnvelope{
		Kind: KindActionRequest,
		Payload: mustMarshal(t, protocols.ActionRequest{
			ToolName:  "Bash",
			ToolInput: "ls",
		}),
	}
	if err := protocols.WriteFramed(conn, req); err != nil {
		t.Fatalf("write request: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var env SocketEnvelope
	if err := protocols.ReadFramed(conn, &env); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if env.Kind != KindError {
		t.Fatalf("expected error kind, got %q", env.Kind)
	}

	cancel()
	<-errCh
}

// TestRun_ConcurrentConnections verifies the server handles multiple in-flight
// client connections in parallel. Each client gets its own goroutine and the
// responses are not cross-contaminated.
func TestRun_ConcurrentConnections(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	path := socketPath(t, "agenthive.sock")
	rig := newTestGate(t)
	s := NewSocketServer(path, rig.gate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- s.Run(ctx) }()

	waitForSocket(t, path, 2*time.Second)

	const n = 5
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = "concur-" + string(rune('a'+i))
	}

	// Drop responses for every expected action id once their pending files
	// appear. We scan the queue dir so we don't have to coordinate ordering.
	go func() {
		want := map[string]bool{}
		for _, id := range ids {
			want[id] = true
		}
		done := map[string]bool{}
		end := time.Now().Add(3 * time.Second)
		for time.Now().Before(end) && len(done) < n {
			entries, _ := os.ReadDir(rig.queueDir)
			for _, e := range entries {
				name := e.Name()
				if !strings.HasSuffix(name, ".pending") {
					continue
				}
				id := strings.TrimSuffix(name, ".pending")
				if !want[id] || done[id] {
					continue
				}
				rig.writeResponse(t, protocols.ActionResponse{
					ActionID:  id,
					Decision:  "allow",
					DecidedBy: "concurrent",
				})
				done[id] = true
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	var wg sync.WaitGroup
	results := make([]string, n)
	resultErr := make([]error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("unix", path)
			if err != nil {
				resultErr[i] = err
				return
			}
			defer conn.Close()
			req := SocketEnvelope{
				Kind: KindActionRequest,
				Payload: mustMarshal(t, protocols.ActionRequest{
					ActionID:  ids[i],
					SessionID: "s",
					ToolUseID: "t",
					ToolName:  "Bash",
					ToolInput: "ls",
				}),
			}
			if err := protocols.WriteFramed(conn, req); err != nil {
				resultErr[i] = err
				return
			}
			_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
			var env SocketEnvelope
			if err := protocols.ReadFramed(conn, &env); err != nil {
				resultErr[i] = err
				return
			}
			if env.Kind != KindActionResponse {
				resultErr[i] = errors.New("wrong kind: " + env.Kind)
				return
			}
			var resp protocols.ActionResponse
			if err := json.Unmarshal(env.Payload, &resp); err != nil {
				resultErr[i] = err
				return
			}
			results[i] = resp.ActionID
		}()
	}
	wg.Wait()

	for i, err := range resultErr {
		if err != nil {
			t.Fatalf("client %d: %v", i, err)
		}
		if results[i] != ids[i] {
			t.Fatalf("client %d: got id %q want %q", i, results[i], ids[i])
		}
	}

	cancel()
	<-errCh
}

// TestRun_ContextCancelReturnsCleanly verifies that cancelling the supplied
// context causes Run to return promptly and remove the socket file (so a
// follow-up Run on the same path can bind cleanly).
func TestRun_ContextCancelReturnsCleanly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	path := socketPath(t, "agenthive.sock")
	rig := newTestGate(t)
	s := NewSocketServer(path, rig.gate)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- s.Run(ctx) }()

	waitForSocket(t, path, 2*time.Second)

	start := time.Now()
	cancel()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return after cancel")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Run took too long to return: %v", elapsed)
	}

	// Socket file must be cleaned up so a restart can bind cleanly without the
	// stale-file dance every time.
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected socket file removed after cancel, stat err: %v", err)
	}
}

// TestRun_ReturnsErrorOnBindFailure verifies the server reports a non-nil
// error when it cannot bind the unix socket at all (e.g., the parent
// directory does not exist).
func TestRun_ReturnsErrorOnBindFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	path := filepath.Join(t.TempDir(), "does-not-exist-dir", "agenthive.sock")
	rig := newTestGate(t)
	s := NewSocketServer(path, rig.gate)

	err := s.Run(context.Background())
	if err == nil {
		t.Fatalf("expected bind error, got nil")
	}
}

// mustMarshal JSON-encodes v or fails the test.
func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
