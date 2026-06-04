package dispatch

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// recordingExec is a CmdExecutor that captures every invocation of Run for
// later assertion. It is safe for concurrent use by multiple goroutines so
// surface tests can run under -race.
//
// It is shared between desktop_surface_test.go and tmux_surface_test.go.
type recordingExec struct {
	mu    sync.Mutex
	calls []recordedCall
	err   error
	out   []byte
}

// recordedCall captures one Run invocation: the binary name and the argv
// passed to it.
type recordedCall struct {
	name string
	args []string
}

// Run records the call and returns the configured stub output/error.
func (r *recordingExec) Run(name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Defensive copy of args so callers mutating their slice later don't
	// poison the recording.
	cp := make([]string, len(args))
	copy(cp, args)
	r.calls = append(r.calls, recordedCall{name: name, args: cp})
	return r.out, r.err
}

// snapshot returns a copy of the calls recorded so far.
func (r *recordingExec) snapshot() []recordedCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedCall, len(r.calls))
	copy(out, r.calls)
	return out
}

func newDesktopSurfaceForOS(t *testing.T, exec CmdExecutor, osName string) *DesktopSurface {
	t.Helper()
	d := NewDesktopSurface(exec)
	// Override the detected OS so tests are deterministic across CI hosts.
	d.os = osName
	return d
}

func TestNewDesktopSurface_NotNil(t *testing.T) {
	r := &recordingExec{}
	d := NewDesktopSurface(r)
	if d == nil {
		t.Fatal("NewDesktopSurface returned nil")
	}
}

func TestDesktopSurface_Name_Linux(t *testing.T) {
	r := &recordingExec{}
	d := newDesktopSurfaceForOS(t, r, "linux")
	if got, want := d.Name(), "desktop:linux"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}
}

func TestDesktopSurface_Name_Darwin(t *testing.T) {
	r := &recordingExec{}
	d := newDesktopSurfaceForOS(t, r, "darwin")
	if got, want := d.Name(), "desktop:darwin"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}
}

func TestDesktopSurface_Name_Unsupported(t *testing.T) {
	r := &recordingExec{}
	d := newDesktopSurfaceForOS(t, r, "windows")
	if got, want := d.Name(), "desktop:unsupported"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}
}

func TestDesktopSurface_Dispatch_LinuxCallsNotifySend(t *testing.T) {
	r := &recordingExec{}
	d := newDesktopSurfaceForOS(t, r, "linux")

	n := protocols.Notification{
		Source:    "claude-code",
		Project:   "agenthive",
		Priority:  "normal",
		Message:   "build complete",
		Timestamp: time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC),
	}
	if err := d.Dispatch(context.Background(), n); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}

	calls := r.snapshot()
	if len(calls) != 1 {
		t.Fatalf("got %d exec calls, want 1: %+v", len(calls), calls)
	}
	c := calls[0]
	if c.name != "notify-send" {
		t.Fatalf("exec name = %q, want %q", c.name, "notify-send")
	}

	joined := strings.Join(c.args, " ")
	if !strings.Contains(joined, "--urgency=normal") {
		t.Fatalf("missing urgency flag in args: %v", c.args)
	}
	if !strings.Contains(joined, "--app-name=agenthive") {
		t.Fatalf("missing app-name flag in args: %v", c.args)
	}
	// Title and body should both appear somewhere in the trailing positional args.
	if !strings.Contains(joined, "build complete") {
		t.Fatalf("body missing from args: %v", c.args)
	}
}

func TestDesktopSurface_Dispatch_LinuxPriorityMapping(t *testing.T) {
	cases := []struct {
		priority string
		urgency  string
	}{
		{"low", "low"},
		{"normal", "normal"},
		{"high", "normal"},
		{"critical", "critical"},
		{"", "normal"},
		{"WeIrD", "normal"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.priority, func(t *testing.T) {
			r := &recordingExec{}
			d := newDesktopSurfaceForOS(t, r, "linux")
			if err := d.Dispatch(context.Background(), protocols.Notification{
				Priority: tc.priority,
				Message:  "hi",
			}); err != nil {
				t.Fatalf("Dispatch returned error: %v", err)
			}
			calls := r.snapshot()
			if len(calls) != 1 {
				t.Fatalf("got %d calls, want 1", len(calls))
			}
			joined := strings.Join(calls[0].args, " ")
			want := "--urgency=" + tc.urgency
			if !strings.Contains(joined, want) {
				t.Fatalf("priority %q → urgency want %q, args: %v", tc.priority, tc.urgency, calls[0].args)
			}
		})
	}
}

func TestDesktopSurface_Dispatch_DarwinCallsOsascript(t *testing.T) {
	r := &recordingExec{}
	d := newDesktopSurfaceForOS(t, r, "darwin")

	n := protocols.Notification{
		Source:   "claude-code",
		Project:  "agenthive",
		Priority: "normal",
		Message:  "build complete",
	}
	if err := d.Dispatch(context.Background(), n); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}

	calls := r.snapshot()
	if len(calls) != 1 {
		t.Fatalf("got %d exec calls, want 1: %+v", len(calls), calls)
	}
	c := calls[0]
	if c.name != "osascript" {
		t.Fatalf("exec name = %q, want %q", c.name, "osascript")
	}
	// We expect a -e flag followed by the AppleScript expression.
	if len(c.args) < 2 || c.args[0] != "-e" {
		t.Fatalf("args do not start with -e: %v", c.args)
	}
	script := c.args[1]
	if !strings.Contains(script, "display notification") {
		t.Fatalf("script missing display notification: %q", script)
	}
	if !strings.Contains(script, "build complete") {
		t.Fatalf("script missing body: %q", script)
	}
	if !strings.Contains(script, "claude-code") {
		t.Fatalf("script missing source: %q", script)
	}
}

func TestDesktopSurface_Dispatch_UnsupportedNoOp(t *testing.T) {
	r := &recordingExec{}
	d := newDesktopSurfaceForOS(t, r, "windows")

	if err := d.Dispatch(context.Background(), protocols.Notification{Message: "hi"}); err != nil {
		t.Fatalf("Dispatch on unsupported OS returned error: %v", err)
	}
	if calls := r.snapshot(); len(calls) != 0 {
		t.Fatalf("unsupported OS should not exec anything, got %v", calls)
	}
}

func TestDesktopSurface_DispatchAction_Linux(t *testing.T) {
	r := &recordingExec{}
	d := newDesktopSurfaceForOS(t, r, "linux")

	a := protocols.ActionRequest{
		ActionID: "abc-123",
		ToolName: "Bash",
	}
	if err := d.DispatchAction(context.Background(), a); err != nil {
		t.Fatalf("DispatchAction returned error: %v", err)
	}
	calls := r.snapshot()
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	c := calls[0]
	if c.name != "notify-send" {
		t.Fatalf("exec name = %q, want notify-send", c.name)
	}
	joined := strings.Join(c.args, " ")
	if !strings.Contains(joined, "Bash") {
		t.Fatalf("action tool missing from args: %v", c.args)
	}
	if !strings.Contains(joined, "abc-123") {
		t.Fatalf("action id missing from args: %v", c.args)
	}
}

func TestDesktopSurface_DispatchAction_Darwin(t *testing.T) {
	r := &recordingExec{}
	d := newDesktopSurfaceForOS(t, r, "darwin")

	a := protocols.ActionRequest{
		ActionID: "abc-123",
		ToolName: "Bash",
	}
	if err := d.DispatchAction(context.Background(), a); err != nil {
		t.Fatalf("DispatchAction returned error: %v", err)
	}
	calls := r.snapshot()
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	c := calls[0]
	if c.name != "osascript" {
		t.Fatalf("exec name = %q, want osascript", c.name)
	}
	script := strings.Join(c.args, " ")
	if !strings.Contains(script, "Bash") || !strings.Contains(script, "abc-123") {
		t.Fatalf("script missing action data: %q", script)
	}
}

func TestDesktopSurface_DispatchAction_UnsupportedNoOp(t *testing.T) {
	r := &recordingExec{}
	d := newDesktopSurfaceForOS(t, r, "plan9")

	if err := d.DispatchAction(context.Background(), protocols.ActionRequest{ActionID: "x"}); err != nil {
		t.Fatalf("DispatchAction on unsupported OS returned error: %v", err)
	}
	if calls := r.snapshot(); len(calls) != 0 {
		t.Fatalf("unsupported OS should not exec anything, got %v", calls)
	}
}

func TestDesktopSurface_Dispatch_PropagatesExecError(t *testing.T) {
	want := errors.New("notify-send failed")
	r := &recordingExec{err: want}
	d := newDesktopSurfaceForOS(t, r, "linux")

	err := d.Dispatch(context.Background(), protocols.Notification{Message: "hi"})
	if err == nil {
		t.Fatal("expected error from Dispatch, got nil")
	}
	if !errors.Is(err, want) {
		t.Fatalf("error %v does not wrap %v", err, want)
	}
}

func TestDesktopSurface_Dispatch_RespectsCancelledContext(t *testing.T) {
	r := &recordingExec{}
	d := newDesktopSurfaceForOS(t, r, "linux")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := d.Dispatch(ctx, protocols.Notification{Message: "hi"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Dispatch with cancelled ctx returned %v, want context.Canceled", err)
	}
	if calls := r.snapshot(); len(calls) != 0 {
		t.Fatalf("cancelled ctx should not exec, got %v", calls)
	}
}

func TestDesktopSurface_Close_NoOp(t *testing.T) {
	r := &recordingExec{}
	d := newDesktopSurfaceForOS(t, r, "linux")
	if err := d.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	// Second Close should also be fine.
	if err := d.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
}

func TestDesktopSurface_DarwinEscapesQuotesInBody(t *testing.T) {
	r := &recordingExec{}
	d := newDesktopSurfaceForOS(t, r, "darwin")

	// AppleScript double-quoted strings break on embedded ". The surface
	// should escape them so the script remains well-formed.
	n := protocols.Notification{
		Source:  "src",
		Message: `hello "world"`,
	}
	if err := d.Dispatch(context.Background(), n); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	calls := r.snapshot()
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	script := calls[0].args[1]
	// The raw `hello "world"` must not appear unescaped because that would
	// terminate the AppleScript string literal mid-stream.
	if strings.Contains(script, `"hello "world""`) {
		t.Fatalf("script contains unescaped double-quoted body: %q", script)
	}
}
