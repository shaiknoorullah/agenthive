package dispatch

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// logLine is the shape every line in the log file must satisfy. Payload is
// kept as a raw message so each test can decode it into the concrete type
// (Notification or ActionRequest) and assert on field values.
type logLine struct {
	TS      time.Time       `json:"ts"`
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

func tempLogPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "surface.log")
}

func readLines(t *testing.T, path string) []logLine {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	var out []logLine
	scanner := bufio.NewScanner(f)
	// Allow long lines (default is 64 KiB which is plenty here, but be safe).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var l logLine
		if err := json.Unmarshal(line, &l); err != nil {
			t.Fatalf("invalid JSON line %q: %v", string(line), err)
		}
		out = append(out, l)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan log: %v", err)
	}
	return out
}

func TestNewLogSurface_CreatesFile(t *testing.T) {
	path := tempLogPath(t)

	s, err := NewLogSurface(path)
	if err != nil {
		t.Fatalf("NewLogSurface: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected log file to exist: %v", err)
	}
}

func TestNewLogSurface_FilePermissionsAre0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file mode bits not meaningful on Windows")
	}
	path := tempLogPath(t)

	s, err := NewLogSurface(path)
	if err != nil {
		t.Fatalf("NewLogSurface: %v", err)
	}
	defer s.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat log: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("log file mode = %o, want 0600", mode)
	}
}

func TestNewLogSurface_AppendsToExistingFile(t *testing.T) {
	path := tempLogPath(t)

	s1, err := NewLogSurface(path)
	if err != nil {
		t.Fatalf("NewLogSurface #1: %v", err)
	}
	if err := s1.Dispatch(context.Background(), protocols.Notification{Message: "one"}); err != nil {
		t.Fatalf("dispatch #1: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("close #1: %v", err)
	}

	s2, err := NewLogSurface(path)
	if err != nil {
		t.Fatalf("NewLogSurface #2: %v", err)
	}
	defer s2.Close()
	if err := s2.Dispatch(context.Background(), protocols.Notification{Message: "two"}); err != nil {
		t.Fatalf("dispatch #2: %v", err)
	}

	lines := readLines(t, path)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}

	var n0, n1 protocols.Notification
	if err := json.Unmarshal(lines[0].Payload, &n0); err != nil {
		t.Fatalf("decode payload 0: %v", err)
	}
	if err := json.Unmarshal(lines[1].Payload, &n1); err != nil {
		t.Fatalf("decode payload 1: %v", err)
	}
	if n0.Message != "one" || n1.Message != "two" {
		t.Fatalf("messages = %q, %q; want %q, %q", n0.Message, n1.Message, "one", "two")
	}
}

func TestLogSurface_Name(t *testing.T) {
	s, err := NewLogSurface(tempLogPath(t))
	if err != nil {
		t.Fatalf("NewLogSurface: %v", err)
	}
	defer s.Close()

	if got := s.Name(); got != "log" {
		t.Fatalf("Name() = %q, want %q", got, "log")
	}
}

func TestLogSurface_Dispatch_WritesNotificationLine(t *testing.T) {
	path := tempLogPath(t)
	s, err := NewLogSurface(path)
	if err != nil {
		t.Fatalf("NewLogSurface: %v", err)
	}
	defer s.Close()

	n := protocols.Notification{
		SessionID: "sess-1",
		Source:    "claude-code",
		Project:   "agenthive",
		Priority:  "info",
		Message:   "hello world",
		Timestamp: time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC),
	}
	before := time.Now()
	if err := s.Dispatch(context.Background(), n); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	after := time.Now()

	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	if lines[0].Kind != "notification" {
		t.Fatalf("kind = %q, want notification", lines[0].Kind)
	}
	// The "ts" envelope field is the dispatch wall-clock time, distinct from
	// the inner Notification.Timestamp. It must fall within the call window.
	if lines[0].TS.Before(before.Add(-time.Second)) || lines[0].TS.After(after.Add(time.Second)) {
		t.Fatalf("envelope ts %v outside [%v, %v]", lines[0].TS, before, after)
	}

	var got protocols.Notification
	if err := json.Unmarshal(lines[0].Payload, &got); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if got != n {
		t.Fatalf("payload mismatch: got %+v want %+v", got, n)
	}
}

func TestLogSurface_DispatchAction_WritesActionLine(t *testing.T) {
	path := tempLogPath(t)
	s, err := NewLogSurface(path)
	if err != nil {
		t.Fatalf("NewLogSurface: %v", err)
	}
	defer s.Close()

	a := protocols.ActionRequest{
		ActionID:  "act-1",
		SessionID: "sess-1",
		ToolUseID: "tu-1",
		ToolName:  "Bash",
		ToolInput: "rm -rf /tmp/foo",
		Project:   "agenthive",
		CWD:       "/home/user",
		Timestamp: time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2026, 6, 4, 12, 0, 30, 0, time.UTC),
	}
	if err := s.DispatchAction(context.Background(), a); err != nil {
		t.Fatalf("DispatchAction: %v", err)
	}

	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	if lines[0].Kind != "action" {
		t.Fatalf("kind = %q, want action", lines[0].Kind)
	}
	var got protocols.ActionRequest
	if err := json.Unmarshal(lines[0].Payload, &got); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if got != a {
		t.Fatalf("payload mismatch: got %+v want %+v", got, a)
	}
}

func TestLogSurface_MultipleDispatches_OneLinePerCall(t *testing.T) {
	path := tempLogPath(t)
	s, err := NewLogSurface(path)
	if err != nil {
		t.Fatalf("NewLogSurface: %v", err)
	}
	defer s.Close()

	for i := 0; i < 5; i++ {
		if err := s.Dispatch(context.Background(), protocols.Notification{Message: fmt.Sprintf("m-%d", i)}); err != nil {
			t.Fatalf("dispatch %d: %v", i, err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := s.DispatchAction(context.Background(), protocols.ActionRequest{ActionID: fmt.Sprintf("a-%d", i)}); err != nil {
			t.Fatalf("dispatch action %d: %v", i, err)
		}
	}

	lines := readLines(t, path)
	if len(lines) != 8 {
		t.Fatalf("got %d lines, want 8", len(lines))
	}
	notifs, actions := 0, 0
	for _, l := range lines {
		switch l.Kind {
		case "notification":
			notifs++
		case "action":
			actions++
		default:
			t.Fatalf("unexpected kind %q", l.Kind)
		}
	}
	if notifs != 5 || actions != 3 {
		t.Fatalf("notifs=%d actions=%d, want 5 and 3", notifs, actions)
	}
}

func TestLogSurface_ConcurrentDispatch(t *testing.T) {
	path := tempLogPath(t)
	s, err := NewLogSurface(path)
	if err != nil {
		t.Fatalf("NewLogSurface: %v", err)
	}
	defer s.Close()

	const goroutines = 32
	const perGoroutine = 20
	const want = goroutines * perGoroutine

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				n := protocols.Notification{
					Message: fmt.Sprintf("g=%d i=%d", g, i),
				}
				if err := s.Dispatch(context.Background(), n); err != nil {
					t.Errorf("dispatch g=%d i=%d: %v", g, i, err)
					return
				}
			}
		}()
	}
	wg.Wait()

	lines := readLines(t, path)
	if len(lines) != want {
		t.Fatalf("got %d lines, want %d", len(lines), want)
	}
	for i, l := range lines {
		if l.Kind != "notification" {
			t.Fatalf("line %d kind = %q, want notification", i, l.Kind)
		}
		var n protocols.Notification
		if err := json.Unmarshal(l.Payload, &n); err != nil {
			t.Fatalf("line %d payload decode: %v", i, err)
		}
		if !strings.HasPrefix(n.Message, "g=") {
			t.Fatalf("line %d unexpected message: %q", i, n.Message)
		}
	}
}

func TestLogSurface_Close_IsIdempotent(t *testing.T) {
	s, err := NewLogSurface(tempLogPath(t))
	if err != nil {
		t.Fatalf("NewLogSurface: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestLogSurface_DispatchAfterClose_ReturnsError(t *testing.T) {
	s, err := NewLogSurface(tempLogPath(t))
	if err != nil {
		t.Fatalf("NewLogSurface: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := s.Dispatch(context.Background(), protocols.Notification{}); err == nil {
		t.Fatal("Dispatch after Close: expected error, got nil")
	}
	if err := s.DispatchAction(context.Background(), protocols.ActionRequest{}); err == nil {
		t.Fatal("DispatchAction after Close: expected error, got nil")
	}
}

func TestNewLogSurface_UnopenableReturnsError(t *testing.T) {
	// A path under a non-existent parent directory should fail to open.
	bad := filepath.Join(t.TempDir(), "does-not-exist", "surface.log")
	if _, err := NewLogSurface(bad); err == nil {
		t.Fatal("expected error opening log under missing directory, got nil")
	}
}

func TestLogSurface_HonorsCtxCancellation(t *testing.T) {
	// Cancelled context should be reported promptly. The surface is allowed
	// to either skip the write or perform it before returning the error;
	// callers only require ctx.Err() (or a wrapping error) to be reported.
	s, err := NewLogSurface(tempLogPath(t))
	if err != nil {
		t.Fatalf("NewLogSurface: %v", err)
	}
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = s.Dispatch(ctx, protocols.Notification{})
	if err == nil {
		t.Fatal("expected ctx error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Dispatch err = %v, want context.Canceled", err)
	}
}
