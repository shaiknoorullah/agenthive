package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"github.com/shaiknoorullah/agenthive/internal/protocol"
)

// MessageHandler is called for each message received on the socket.
type MessageHandler func(msg protocol.Message)

// SocketListener listens on a Unix domain socket for hook IPC.
// Hooks connect, send newline-delimited JSON messages, and disconnect.
// Each connection is handled in a separate goroutine.
type SocketListener struct {
	path     string
	handler  MessageHandler
	listener net.Listener
	wg       sync.WaitGroup
	closed   chan struct{}
}

// NewSocketListener creates a Unix socket listener at the given path.
// Removes any existing socket file before binding.
func NewSocketListener(path string, handler MessageHandler) (*SocketListener, error) {
	// Remove stale socket
	_ = os.Remove(path) // best-effort cleanup of stale socket

	lc := net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "unix", path)
	if err != nil {
		return nil, err
	}

	// Set socket permissions to owner-only
	if err := os.Chmod(path, 0600); err != nil {
		_ = ln.Close() // best-effort cleanup
		return nil, fmt.Errorf("set socket permissions: %w", err)
	}

	return &SocketListener{
		path:     path,
		handler:  handler,
		listener: ln,
		closed:   make(chan struct{}),
	}, nil
}

// Serve accepts connections and processes messages.
// Blocks until Close is called.
func (s *SocketListener) Serve() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return nil // normal shutdown
			default:
				return err
			}
		}
		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

// handleConn reads newline-delimited JSON messages from a connection.
func (s *SocketListener) handleConn(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close() //nolint:errcheck // best-effort close on connection teardown

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg protocol.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			log.Printf("socket: invalid JSON from %s: %v", conn.RemoteAddr(), err)
			continue
		}
		s.handler(msg)
	}
}

// Close stops the listener and waits for active connections to finish.
func (s *SocketListener) Close() error {
	close(s.closed)
	err := s.listener.Close()
	s.wg.Wait()
	_ = os.Remove(s.path) // best-effort cleanup of socket file
	return err
}

// Path returns the socket file path.
func (s *SocketListener) Path() string {
	return s.path
}
