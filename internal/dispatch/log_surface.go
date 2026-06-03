package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// ErrLogSurfaceClosed is returned by Dispatch / DispatchAction after Close.
var ErrLogSurfaceClosed = errors.New("dispatch: log surface closed")

// LogSurface is the minimum-viable Surface: it appends one JSON line per
// dispatch to a file. The line shape is:
//
//	{"ts":"...","kind":"notification","payload":{...}}
//	{"ts":"...","kind":"action","payload":{...}}
//
// "ts" is the wall-clock time the dispatch was recorded by the surface,
// distinct from any timestamp inside the payload.
//
// Writes are serialised through mu so concurrent callers each produce one
// well-formed line on disk. The file is opened with O_APPEND, so the kernel
// guarantees each write is atomic relative to other writers on the same fd
// (we still hold mu to avoid interleaving with our own marshal step).
type LogSurface struct {
	path string

	mu     sync.Mutex
	f      *os.File
	closed bool

	// nowFn is overridable in tests; callers in production never touch it.
	nowFn func() time.Time
}

// logEnvelope is the on-disk shape. We always marshal the envelope rather
// than hand-build the JSON so the encoder handles escaping for us.
type logEnvelope struct {
	TS      time.Time `json:"ts"`
	Kind    string    `json:"kind"`
	Payload any       `json:"payload"`
}

const (
	kindNotification = "notification"
	kindAction       = "action"

	logFileMode = 0o600
	logOpenFlag = os.O_CREATE | os.O_APPEND | os.O_WRONLY
)

// NewLogSurface opens (or creates) the log file at path with mode 0600 and
// O_APPEND. If the file exists it is appended to.
func NewLogSurface(path string) (*LogSurface, error) {
	f, err := os.OpenFile(path, logOpenFlag, logFileMode)
	if err != nil {
		return nil, fmt.Errorf("dispatch: open log %q: %w", path, err)
	}
	// In case the file existed with looser permissions, force-narrow it. We
	// deliberately ignore failures here (e.g. on filesystems that don't
	// support chmod) — the open succeeded which is what callers depend on.
	_ = os.Chmod(path, logFileMode)

	return &LogSurface{
		path:  path,
		f:     f,
		nowFn: time.Now,
	}, nil
}

// Name returns "log".
func (l *LogSurface) Name() string { return "log" }

// Dispatch appends a notification line to the log file.
func (l *LogSurface) Dispatch(ctx context.Context, n protocols.Notification) error {
	return l.write(ctx, kindNotification, n)
}

// DispatchAction appends an action line to the log file.
func (l *LogSurface) DispatchAction(ctx context.Context, a protocols.ActionRequest) error {
	return l.write(ctx, kindAction, a)
}

// write marshals the envelope and appends it as a single line.
//
// We honour ctx by checking it once before touching the file: if the caller
// already cancelled, we refuse to write rather than silently performing IO
// the caller no longer wants. We do not attempt to interrupt an in-flight
// write — local filesystem writes are fast and uncancellable in practice.
func (l *LogSurface) write(ctx context.Context, kind string, payload any) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	env := logEnvelope{
		TS:      l.nowFn().UTC(),
		Kind:    kind,
		Payload: payload,
	}
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("dispatch: marshal log line: %w", err)
	}
	// Append the newline so each call produces exactly one line. Build the
	// full byte slice up-front so a single Write reaches the kernel — under
	// O_APPEND that write is atomic relative to other writers, which keeps
	// the file parseable even if some external process happens to share the
	// fd in the future.
	line := make([]byte, 0, len(body)+1)
	line = append(line, body...)
	line = append(line, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return ErrLogSurfaceClosed
	}
	if _, err := l.f.Write(line); err != nil {
		return fmt.Errorf("dispatch: write log line: %w", err)
	}
	return nil
}

// Close closes the underlying file. It is safe to call multiple times — the
// second and subsequent calls return nil.
func (l *LogSurface) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if l.f == nil {
		return nil
	}
	if err := l.f.Close(); err != nil {
		return fmt.Errorf("dispatch: close log: %w", err)
	}
	l.f = nil
	return nil
}
