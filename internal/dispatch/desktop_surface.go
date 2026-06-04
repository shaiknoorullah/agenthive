// Package dispatch — desktop surface.
//
// DesktopSurface dispatches notifications via the host OS' native notifier:
// notify-send on Linux, osascript on macOS. On other operating systems it
// reports itself as "desktop:unsupported" and returns nil from Dispatch.
package dispatch

import (
	"context"
	"fmt"
	"runtime"
	"strings"

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
	return &DesktopSurface{
		exec: exec,
		os:   runtime.GOOS,
	}
}

// Name reports "desktop:linux", "desktop:darwin", or
// "desktop:unsupported" depending on the OS detected at construction.
func (d *DesktopSurface) Name() string {
	switch d.os {
	case "linux":
		return "desktop:linux"
	case "darwin":
		return "desktop:darwin"
	default:
		return "desktop:unsupported"
	}
}

// Dispatch invokes the per-OS notifier. On unsupported operating systems it
// returns nil and performs no IO.
func (d *DesktopSurface) Dispatch(ctx context.Context, n protocols.Notification) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch d.os {
	case "linux":
		return d.dispatchLinux(notificationTitle(n), n.Message, mapUrgency(n.Priority))
	case "darwin":
		return d.dispatchDarwin(notificationTitle(n), n.Message, n.Source)
	default:
		return nil
	}
}

// DispatchAction invokes the per-OS notifier with an action prompt body.
func (d *DesktopSurface) DispatchAction(ctx context.Context, a protocols.ActionRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	title := "agenthive action: " + a.ToolName
	body := fmt.Sprintf("%s (id=%s)", a.ToolName, a.ActionID)
	switch d.os {
	case "linux":
		// Actions are always treated as critical urgency since they block work.
		return d.dispatchLinux(title, body, "critical")
	case "darwin":
		return d.dispatchDarwin(title, body, a.ActionID)
	default:
		return nil
	}
}

// Close is a no-op for the desktop surface — there is no persistent state
// to release.
func (d *DesktopSurface) Close() error {
	return nil
}

// dispatchLinux shells out to notify-send. notify-send treats every argument
// as a literal: there is no shell expansion since we never go through /bin/sh.
// The message body therefore needs no escaping.
func (d *DesktopSurface) dispatchLinux(title, body, urgency string) error {
	args := []string{
		"--urgency=" + urgency,
		"--app-name=agenthive",
		title,
		body,
	}
	if _, err := d.exec.Run("notify-send", args...); err != nil {
		return fmt.Errorf("dispatch: notify-send: %w", err)
	}
	return nil
}

// dispatchDarwin shells out to osascript -e. AppleScript string literals are
// delimited by " so we must escape embedded " and \ in the body.
func (d *DesktopSurface) dispatchDarwin(title, body, subtitle string) error {
	script := fmt.Sprintf(
		`display notification "%s" with title "%s" subtitle "%s"`,
		escapeAppleScript(body),
		escapeAppleScript(title),
		escapeAppleScript(subtitle),
	)
	if _, err := d.exec.Run("osascript", "-e", script); err != nil {
		return fmt.Errorf("dispatch: osascript: %w", err)
	}
	return nil
}

// escapeAppleScript escapes a string for safe inclusion inside an AppleScript
// double-quoted string literal. AppleScript only requires escaping of " and \.
func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// notificationTitle builds the title shown in the system notification. It
// favours the project name when present and falls back to the source.
func notificationTitle(n protocols.Notification) string {
	switch {
	case n.Project != "" && n.Source != "":
		return fmt.Sprintf("agenthive: %s / %s", n.Project, n.Source)
	case n.Project != "":
		return "agenthive: " + n.Project
	case n.Source != "":
		return "agenthive: " + n.Source
	default:
		return "agenthive"
	}
}

// mapUrgency maps an agenthive priority string to a notify-send urgency
// level. Unknown priorities collapse to "normal" rather than erroring so a
// future producer adding a new level doesn't break the dispatch path.
func mapUrgency(priority string) string {
	switch strings.ToLower(priority) {
	case "low":
		return "low"
	case "critical":
		return "critical"
	default:
		return "normal"
	}
}
