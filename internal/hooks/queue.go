package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// pollInterval is the spacing between filesystem checks in WaitResponse. The
// plan requires sub-100ms polling so that the action gate stays responsive
// from the user's perspective. 25ms keeps wall-clock latency from a written
// response to its pickup well under 100ms even with one missed tick.
const pollInterval = 25 * time.Millisecond

// Queue is a directory-backed action queue. Pending actions are written as
// <id>.pending files (atomically via temp + rename). Responses are written as
// <id>.response files using O_CREAT|O_EXCL so exactly one writer wins.
type Queue struct {
	dir string
}

// NewQueue returns a Queue rooted at dir. dir is created with mode 0700 if
// missing. An existing directory is accepted but its mode is left untouched.
func NewQueue(dir string) (*Queue, error) {
	if dir == "" {
		return nil, errors.New("hooks: queue dir must not be empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("hooks: create queue dir: %w", err)
	}
	return &Queue{dir: dir}, nil
}

// pendingPath returns the on-disk location of the pending file for an action.
func (q *Queue) pendingPath(id string) string {
	return filepath.Join(q.dir, id+".pending")
}

// responsePath returns the on-disk location of the response file for an
// action.
func (q *Queue) responsePath(id string) string {
	return filepath.Join(q.dir, id+".response")
}

// WritePending writes <id>.pending atomically: it first writes to a temp file
// in the same directory, fsyncs, then renames into place. A reader that opens
// <id>.pending therefore always sees a complete JSON document.
func (q *Queue) WritePending(action protocols.ActionRequest) error {
	if action.ActionID == "" {
		return errors.New("hooks: WritePending: ActionID must be set")
	}

	body, err := json.Marshal(action)
	if err != nil {
		return fmt.Errorf("hooks: marshal pending: %w", err)
	}

	// CreateTemp picks a unique name within q.dir so concurrent WritePending
	// calls for distinct IDs don't collide on the temp path.
	tmp, err := os.CreateTemp(q.dir, action.ActionID+".pending.*")
	if err != nil {
		return fmt.Errorf("hooks: create temp pending: %w", err)
	}
	tmpName := tmp.Name()

	// Best-effort cleanup if anything below fails.
	cleanup := func() {
		_ = os.Remove(tmpName)
	}

	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("hooks: write temp pending: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("hooks: fsync temp pending: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("hooks: close temp pending: %w", err)
	}

	if err := os.Rename(tmpName, q.pendingPath(action.ActionID)); err != nil {
		cleanup()
		return fmt.Errorf("hooks: rename pending into place: %w", err)
	}
	return nil
}

// WriteResponse atomically claims the <id>.response slot using
// O_CREAT|O_EXCL. The first writer to call WriteResponse for a given action
// ID wins; all subsequent callers receive an error wrapping os.ErrExist.
//
// This is the linchpin that makes the multi-peer action gate safe: the daemon
// invokes WriteResponse from each surface that produces a decision, and only
// the first decision is recorded.
func (q *Queue) WriteResponse(resp protocols.ActionResponse) error {
	if resp.ActionID == "" {
		return errors.New("hooks: WriteResponse: ActionID must be set")
	}

	body, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("hooks: marshal response: %w", err)
	}

	f, err := os.OpenFile(q.responsePath(resp.ActionID),
		os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		// Bubble up so the caller can recognise os.ErrExist if they want.
		return fmt.Errorf("hooks: open response: %w", err)
	}

	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		// Leave the partial file in place — a subsequent reader will get a
		// JSON decode error rather than silently miss the response. In
		// practice the OS rarely fails a small write to a freshly opened
		// file, and the gate will simply time out.
		return fmt.Errorf("hooks: write response: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("hooks: fsync response: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("hooks: close response: %w", err)
	}
	return nil
}

// WaitResponse polls for <id>.response on a sub-100ms cadence until it
// appears, the context is cancelled, or the deadline elapses. On success it
// removes both the pending and response files and returns the decoded
// response. On timeout it returns context.DeadlineExceeded (or
// context.Canceled) and leaves the files in place so the caller can decide
// what to do.
func (q *Queue) WaitResponse(ctx context.Context, actionID string) (protocols.ActionResponse, error) {
	var zero protocols.ActionResponse
	if actionID == "" {
		return zero, errors.New("hooks: WaitResponse: actionID must be set")
	}

	respPath := q.responsePath(actionID)

	// Fast-path: check once before sleeping so an already-written response is
	// picked up immediately.
	if resp, ok, err := q.tryReadResponse(respPath); err != nil {
		return zero, err
	} else if ok {
		return q.consume(actionID, resp)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-ticker.C:
			resp, ok, err := q.tryReadResponse(respPath)
			if err != nil {
				return zero, err
			}
			if !ok {
				continue
			}
			return q.consume(actionID, resp)
		}
	}
}

// tryReadResponse attempts to load and decode the response file at path. It
// returns (response, true, nil) on success, (zero, false, nil) when the file
// does not yet exist, and (zero, false, err) on any other error (including
// JSON decode failures, which usually indicate a torn write).
func (q *Queue) tryReadResponse(path string) (protocols.ActionResponse, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return protocols.ActionResponse{}, false, nil
		}
		return protocols.ActionResponse{}, false, fmt.Errorf("hooks: read response: %w", err)
	}
	// A zero-length file means the writer crashed mid-write. Keep waiting in
	// case a later writer replaces it.
	if len(data) == 0 {
		return protocols.ActionResponse{}, false, nil
	}
	var resp protocols.ActionResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return protocols.ActionResponse{}, false, fmt.Errorf("hooks: unmarshal response: %w", err)
	}
	return resp, true, nil
}

// consume removes the pending and response files associated with actionID and
// returns the supplied response. Removal errors other than "not exist" are
// returned so the caller knows the queue may be in a weird state, but the
// response itself is still delivered: the decision has already been made.
func (q *Queue) consume(actionID string, resp protocols.ActionResponse) (protocols.ActionResponse, error) {
	if err := os.Remove(q.responsePath(actionID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return resp, fmt.Errorf("hooks: cleanup response file: %w", err)
	}
	if err := os.Remove(q.pendingPath(actionID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return resp, fmt.Errorf("hooks: cleanup pending file: %w", err)
	}
	return resp, nil
}
