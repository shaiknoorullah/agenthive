package dispatch

import (
	"context"
	"os"
	"sync"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// LogSurface is the minimum-viable Surface: it appends one JSON line per
// dispatch to a file. The line shape is:
//
//	{"ts":"...","kind":"notification","payload":{...}}
//	{"ts":"...","kind":"action","payload":{...}}
//
// Writes are serialised through mu so concurrent callers produce one valid
// JSON line each.
type LogSurface struct {
	path string
	mu   sync.Mutex
	f    *os.File
}

// NewLogSurface opens (or creates) the log file at path with mode 0600 and
// O_APPEND.
func NewLogSurface(path string) (*LogSurface, error) {
	panic("not implemented: dispatch.NewLogSurface")
}

// Name returns "log".
func (l *LogSurface) Name() string {
	panic("not implemented: dispatch.LogSurface.Name")
}

// Dispatch appends a notification line to the log file.
func (l *LogSurface) Dispatch(ctx context.Context, n protocols.Notification) error {
	panic("not implemented: dispatch.LogSurface.Dispatch")
}

// DispatchAction appends an action line to the log file.
func (l *LogSurface) DispatchAction(ctx context.Context, a protocols.ActionRequest) error {
	panic("not implemented: dispatch.LogSurface.DispatchAction")
}

// Close closes the underlying file.
func (l *LogSurface) Close() error {
	panic("not implemented: dispatch.LogSurface.Close")
}
