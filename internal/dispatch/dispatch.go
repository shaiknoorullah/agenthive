// Package dispatch fans out notifications and action requests to one or more
// Surfaces (log file, future tmux/desktop/Slack/etc adapters).
//
// Dispatcher.Dispatch* methods invoke every registered surface in parallel
// and collect their errors. They never block longer than the supplied ctx.
package dispatch

import (
	"context"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// Surface is anything that can present a notification or action request to a
// user (or downstream agent). Implementations must be safe for concurrent use.
type Surface interface {
	Name() string
	Dispatch(ctx context.Context, n protocols.Notification) error
	DispatchAction(ctx context.Context, a protocols.ActionRequest) error
	Close() error
}

// Dispatcher fans out to a set of Surfaces.
type Dispatcher struct {
	surfaces []Surface
}

// New constructs a Dispatcher pre-populated with the given Surfaces.
func New(surfaces []Surface) *Dispatcher {
	panic("not implemented: dispatch.New")
}

// Add registers an additional Surface. Safe for concurrent use with Dispatch*.
func (d *Dispatcher) Add(s Surface) {
	panic("not implemented: dispatch.Dispatcher.Add")
}

// Dispatch fans n out to every Surface in parallel and returns the collected
// non-nil errors. Returns when every surface has returned or ctx is done.
func (d *Dispatcher) Dispatch(ctx context.Context, n protocols.Notification) []error {
	panic("not implemented: dispatch.Dispatcher.Dispatch")
}

// DispatchAction fans a out to every Surface in parallel and returns the
// collected non-nil errors. Returns when every surface has returned or ctx
// is done.
func (d *Dispatcher) DispatchAction(ctx context.Context, a protocols.ActionRequest) []error {
	panic("not implemented: dispatch.Dispatcher.DispatchAction")
}

// Close closes every Surface and returns the first non-nil error, if any.
func (d *Dispatcher) Close() error {
	panic("not implemented: dispatch.Dispatcher.Close")
}
