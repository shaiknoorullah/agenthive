// Package dispatch — desktop surface.
//
// DesktopSurface dispatches notifications via the host OS' native notifier:
// notify-send on Linux, osascript on macOS. On other operating systems it
// reports itself as "desktop:unsupported" and returns nil from Dispatch.
package dispatch

import (
	"context"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// DesktopSurface dispatches via the host OS' native notifier. It detects the
// OS via runtime.GOOS in the constructor and uses notify-send on Linux,
// osascript on macOS, and a no-op on every other OS.
type DesktopSurface struct {
	exec CmdExecutor
	os   string
}

// NewDesktopSurface constructs a DesktopSurface bound to the current OS.
// The exec parameter is the CmdExecutor used to invoke notify-send or
// osascript; production callers pass NewOSExecutor and tests pass a recorder.
func NewDesktopSurface(exec CmdExecutor) *DesktopSurface {
	panic("not implemented: dispatch.NewDesktopSurface")
}

// Name reports "desktop:linux", "desktop:darwin", or
// "desktop:unsupported" depending on the OS detected at construction.
func (d *DesktopSurface) Name() string {
	panic("not implemented: dispatch.DesktopSurface.Name")
}

// Dispatch invokes the per-OS notifier. On unsupported operating systems it
// returns nil and performs no IO.
func (d *DesktopSurface) Dispatch(ctx context.Context, n protocols.Notification) error {
	panic("not implemented: dispatch.DesktopSurface.Dispatch")
}

// DispatchAction invokes the per-OS notifier with an action prompt body.
func (d *DesktopSurface) DispatchAction(ctx context.Context, a protocols.ActionRequest) error {
	panic("not implemented: dispatch.DesktopSurface.DispatchAction")
}

// Close is a no-op for the desktop surface — there is no persistent state
// to release.
func (d *DesktopSurface) Close() error {
	panic("not implemented: dispatch.DesktopSurface.Close")
}
