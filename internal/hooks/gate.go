package hooks

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// Default timeouts applied by NewGate. They match the contract in
// docs/superpowers/plans/2026-06-04-libp2p-impl.md task L2.C.
const (
	defaultGateTimeout            = 5 * time.Minute
	defaultGateDestructiveTimeout = 30 * time.Second
)

// actionDispatcher is the subset of dispatch.Dispatcher that the action gate
// needs. Declaring it locally lets gate_test.go drop in a fake without an
// import cycle with the dispatch package.
type actionDispatcher interface {
	DispatchAction(ctx context.Context, a protocols.ActionRequest) []error
}

// Gate is the local-device action gate. Handle blocks until a response is
// written to the queue or the (possibly shortened, for destructive actions)
// timeout elapses.
//
// The gate is intentionally stateless beyond its dependencies: every Handle
// call is independent, with no shared in-memory tracking between calls. State
// for in-flight actions lives entirely on the filesystem via the Queue, so a
// crash and restart of the daemon does not lose pending decisions.
type Gate struct {
	queue              *Queue
	dispatcher         actionDispatcher
	timeout            time.Duration
	destructiveTimeout time.Duration
	// nowFn is overridable in tests; production code uses time.Now.
	nowFn func() time.Time
}

// NewGate constructs a Gate with the default timeout (5m) and destructive
// timeout (30s). The dispatcher fan-out is treated as best-effort: errors from
// individual surfaces are logged but never block Handle from returning the
// queue's eventual decision.
func NewGate(queue *Queue, dispatcher actionDispatcher) *Gate {
	return &Gate{
		queue:              queue,
		dispatcher:         dispatcher,
		timeout:            defaultGateTimeout,
		destructiveTimeout: defaultGateDestructiveTimeout,
		nowFn:              time.Now,
	}
}

// Handle is called by the cmd/agenthive hook subcommand for each PreToolUse
// event. Returns the response decision or context.DeadlineExceeded.
//
// Workflow:
//  1. Set action.ExpiresAt = now + timeout (or destructiveTimeout when
//     security.Classify says destructive).
//  2. queue.WritePending(action).
//  3. dispatcher.DispatchAction(ctx, action) (errors logged, non-fatal).
//  4. queue.WaitResponse(ctx, action.ActionID).
//  5. Return response.
//
// The dispatcher is invoked synchronously but its errors are intentionally
// non-fatal: a remote peer being down must not deny a local user the chance
// to resolve the action via another surface (or the manual respond CLI).
func (g *Gate) Handle(ctx context.Context, action protocols.ActionRequest) (protocols.ActionResponse, error) {
	var zero protocols.ActionResponse

	if action.ActionID == "" {
		return zero, errors.New("hooks: Gate.Handle: ActionID must be set")
	}

	// Step 1: stamp the deadline that out-of-band surfaces will see. The
	// destructive timeout takes precedence so a remote operator has less time
	// to dither over an rm -rf.
	timeout := g.timeout
	if Classify(action.ToolName, action.ToolInput) == ActionDestructive {
		timeout = g.destructiveTimeout
	}
	action.ExpiresAt = g.nowFn().Add(timeout)

	// Step 2: persist the pending action atomically so concurrent surfaces
	// can read a complete request even if the daemon is killed mid-write.
	if err := g.queue.WritePending(action); err != nil {
		return zero, err
	}

	// Step 3: fan out to every surface in parallel. The dispatcher is told to
	// honour ctx, so a long-hanging surface still unblocks when the caller's
	// deadline fires. We log surface failures but never bubble them up: the
	// gate's contract is that the queue's response is authoritative.
	if errs := g.dispatcher.DispatchAction(ctx, action); len(errs) > 0 {
		for _, err := range errs {
			if err == nil {
				continue
			}
			log.Printf("hooks: dispatch action %s: %v", action.ActionID, err)
		}
	}

	// Step 4: block until a response file appears, ctx fires, or the
	// stamped ExpiresAt is reached (whichever happens first).
	waitCtx, cancel := contextWithDeadline(ctx, action.ExpiresAt)
	defer cancel()

	return g.queue.WaitResponse(waitCtx, action.ActionID)
}

// contextWithDeadline returns a child context bounded by both the parent
// context and the supplied deadline. If the parent already has a tighter
// deadline, the parent's deadline wins (context.WithDeadline guarantees this).
// The returned cancel func is always safe to call.
func contextWithDeadline(parent context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	if deadline.IsZero() {
		return context.WithCancel(parent)
	}
	return context.WithDeadline(parent, deadline)
}
