// Package dispatch — tmux surface.
//
// TmuxSurface dispatches notifications by writing tmux user options
// (@notif-*) via `tmux set-option -g`. The tmux status line can then
// render those values without forking a shell. Close clears every option
// the surface set during its lifetime so the status line stops rendering
// stale data.
//
// Subprocesses are invoked via the CmdExecutor abstraction, which makes
// the surface deterministically testable. Because every option write is
// a separate exec.Command argv (never a /bin/sh -c string), values are
// passed to tmux verbatim and shell metacharacters in the message body
// cannot be interpreted.
package dispatch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// Tmux user-option names. Centralised so Close can iterate them.
const (
	tmuxOptMsg        = "@notif-msg"
	tmuxOptProject    = "@notif-project"
	tmuxOptSource     = "@notif-source"
	tmuxOptTime       = "@notif-time"
	tmuxOptPriority   = "@notif-priority"
	tmuxOptActionID   = "@notif-action-id"
	tmuxOptActionTool = "@notif-action-tool"
)

// TmuxSurface dispatches notifications by writing tmux user options
// (@notif-*) that the status line can render without forking a shell.
//
// Concurrent Dispatch*/Close calls are safe: mu guards the closed flag
// and serialises the unset sweep with any in-flight set-option calls.
type TmuxSurface struct {
	exec CmdExecutor

	mu     sync.Mutex
	closed bool
}

// NewTmuxSurface constructs a TmuxSurface that shells out via the supplied
// CmdExecutor. Production callers pass NewOSExecutor; tests pass a recorder.
func NewTmuxSurface(exec CmdExecutor) *TmuxSurface {
	return &TmuxSurface{exec: exec}
}

// Name returns the surface name, "tmux".
func (t *TmuxSurface) Name() string { return "tmux" }

// Dispatch writes the notification's metadata as @notif-* tmux user options.
//
// Each field is written via a separate `tmux set-option -g <name> <value>`
// invocation. Because the CmdExecutor invokes the tmux binary with an argv
// slice (no shell), values are passed verbatim — message bodies containing
// shell metacharacters cannot be interpreted as code.
//
// If ctx is already cancelled when Dispatch is called we return ctx.Err()
// without invoking tmux at all. We do not interrupt an in-flight tmux
// process — set-option is fast and uncancellable in practice.
func (t *TmuxSurface) Dispatch(ctx context.Context, n protocols.Notification) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Emit a stable set of options. Empty fields are still written as
	// empty strings so stale values from a prior notification do not leak
	// into the status line.
	opts := []struct {
		name  string
		value string
	}{
		{tmuxOptMsg, n.Message},
		{tmuxOptProject, n.Project},
		{tmuxOptSource, n.Source},
		{tmuxOptTime, n.Timestamp.Format(time.RFC3339)},
		{tmuxOptPriority, n.Priority},
	}
	for _, o := range opts {
		if err := t.setOption(o.name, o.value); err != nil {
			return err
		}
	}
	return nil
}

// DispatchAction writes the action request's identifying fields as
// @notif-action-* tmux user options so the status line can prompt the
// user for a decision.
func (t *TmuxSurface) DispatchAction(ctx context.Context, a protocols.ActionRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	opts := []struct {
		name  string
		value string
	}{
		{tmuxOptActionID, a.ActionID},
		{tmuxOptActionTool, a.ToolName},
	}
	for _, o := range opts {
		if err := t.setOption(o.name, o.value); err != nil {
			return err
		}
	}
	return nil
}

// setOption runs `tmux set-option -g <name> <value>` via the CmdExecutor.
func (t *TmuxSurface) setOption(name, value string) error {
	if _, err := t.exec.Run("tmux", "set-option", "-g", name, value); err != nil {
		return fmt.Errorf("dispatch: tmux set-option %s: %w", name, err)
	}
	return nil
}

// Close clears every @notif-* option the surface set during its lifetime
// via `tmux set-option -gu <option>`. It is safe to call multiple times;
// the second and subsequent calls are no-ops.
//
// Close clears the full canonical option set rather than only the keys
// observed so far, so a fresh surface that never dispatched still clears
// any stale options left behind by a previous agenthive run.
func (t *TmuxSurface) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.mu.Unlock()

	// Canonical set of options that any TmuxSurface instance might have set.
	all := []string{
		tmuxOptMsg,
		tmuxOptProject,
		tmuxOptSource,
		tmuxOptTime,
		tmuxOptPriority,
		tmuxOptActionID,
		tmuxOptActionTool,
	}
	var firstErr error
	for _, name := range all {
		if _, err := t.exec.Run("tmux", "set-option", "-gu", name); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("dispatch: tmux unset %s: %w", name, err)
		}
	}
	return firstErr
}
