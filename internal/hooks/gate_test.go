package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// fakeDispatcher records DispatchAction invocations and can be configured to
// return synthetic errors or block until released so we can exercise the
// Gate's behaviour in isolation from the real dispatch package.
type fakeDispatcher struct {
	mu       sync.Mutex
	calls    []protocols.ActionRequest
	errs     []error
	blockCh  chan struct{}
	released atomic.Bool
}

func (f *fakeDispatcher) DispatchAction(ctx context.Context, a protocols.ActionRequest) []error {
	if f.blockCh != nil && !f.released.Load() {
		select {
		case <-f.blockCh:
		case <-ctx.Done():
			return []error{ctx.Err()}
		}
	}
	f.mu.Lock()
	f.calls = append(f.calls, a)
	f.mu.Unlock()
	return f.errs
}

func (f *fakeDispatcher) snapshot() []protocols.ActionRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]protocols.ActionRequest, len(f.calls))
	copy(out, f.calls)
	return out
}

func mkGateAction(id, tool, input string) protocols.ActionRequest {
	return protocols.ActionRequest{
		ActionID:  id,
		SessionID: "session-x",
		ToolUseID: "tu-x",
		ToolName:  tool,
		ToolInput: input,
		Project:   "agenthive",
		CWD:       "/tmp",
		Timestamp: time.Unix(1_700_000_000, 0).UTC(),
	}
}

// TestNewGate_Defaults verifies the constructor sets the documented default
// timeouts (5m normal, 30s destructive).
func TestNewGate_Defaults(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	g := NewGate(q, &fakeDispatcher{})
	if g == nil {
		t.Fatalf("NewGate returned nil")
	}
	if g.timeout != 5*time.Minute {
		t.Fatalf("expected default timeout 5m, got %v", g.timeout)
	}
	if g.destructiveTimeout != 30*time.Second {
		t.Fatalf("expected default destructive timeout 30s, got %v", g.destructiveTimeout)
	}
}

// TestHandle_HappyPath_RecordsDispatchAndReturnsDecision drives the gate
// end-to-end: a response is written to the queue while Handle is waiting and
// the decision is returned to the caller.
func TestHandle_HappyPath_RecordsDispatchAndReturnsDecision(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(dir)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	disp := &fakeDispatcher{}
	g := NewGate(q, disp)

	action := mkGateAction("happy-1", "Bash", "ls -la")

	// Write the response shortly after Handle starts so the gate has to wait
	// for it via the queue's polling loop.
	go func() {
		time.Sleep(25 * time.Millisecond)
		_ = q.WriteResponse(protocols.ActionResponse{
			ActionID:  "happy-1",
			Decision:  "allow",
			DecidedBy: "peer-z",
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := g.Handle(ctx, action)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.Decision != "allow" || resp.DecidedBy != "peer-z" || resp.ActionID != "happy-1" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	calls := disp.snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected dispatcher called once, got %d", len(calls))
	}
	got := calls[0]
	if got.ActionID != "happy-1" || got.ToolName != "Bash" {
		t.Fatalf("dispatcher received wrong action: %+v", got)
	}
	if got.ExpiresAt.IsZero() {
		t.Fatalf("dispatcher action must carry an ExpiresAt deadline")
	}

	// The queue must have cleaned up its tracking files.
	for _, name := range []string{"happy-1.pending", "happy-1.response"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("expected %s removed, stat err: %v", name, err)
		}
	}
}

// TestHandle_SetsNormalExpiresAt verifies a non-destructive action gets the
// 5m default expiry stamped onto its ExpiresAt field.
func TestHandle_SetsNormalExpiresAt(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	disp := &fakeDispatcher{}
	g := NewGate(q, disp)

	action := mkGateAction("normal-exp", "Read", "/etc/hosts")

	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = q.WriteResponse(protocols.ActionResponse{ActionID: "normal-exp", Decision: "allow"})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	before := time.Now()
	if _, err := g.Handle(ctx, action); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	after := time.Now()

	calls := disp.snapshot()
	if len(calls) != 1 {
		t.Fatalf("dispatcher call count: %d", len(calls))
	}
	exp := calls[0].ExpiresAt
	minDeadline := before.Add(5 * time.Minute).Add(-2 * time.Second)
	maxDeadline := after.Add(5 * time.Minute).Add(2 * time.Second)
	if exp.Before(minDeadline) || exp.After(maxDeadline) {
		t.Fatalf("normal ExpiresAt outside expected window: got %v, want in [%v,%v]", exp, minDeadline, maxDeadline)
	}
}

// TestHandle_DestructiveShortensExpiresAt verifies that an rm -rf action gets
// the 30s destructive deadline stamped onto its ExpiresAt field rather than
// the 5m default.
func TestHandle_DestructiveShortensExpiresAt(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	disp := &fakeDispatcher{}
	g := NewGate(q, disp)

	action := mkGateAction("destruct", "Bash", "rm -rf /tmp/danger")

	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = q.WriteResponse(protocols.ActionResponse{ActionID: "destruct", Decision: "deny"})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	before := time.Now()
	resp, err := g.Handle(ctx, action)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	after := time.Now()
	if resp.Decision != "deny" {
		t.Fatalf("expected deny, got %+v", resp)
	}

	calls := disp.snapshot()
	if len(calls) != 1 {
		t.Fatalf("dispatcher call count: %d", len(calls))
	}
	exp := calls[0].ExpiresAt
	minDeadline := before.Add(30 * time.Second).Add(-2 * time.Second)
	maxDeadline := after.Add(30 * time.Second).Add(2 * time.Second)
	if exp.Before(minDeadline) || exp.After(maxDeadline) {
		t.Fatalf("destructive ExpiresAt outside expected window: got %v, want in [%v,%v]", exp, minDeadline, maxDeadline)
	}
}

// TestHandle_PendingFileMirrorsExpiresAt verifies the on-disk pending file
// captures the stamped ExpiresAt so out-of-band peers see the deadline too.
func TestHandle_PendingFileMirrorsExpiresAt(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(dir)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	disp := &fakeDispatcher{blockCh: make(chan struct{})}
	g := NewGate(q, disp)

	action := mkGateAction("peek", "Bash", "echo hi")

	// Run Handle in the background. Dispatch will block until released so we
	// can inspect the pending file at our leisure.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = g.Handle(ctx, action)
	}()

	// Wait for the pending file to appear.
	pendingPath := filepath.Join(dir, "peek.pending")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(pendingPath); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	data, err := os.ReadFile(pendingPath)
	if err != nil {
		t.Fatalf("read pending: %v", err)
	}
	var got protocols.ActionRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal pending: %v", err)
	}
	if got.ExpiresAt.IsZero() {
		t.Fatalf("pending file does not carry an ExpiresAt deadline")
	}

	// Release dispatcher, drop a response, and let Handle finish so the
	// goroutine exits cleanly.
	disp.released.Store(true)
	close(disp.blockCh)
	if err := q.WriteResponse(protocols.ActionResponse{ActionID: "peek", Decision: "allow"}); err != nil {
		t.Fatalf("WriteResponse: %v", err)
	}
	<-done
}

// TestHandle_ContextDeadlineExceeded asserts that when no response ever
// arrives and the caller's context deadline fires, Handle returns
// context.DeadlineExceeded promptly.
func TestHandle_ContextDeadlineExceeded(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	g := NewGate(q, &fakeDispatcher{})

	action := mkGateAction("never", "Read", "/etc/hosts")

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = g.Handle(ctx, action)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if elapsed > 800*time.Millisecond {
		t.Fatalf("Handle did not return promptly after deadline: %v", elapsed)
	}
}

// TestHandle_DispatcherErrorsDoNotBlockReturn verifies that errors returned
// from DispatchAction are swallowed by Handle as long as a response is later
// written to the queue: the gate's contract is that a queue response always
// wins, regardless of surface failures.
func TestHandle_DispatcherErrorsDoNotBlockReturn(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	disp := &fakeDispatcher{errs: []error{errors.New("surface down"), errors.New("other")}}
	g := NewGate(q, disp)

	action := mkGateAction("err-still-ok", "Bash", "ls")

	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = q.WriteResponse(protocols.ActionResponse{ActionID: "err-still-ok", Decision: "allow"})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := g.Handle(ctx, action)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.Decision != "allow" {
		t.Fatalf("expected allow, got %+v", resp)
	}
}

// TestHandle_RejectsEmptyActionID enforces the caller contract: the gate must
// not write a pending file with an empty ID since the queue would refuse it
// anyway, but we want a clean error from Handle without side effects.
func TestHandle_RejectsEmptyActionID(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	disp := &fakeDispatcher{}
	g := NewGate(q, disp)

	action := mkGateAction("", "Bash", "ls")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = g.Handle(ctx, action)
	if err == nil {
		t.Fatalf("expected error for empty ActionID")
	}
	if calls := disp.snapshot(); len(calls) != 0 {
		t.Fatalf("dispatcher should not be called with empty ActionID, got %d calls", len(calls))
	}
}
