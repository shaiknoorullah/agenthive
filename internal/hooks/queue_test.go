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

func mkAction(id string) protocols.ActionRequest {
	return protocols.ActionRequest{
		ActionID:  id,
		SessionID: "session-1",
		ToolUseID: "tool-use-1",
		ToolName:  "Bash",
		ToolInput: "ls -la",
		Project:   "agenthive",
		CWD:       "/tmp",
		Timestamp: time.Unix(1_700_000_000, 0).UTC(),
		ExpiresAt: time.Unix(1_700_000_300, 0).UTC(),
	}
}

func mkResponse(id string) protocols.ActionResponse {
	return protocols.ActionResponse{
		ActionID:  id,
		Decision:  "allow",
		DecidedBy: "peer-1",
	}
}

func TestNewQueue_CreatesDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "queue-subdir")

	q, err := NewQueue(dir)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	if q == nil {
		t.Fatalf("NewQueue returned nil queue")
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("queue dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory at %s", dir)
	}
	// Mode must restrict to owner only (0700). Mask out non-perm bits.
	if mode := info.Mode().Perm(); mode != 0o700 {
		t.Fatalf("expected dir mode 0700, got %o", mode)
	}
}

func TestNewQueue_AcceptsExistingDir(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewQueue(dir); err != nil {
		t.Fatalf("NewQueue on existing dir: %v", err)
	}
}

func TestWritePending_AtomicallyWritesFile(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	action := mkAction("abc123")
	if err := q.WritePending(action); err != nil {
		t.Fatalf("WritePending: %v", err)
	}

	path := filepath.Join(q.dir, "abc123.pending")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pending file: %v", err)
	}

	var got protocols.ActionRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal pending JSON: %v", err)
	}
	if got.ActionID != action.ActionID || got.ToolName != action.ToolName {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, action)
	}

	// Sanity check: no leftover temp files in the dir.
	entries, _ := os.ReadDir(q.dir)
	for _, e := range entries {
		if e.Name() == "abc123.pending" {
			continue
		}
		t.Fatalf("unexpected leftover file in queue dir: %s", e.Name())
	}
}

func TestWritePending_RequiresActionID(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	action := mkAction("")
	if err := q.WritePending(action); err == nil {
		t.Fatalf("expected error when ActionID is empty")
	}
}

func TestWriteResponse_OExclFirstWinsOthersFail(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	resp := mkResponse("collide")

	const N = 32
	var (
		wg       sync.WaitGroup
		successes int64
		failures  int64
	)
	start := make(chan struct{})

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := q.WriteResponse(resp); err == nil {
				atomic.AddInt64(&successes, 1)
			} else {
				atomic.AddInt64(&failures, 1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if successes != 1 {
		t.Fatalf("expected exactly 1 successful WriteResponse, got %d (failures=%d)", successes, failures)
	}
	if failures != N-1 {
		t.Fatalf("expected %d failed WriteResponse calls, got %d", N-1, failures)
	}
}

func TestWriteResponse_RequiresActionID(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	if err := q.WriteResponse(protocols.ActionResponse{Decision: "allow"}); err == nil {
		t.Fatalf("expected error for empty ActionID")
	}
}

func TestWaitResponse_DeliversResponseAndCleansFiles(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	action := mkAction("xyz789")
	if err := q.WritePending(action); err != nil {
		t.Fatalf("WritePending: %v", err)
	}

	want := mkResponse("xyz789")

	// Schedule a response after a short delay so WaitResponse must actually
	// poll for it.
	go func() {
		time.Sleep(50 * time.Millisecond)
		if err := q.WriteResponse(want); err != nil {
			t.Errorf("WriteResponse: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := q.WaitResponse(ctx, "xyz789")
	if err != nil {
		t.Fatalf("WaitResponse: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}

	// Both files must be cleaned up.
	if _, err := os.Stat(filepath.Join(q.dir, "xyz789.pending")); !os.IsNotExist(err) {
		t.Fatalf("pending file should be deleted, stat err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(q.dir, "xyz789.response")); !os.IsNotExist(err) {
		t.Fatalf("response file should be deleted, stat err: %v", err)
	}
}

func TestWaitResponse_HonorsContextDeadline(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	if err := q.WritePending(mkAction("never")); err != nil {
		t.Fatalf("WritePending: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = q.WaitResponse(ctx, "never")
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	// Must return promptly after the deadline. Allow generous slack for slow CI.
	if elapsed > 800*time.Millisecond {
		t.Fatalf("WaitResponse did not return promptly: %v", elapsed)
	}
}

func TestWaitResponse_HonorsCancellation(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	if err := q.WritePending(mkAction("cancel-me")); err != nil {
		t.Fatalf("WritePending: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err = q.WaitResponse(ctx, "cancel-me")
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.Canceled or DeadlineExceeded, got %v", err)
	}
}

// TestWaitResponse_SubHundredMsPolling guards the responsiveness requirement
// from the plan: the action gate must notice a written response within
// well under 100ms.
func TestWaitResponse_SubHundredMsPolling(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	action := mkAction("fast")
	if err := q.WritePending(action); err != nil {
		t.Fatalf("WritePending: %v", err)
	}

	resp := mkResponse("fast")

	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, err := q.WaitResponse(ctx, "fast"); err != nil {
			t.Errorf("WaitResponse: %v", err)
		}
		done <- time.Since(start)
	}()

	// Write the response almost immediately so we can measure the polling
	// pickup latency from the waiter's perspective.
	time.Sleep(5 * time.Millisecond)
	if err := q.WriteResponse(resp); err != nil {
		t.Fatalf("WriteResponse: %v", err)
	}

	select {
	case elapsed := <-done:
		// Plan requires sub-100ms polling. Be a bit lenient for CI variance:
		// require pickup within 150ms of the write being available.
		if elapsed > 150*time.Millisecond {
			t.Fatalf("WaitResponse pickup latency too high: %v", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("WaitResponse did not return")
	}
}

func TestWaitResponse_RequiresActionID(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := q.WaitResponse(ctx, ""); err == nil {
		t.Fatalf("expected error for empty actionID")
	}
}

// TestPendingThenResponseThenWait covers the case where the response file
// already exists when WaitResponse is called (e.g. peer was very fast).
func TestPendingThenResponseThenWait(t *testing.T) {
	q, err := NewQueue(t.TempDir())
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	action := mkAction("eager")
	if err := q.WritePending(action); err != nil {
		t.Fatalf("WritePending: %v", err)
	}
	want := mkResponse("eager")
	if err := q.WriteResponse(want); err != nil {
		t.Fatalf("WriteResponse: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := q.WaitResponse(ctx, "eager")
	if err != nil {
		t.Fatalf("WaitResponse: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}
