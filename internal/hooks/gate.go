package hooks

import (
	"context"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
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
type Gate struct {
	queue              *Queue
	dispatcher         actionDispatcher
	timeout            time.Duration
	destructiveTimeout time.Duration
}

// NewGate constructs a Gate with the default timeout (5m) and destructive
// timeout (30s).
func NewGate(queue *Queue, dispatcher actionDispatcher) *Gate {
	panic("not implemented: hooks.NewGate")
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
func (g *Gate) Handle(ctx context.Context, action protocols.ActionRequest) (protocols.ActionResponse, error) {
	panic("not implemented: hooks.Gate.Handle")
}
