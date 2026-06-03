package hooks

import (
	"context"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// Queue is a directory-backed action queue. Pending actions are written as
// <id>.pending files (atomically via temp + rename). Responses are written as
// <id>.response files using O_CREAT|O_EXCL so exactly one writer wins.
type Queue struct {
	dir string
}

// NewQueue returns a Queue rooted at dir. dir is created with mode 0700 if
// missing.
func NewQueue(dir string) (*Queue, error) {
	panic("not implemented: hooks.NewQueue")
}

// WritePending writes <id>.pending atomically (temp + rename).
func (q *Queue) WritePending(action protocols.ActionRequest) error {
	panic("not implemented: hooks.Queue.WritePending")
}

// WaitResponse polls for <id>.response with backoff. Deletes both files on
// success. Returns context.DeadlineExceeded if not seen.
func (q *Queue) WaitResponse(ctx context.Context, actionID string) (protocols.ActionResponse, error) {
	panic("not implemented: hooks.Queue.WaitResponse")
}

// WriteResponse uses O_CREAT|O_EXCL — first writer wins, all others get
// EEXIST.
func (q *Queue) WriteResponse(resp protocols.ActionResponse) error {
	panic("not implemented: hooks.Queue.WriteResponse")
}
