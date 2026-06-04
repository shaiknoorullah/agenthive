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

// tmuxRecordingExec is a CmdExecutor that records every call. It is safe for
// concurrent use by multiple goroutines.
type tmuxRecordingExec struct {
	mu    sync.Mutex
	calls []tmuxRecordedCall
	err   error
}

type tmuxRecordedCall struct {
	name string
	args []string
}

func (r *tmuxRecordingExec) Run(name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Defensive copy so callers mutating args later don't poison the record.
	cp := make([]string, len(args))
	copy(cp, args)
	r.calls = append(r.calls, tmuxRecordedCall{name: name, args: cp})
	return nil, r.err
}

func (r *tmuxRecordingExec) snapshot() []tmuxRecordedCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]tmuxRecordedCall, len(r.calls))
	copy(out, r.calls)
	return out
}

func (r *tmuxRecordingExec) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = nil
}

// findTmuxCall returns the first recorded call whose args contain the given
// option name (e.g. "@notif-msg") and reports whether one was found.
func findTmuxCall(calls []tmuxRecordedCall, option string) (tmuxRecordedCall, bool) {
	for _, c := range calls {
		for _, a := range c.args {
			if a == option {
				return c, true
			}
		}
	}
	return tmuxRecordedCall{}, false
}

// tmuxOptionValue returns the value tmux would set for the given option name in
// the recorded call. The expected shape is:
//
//	tmux set-option -g <option> <value>
func tmuxOptionValue(c tmuxRecordedCall, option string) (string, bool) {
	for i, a := range c.args {
		if a == option && i+1 < len(c.args) {
			return c.args[i+1], true
		}
	}
	return "", false
}

func TestTmuxSurface_Name(t *testing.T) {
	s := NewTmuxSurface(&tmuxRecordingExec{})
	if got := s.Name(); got != "tmux" {
		t.Fatalf("Name() = %q, want %q", got, "tmux")
	}
}

func TestTmuxSurface_DispatchWritesAllOptions(t *testing.T) {
	exec := &tmuxRecordingExec{}
	s := NewTmuxSurface(exec)

	ts := time.Date(2026, 6, 4, 12, 30, 45, 0, time.UTC)
	n := protocols.Notification{
		SessionID: "sess-1",
		Source:    "claude",
		Project:   "agenthive",
		Priority:  "high",
		Message:   "build finished",
		Timestamp: ts,
	}
	if err := s.Dispatch(context.Background(), n); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}

	calls := exec.snapshot()
	want := map[string]string{
		"@notif-msg":      "build finished",
		"@notif-project":  "agenthive",
		"@notif-source":   "claude",
		"@notif-time":     ts.Format(time.RFC3339),
		"@notif-priority": "high",
	}
	for opt, expected := range want {
		call, ok := findTmuxCall(calls, opt)
		if !ok {
			t.Fatalf("missing tmux call for option %q; got calls=%+v", opt, calls)
		}
		if call.name != "tmux" {
			t.Fatalf("call for %q used binary %q, want %q", opt, call.name, "tmux")
		}
		// We expect the exact shape: tmux set-option -g <option> <value>
		if len(call.args) < 4 {
			t.Fatalf("call for %q has too few args: %v", opt, call.args)
		}
		if call.args[0] != "set-option" || call.args[1] != "-g" {
			t.Fatalf("call for %q has wrong prefix: %v", opt, call.args)
		}
		got, _ := tmuxOptionValue(call, opt)
		if got != expected {
			t.Fatalf("option %q = %q, want %q", opt, got, expected)
		}
	}
}

func TestTmuxSurface_DispatchActionWritesActionOptions(t *testing.T) {
	exec := &tmuxRecordingExec{}
	s := NewTmuxSurface(exec)

	a := protocols.ActionRequest{
		ActionID: "act-42",
		ToolName: "Bash",
	}
	if err := s.DispatchAction(context.Background(), a); err != nil {
		t.Fatalf("DispatchAction returned error: %v", err)
	}

	calls := exec.snapshot()
	idCall, ok := findTmuxCall(calls, "@notif-action-id")
	if !ok {
		t.Fatalf("missing @notif-action-id call; got %+v", calls)
	}
	if v, _ := tmuxOptionValue(idCall, "@notif-action-id"); v != "act-42" {
		t.Fatalf("@notif-action-id = %q, want %q", v, "act-42")
	}
	toolCall, ok := findTmuxCall(calls, "@notif-action-tool")
	if !ok {
		t.Fatalf("missing @notif-action-tool call; got %+v", calls)
	}
	if v, _ := tmuxOptionValue(toolCall, "@notif-action-tool"); v != "Bash" {
		t.Fatalf("@notif-action-tool = %q, want %q", v, "Bash")
	}
}

func TestTmuxSurface_DispatchShellSafeMessage(t *testing.T) {
	// The message body may contain shell metacharacters. Because we invoke
	// tmux via exec.Command (no shell), each argv element is passed verbatim
	// and never interpreted. The recorded arg must therefore equal the input
	// byte-for-byte, with no escaping or quoting applied by the surface.
	exec := &tmuxRecordingExec{}
	s := NewTmuxSurface(exec)

	dangerous := "build done; rm -rf / `whoami` $(id) \"quoted\" 'single'"
	n := protocols.Notification{Message: dangerous}
	if err := s.Dispatch(context.Background(), n); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	call, ok := findTmuxCall(exec.snapshot(), "@notif-msg")
	if !ok {
		t.Fatalf("missing @notif-msg call")
	}
	v, _ := tmuxOptionValue(call, "@notif-msg")
	if v != dangerous {
		t.Fatalf("@notif-msg = %q, want %q (must be passed verbatim, never via shell)", v, dangerous)
	}
}

func TestTmuxSurface_DispatchOmitsEmptyProject(t *testing.T) {
	// Project is optional. When empty, the surface should not bother
	// writing @notif-project (or should write an empty string, but in any
	// case it must not crash).
	exec := &tmuxRecordingExec{}
	s := NewTmuxSurface(exec)

	n := protocols.Notification{Message: "hi"}
	if err := s.Dispatch(context.Background(), n); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	// We still expect @notif-msg to be written.
	if _, ok := findTmuxCall(exec.snapshot(), "@notif-msg"); !ok {
		t.Fatalf("expected @notif-msg call even with empty project")
	}
}

func TestTmuxSurface_DispatchPropagatesExecError(t *testing.T) {
	exec := &tmuxRecordingExec{err: errors.New("tmux not running")}
	s := NewTmuxSurface(exec)

	err := s.Dispatch(context.Background(), protocols.Notification{Message: "x"})
	if err == nil {
		t.Fatal("expected error from failed exec, got nil")
	}
	if !strings.Contains(err.Error(), "tmux not running") {
		t.Fatalf("error %q does not mention underlying cause", err)
	}
}

func TestTmuxSurface_DispatchHonorsContext(t *testing.T) {
	exec := &tmuxRecordingExec{}
	s := NewTmuxSurface(exec)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := s.Dispatch(ctx, protocols.Notification{Message: "x"}); err == nil {
		t.Fatal("expected ctx error from cancelled context, got nil")
	}
	if len(exec.snapshot()) != 0 {
		t.Fatalf("Dispatch invoked tmux after ctx cancel: %+v", exec.snapshot())
	}
}

func TestTmuxSurface_CloseClearsOptions(t *testing.T) {
	exec := &tmuxRecordingExec{}
	s := NewTmuxSurface(exec)

	// Dispatch first so the surface knows it set options.
	if err := s.Dispatch(context.Background(), protocols.Notification{
		Message:  "hi",
		Project:  "p",
		Source:   "claude",
		Priority: "high",
	}); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if err := s.DispatchAction(context.Background(), protocols.ActionRequest{
		ActionID: "x",
		ToolName: "Bash",
	}); err != nil {
		t.Fatalf("DispatchAction returned error: %v", err)
	}

	// Now close and verify every @notif-* option gets unset via
	// `tmux set-option -gu @notif-*`. The exact set of options unset must
	// cover every option Dispatch* may have set.
	exec.reset()
	if err := s.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	calls := exec.snapshot()

	mustUnset := []string{
		"@notif-msg",
		"@notif-project",
		"@notif-source",
		"@notif-time",
		"@notif-priority",
		"@notif-action-id",
		"@notif-action-tool",
	}
	for _, opt := range mustUnset {
		found := false
		for _, c := range calls {
			if c.name != "tmux" {
				continue
			}
			// Expected shape: tmux set-option -gu <option>
			hasUnset := false
			hasOpt := false
			for _, a := range c.args {
				if a == "-gu" || a == "-u" {
					hasUnset = true
				}
				if a == opt {
					hasOpt = true
				}
			}
			if hasUnset && hasOpt {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Close did not unset option %q; calls=%+v", opt, calls)
		}
	}
}

func TestTmuxSurface_CloseIsIdempotent(t *testing.T) {
	exec := &tmuxRecordingExec{}
	s := NewTmuxSurface(exec)

	if err := s.Close(); err != nil {
		t.Fatalf("first Close returned error: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
}

func TestTmuxSurface_ConcurrentDispatch(t *testing.T) {
	// Smoke test under -race: many concurrent dispatches must not race on
	// internal bookkeeping (the set of options that need unsetting on Close).
	exec := &tmuxRecordingExec{}
	s := NewTmuxSurface(exec)

	var wg sync.WaitGroup
	const goroutines = 16
	const iterations = 32
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = s.Dispatch(context.Background(), protocols.Notification{Message: "m"})
				_ = s.DispatchAction(context.Background(), protocols.ActionRequest{ActionID: "a"})
			}
		}()
	}
	wg.Wait()
	if err := s.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}
