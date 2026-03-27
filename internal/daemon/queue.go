package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/shaiknoorullah/agenthive/internal/protocol"
)

// Queue is a disk-backed message queue for offline peers.
// Messages are stored as newline-delimited JSON, one file per target peer.
// The queue directory is created on first use.
// Thread-safe for concurrent Enqueue/Drain calls.
type Queue struct {
	mu  sync.Mutex
	dir string
}

// NewQueue creates a new disk-backed queue in the given directory.
func NewQueue(dir string) (*Queue, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create queue directory: %w", err)
	}
	return &Queue{dir: dir}, nil
}

// peerFile returns the path to the queue file for a given peer.
func (q *Queue) peerFile(peerID string) string {
	return filepath.Join(q.dir, peerID+".ndjson")
}

// Enqueue appends a message to the queue file for the given peer.
func (q *Queue) Enqueue(peerID string, msg protocol.Message) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	f, err := os.OpenFile(q.peerFile(peerID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open queue file: %w", err)
	}
	defer f.Close() //nolint:errcheck // best-effort close on deferred path

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write to queue: %w", err)
	}

	return nil
}

// Drain reads all queued messages for a peer and removes the queue file.
// Returns an empty slice if no messages are queued.
func (q *Queue) Drain(peerID string) ([]protocol.Message, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	path := q.peerFile(peerID)

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open queue file: %w", err)
	}
	defer f.Close() //nolint:errcheck // best-effort close on deferred path

	var msgs []protocol.Message
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg protocol.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("unmarshal queued message: %w", err)
		}
		msgs = append(msgs, msg)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan queue file: %w", err)
	}

	// Close file before removing
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close queue file: %w", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove queue file: %w", err)
	}

	return msgs, nil
}

// Depth returns the number of queued messages for a peer.
func (q *Queue) Depth(peerID string) int {
	q.mu.Lock()
	defer q.mu.Unlock()

	path := q.peerFile(peerID)
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close() //nolint:errcheck // best-effort close on read-only file

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if len(scanner.Bytes()) > 0 {
			count++
		}
	}
	return count
}
