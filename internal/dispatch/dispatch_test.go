package dispatch

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// mockSurface is a controllable Surface implementation for tests. It records
// every call (atomically) and can be instructed to return an error or block
// until a release channel is closed.
type mockSurface struct {
	name string

	dispatchCalls       int64
	dispatchActionCalls int64
	closeCalls          int64

	mu             sync.Mutex
	gotNotifs      []protocols.Notification
	gotActions     []protocols.ActionRequest
	dispatchErr    error
	actionErr      error
	closeErr       error
	block          chan struct{} // if non-nil, Dispatch/DispatchAction wait on this
	respectContext bool          // if true, Dispatch/DispatchAction also honor ctx.Done()
}

func newMockSurface(name string) *mockSurface {
	return &mockSurface{name: name}
}

func (m *mockSurface) Name() string { return m.name }

func (m *mockSurface) Dispatch(ctx context.Context, n protocols.Notification) error {
	atomic.AddInt64(&m.dispatchCalls, 1)
	m.mu.Lock()
	block := m.block
	respect := m.respectContext
	err := m.dispatchErr
	m.gotNotifs = append(m.gotNotifs, n)
	m.mu.Unlock()
	if block != nil {
		if respect {
			select {
			case <-block:
			case <-ctx.Done():
				return ctx.Err()
			}
		} else {
			<-block
		}
	}
	return err
}

func (m *mockSurface) DispatchAction(ctx context.Context, a protocols.ActionRequest) error {
	atomic.AddInt64(&m.dispatchActionCalls, 1)
	m.mu.Lock()
	block := m.block
	respect := m.respectContext
	err := m.actionErr
	m.gotActions = append(m.gotActions, a)
	m.mu.Unlock()
	if block != nil {
		if respect {
			select {
			case <-block:
			case <-ctx.Done():
				return ctx.Err()
			}
		} else {
			<-block
		}
	}
	return err
}

func (m *mockSurface) Close() error {
	atomic.AddInt64(&m.closeCalls, 1)
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closeErr
}

func (m *mockSurface) notifications() []protocols.Notification {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]protocols.Notification, len(m.gotNotifs))
	copy(out, m.gotNotifs)
	return out
}

func (m *mockSurface) actions() []protocols.ActionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]protocols.ActionRequest, len(m.gotActions))
	copy(out, m.gotActions)
	return out
}

// ---- Tests ----

func TestNew_EmptyConstructor(t *testing.T) {
	d := New(nil)
	if d == nil {
		t.Fatal("New(nil) returned nil")
	}
	errs := d.Dispatch(context.Background(), protocols.Notification{})
	if len(errs) != 0 {
		t.Fatalf("dispatch with no surfaces returned errors: %v", errs)
	}
}

func TestNew_WithSurfaces(t *testing.T) {
	a := newMockSurface("a")
	b := newMockSurface("b")
	d := New([]Surface{a, b})

	errs := d.Dispatch(context.Background(), protocols.Notification{Message: "hi"})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if got := atomic.LoadInt64(&a.dispatchCalls); got != 1 {
		t.Fatalf("a.dispatchCalls = %d, want 1", got)
	}
	if got := atomic.LoadInt64(&b.dispatchCalls); got != 1 {
		t.Fatalf("b.dispatchCalls = %d, want 1", got)
	}
	if got := a.notifications()[0].Message; got != "hi" {
		t.Fatalf("a received Message = %q, want %q", got, "hi")
	}
}

func TestAdd_RegistersAdditionalSurface(t *testing.T) {
	a := newMockSurface("a")
	d := New([]Surface{a})

	b := newMockSurface("b")
	d.Add(b)

	errs := d.Dispatch(context.Background(), protocols.Notification{})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if got := atomic.LoadInt64(&b.dispatchCalls); got != 1 {
		t.Fatalf("b.dispatchCalls = %d, want 1", got)
	}
}

func TestDispatch_CollectsErrors(t *testing.T) {
	a := newMockSurface("a")
	b := newMockSurface("b")
	c := newMockSurface("c")
	a.dispatchErr = errors.New("a failed")
	c.dispatchErr = errors.New("c failed")
	d := New([]Surface{a, b, c})

	errs := d.Dispatch(context.Background(), protocols.Notification{})
	if len(errs) != 2 {
		t.Fatalf("got %d errors, want 2: %v", len(errs), errs)
	}
	// Check that both expected errors are present (order is not guaranteed
	// because dispatch is parallel).
	msgs := map[string]bool{}
	for _, e := range errs {
		msgs[e.Error()] = true
	}
	if !msgs["a failed"] || !msgs["c failed"] {
		t.Fatalf("missing expected errors, got: %v", errs)
	}
}

func TestDispatch_FansOutInParallel(t *testing.T) {
	// Each surface blocks for a fixed delay. If the dispatcher were serial,
	// the total wall time would be N*delay. We verify it is closer to delay.
	const n = 5
	const delay = 100 * time.Millisecond

	surfaces := make([]Surface, 0, n)
	blocks := make([]chan struct{}, 0, n)
	for i := 0; i < n; i++ {
		s := newMockSurface("s")
		ch := make(chan struct{})
		s.block = ch
		surfaces = append(surfaces, s)
		blocks = append(blocks, ch)
	}

	d := New(surfaces)

	// Close all blocks after delay.
	go func() {
		time.Sleep(delay)
		for _, ch := range blocks {
			close(ch)
		}
	}()

	start := time.Now()
	errs := d.Dispatch(context.Background(), protocols.Notification{})
	elapsed := time.Since(start)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	// Parallel execution should be near delay, certainly less than n*delay.
	if elapsed >= n*delay {
		t.Fatalf("dispatch was serial: elapsed=%v, n*delay=%v", elapsed, n*delay)
	}
}

func TestDispatch_RespectsContextCancellation(t *testing.T) {
	s := newMockSurface("blocky")
	s.block = make(chan struct{})
	s.respectContext = true
	defer close(s.block)

	d := New([]Surface{s})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	errs := d.Dispatch(ctx, protocols.Notification{})
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Dispatch did not return after ctx cancel: elapsed=%v", elapsed)
	}
	if len(errs) == 0 {
		t.Fatal("expected ctx error from blocked surface, got none")
	}
}

func TestDispatchAction_CollectsErrors(t *testing.T) {
	a := newMockSurface("a")
	b := newMockSurface("b")
	a.actionErr = errors.New("boom")
	d := New([]Surface{a, b})

	errs := d.DispatchAction(context.Background(), protocols.ActionRequest{ActionID: "x"})
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	if errs[0].Error() != "boom" {
		t.Fatalf("error mismatch: %v", errs[0])
	}
	if got := b.actions()[0].ActionID; got != "x" {
		t.Fatalf("b received ActionID = %q, want %q", got, "x")
	}
}

func TestDispatchAction_FansOutInParallel(t *testing.T) {
	const n = 4
	const delay = 80 * time.Millisecond

	surfaces := make([]Surface, 0, n)
	blocks := make([]chan struct{}, 0, n)
	for i := 0; i < n; i++ {
		s := newMockSurface("s")
		ch := make(chan struct{})
		s.block = ch
		surfaces = append(surfaces, s)
		blocks = append(blocks, ch)
	}
	d := New(surfaces)
	go func() {
		time.Sleep(delay)
		for _, ch := range blocks {
			close(ch)
		}
	}()

	start := time.Now()
	errs := d.DispatchAction(context.Background(), protocols.ActionRequest{})
	elapsed := time.Since(start)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if elapsed >= n*delay {
		t.Fatalf("dispatch was serial: elapsed=%v, n*delay=%v", elapsed, n*delay)
	}
}

func TestClose_ClosesEverySurface(t *testing.T) {
	a := newMockSurface("a")
	b := newMockSurface("b")
	d := New([]Surface{a, b})

	if err := d.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if got := atomic.LoadInt64(&a.closeCalls); got != 1 {
		t.Fatalf("a.closeCalls = %d, want 1", got)
	}
	if got := atomic.LoadInt64(&b.closeCalls); got != 1 {
		t.Fatalf("b.closeCalls = %d, want 1", got)
	}
}

func TestClose_ReturnsFirstError(t *testing.T) {
	a := newMockSurface("a")
	b := newMockSurface("b")
	c := newMockSurface("c")
	a.closeErr = errors.New("a-close")
	c.closeErr = errors.New("c-close")
	d := New([]Surface{a, b, c})

	err := d.Close()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// All three should still have been closed even though some errored.
	if got := atomic.LoadInt64(&b.closeCalls); got != 1 {
		t.Fatalf("b.closeCalls = %d, want 1 (close must continue past errors)", got)
	}
	if got := atomic.LoadInt64(&c.closeCalls); got != 1 {
		t.Fatalf("c.closeCalls = %d, want 1 (close must continue past errors)", got)
	}
}

func TestAdd_ConcurrentWithDispatch(t *testing.T) {
	// This test exercises concurrent Add + Dispatch and must run cleanly
	// under -race. The exact number of surfaces a given Dispatch sees is
	// not deterministic, but no race or panic may occur.
	d := New(nil)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			d.Add(newMockSurface("dyn"))
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				d.Dispatch(context.Background(), protocols.Notification{})
				d.DispatchAction(context.Background(), protocols.ActionRequest{})
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}
