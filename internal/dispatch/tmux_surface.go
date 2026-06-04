// Package dispatch — tmux surface.
//
// TmuxSurface dispatches notifications by writing per-pane user options that
// the tmux status line renders without forking shells. The surface shells
// out to `tmux set-option -g @notif-...` for each metadata field; Close
// clears every option it set.
package dispatch

import (
	"context"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// TmuxSurface dispatches notifications by writing per-pane user options that
// the tmux status line renders without forking shells.
type TmuxSurface struct {
	exec CmdExecutor
}

// NewTmuxSurface constructs a TmuxSurface that shells out via the supplied
// CmdExecutor. Production callers pass NewOSExecutor; tests pass a recorder.
func NewTmuxSurface(exec CmdExecutor) *TmuxSurface {
	panic("not implemented: dispatch.NewTmuxSurface")
}

// Name returns the surface name, "tmux".
func (t *TmuxSurface) Name() string {
	panic("not implemented: dispatch.TmuxSurface.Name")
}

// Dispatch writes the notification's metadata as @notif-* tmux user options
// so the tmux status line can render them without forking a shell.
func (t *TmuxSurface) Dispatch(ctx context.Context, n protocols.Notification) error {
	panic("not implemented: dispatch.TmuxSurface.Dispatch")
}

// DispatchAction writes the action request as @notif-action-* tmux user
// options.
func (t *TmuxSurface) DispatchAction(ctx context.Context, a protocols.ActionRequest) error {
	panic("not implemented: dispatch.TmuxSurface.DispatchAction")
}

// Close clears every @notif-* option the surface set so the status line
// stops rendering stale data.
func (t *TmuxSurface) Close() error {
	panic("not implemented: dispatch.TmuxSurface.Close")
}
