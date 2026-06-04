// Package dispatch fans out notifications and action requests to one or more
// Surfaces (log file, future tmux/desktop/Slack/etc adapters).
//
// Dispatcher.Dispatch* methods invoke every registered surface in parallel
// and collect their errors. They never block longer than the supplied ctx.
package dispatch

import (
	"context"
	"os/exec"
	"sync"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// CmdExecutor is a mockable subprocess runner. Surfaces that shell out to
// external binaries (tmux, notify-send, osascript) take a CmdExecutor so
// tests can substitute an in-memory recorder.
type CmdExecutor interface {
	Run(name string, args ...string) ([]byte, error)
}

// OSExecutor is the concrete CmdExecutor backed by os/exec. It runs the
// named binary with the supplied args and returns the combined stdout +
// stderr output along with any process error.
type OSExecutor struct{}

// NewOSExecutor constructs the default CmdExecutor that runs real
// subprocesses via os/exec.Command.
func NewOSExecutor() *OSExecutor {
	return &OSExecutor{}
}

// Run executes name with args and returns the combined output and the
// process error.
func (e *OSExecutor) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// Surface is anything that can present a notification or action request to a
// user (or downstream agent). Implementations must be safe for concurrent use.
type Surface interface {
	Name() string
	Dispatch(ctx context.Context, n protocols.Notification) error
	DispatchAction(ctx context.Context, a protocols.ActionRequest) error
	Close() error
}

// Dispatcher fans out to a set of Surfaces.
//
// The Surface set is guarded by mu so Add may safely race with Dispatch /
// DispatchAction / Close. Dispatch operations take a snapshot of the current
// surface slice under the read lock and then run their fan-out without
// holding any lock.
type Dispatcher struct {
	mu       sync.RWMutex
	surfaces []Surface
}

// New constructs a Dispatcher pre-populated with the given Surfaces.
// A nil slice is treated as an empty set.
func New(surfaces []Surface) *Dispatcher {
	cp := make([]Surface, len(surfaces))
	copy(cp, surfaces)
	return &Dispatcher{surfaces: cp}
}

// Add registers an additional Surface. Safe for concurrent use with Dispatch*.
func (d *Dispatcher) Add(s Surface) {
	d.mu.Lock()
	d.surfaces = append(d.surfaces, s)
	d.mu.Unlock()
}

// snapshot returns a copy of the current surface slice. Callers can iterate
// it without holding the dispatcher lock.
func (d *Dispatcher) snapshot() []Surface {
	d.mu.RLock()
	out := make([]Surface, len(d.surfaces))
	copy(out, d.surfaces)
	d.mu.RUnlock()
	return out
}

// Dispatch fans n out to every Surface in parallel and returns the collected
// non-nil errors. Returns when every surface has returned or ctx is done.
//
// If ctx is cancelled before all surfaces complete, Dispatch returns the
// errors observed so far plus ctx.Err() for each pending surface. Surfaces
// that honor ctx should themselves return ctx.Err() and terminate; surfaces
// that do not honor ctx may continue running in the background after this
// call returns.
func (d *Dispatcher) Dispatch(ctx context.Context, n protocols.Notification) []error {
	surfaces := d.snapshot()
	if len(surfaces) == 0 {
		return nil
	}
	return fanOut(ctx, len(surfaces), func(i int) error {
		return surfaces[i].Dispatch(ctx, n)
	})
}

// DispatchAction fans a out to every Surface in parallel and returns the
// collected non-nil errors. Returns when every surface has returned or ctx
// is done.
//
// Same semantics as Dispatch with respect to ctx cancellation.
func (d *Dispatcher) DispatchAction(ctx context.Context, a protocols.ActionRequest) []error {
	surfaces := d.snapshot()
	if len(surfaces) == 0 {
		return nil
	}
	return fanOut(ctx, len(surfaces), func(i int) error {
		return surfaces[i].DispatchAction(ctx, a)
	})
}

// fanOut runs fn(i) for i in [0,n) in parallel and returns the collected
// non-nil errors. It returns as soon as every fn has returned or ctx is
// done, whichever comes first.
func fanOut(ctx context.Context, n int, fn func(i int) error) []error {
	errs := make([]error, n)
	done := make([]chan struct{}, n)
	for i := 0; i < n; i++ {
		ch := make(chan struct{})
		done[i] = ch
		i := i
		go func() {
			defer close(ch)
			errs[i] = fn(i)
		}()
	}

	// Wait for every goroutine OR for ctx to be done. If ctx fires first,
	// slots that haven't completed get ctx.Err() and we return immediately;
	// the underlying goroutines will still finish in the background and
	// write to their (now-unread) slots — that's fine because we no longer
	// touch errs after returning.
	allDone := make(chan struct{})
	go func() {
		for _, ch := range done {
			<-ch
		}
		close(allDone)
	}()

	select {
	case <-allDone:
		return collectNonNil(errs)
	case <-ctx.Done():
		// Collect a snapshot: for completed slots use the recorded error,
		// for pending slots use ctx.Err().
		out := make([]error, 0, n)
		for i := 0; i < n; i++ {
			select {
			case <-done[i]:
				if errs[i] != nil {
					out = append(out, errs[i])
				}
			default:
				out = append(out, ctx.Err())
			}
		}
		return out
	}
}

// Close closes every Surface and returns the first non-nil error, if any.
// Close always attempts to close every surface, even after an error.
func (d *Dispatcher) Close() error {
	surfaces := d.snapshot()
	var firstErr error
	for _, s := range surfaces {
		if err := s.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func collectNonNil(errs []error) []error {
	var out []error
	for _, e := range errs {
		if e != nil {
			out = append(out, e)
		}
	}
	return out
}
