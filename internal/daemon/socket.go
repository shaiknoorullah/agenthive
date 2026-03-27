package daemon

import (
	"github.com/shaiknoorullah/agenthive/internal/protocol"
)

// MessageHandler is called for each message received on the socket.
type MessageHandler func(msg protocol.Message)

// SocketListener listens on a Unix domain socket for hook IPC.
type SocketListener struct{}

func NewSocketListener(path string, handler MessageHandler) (*SocketListener, error) {
	return nil, nil
}
func (s *SocketListener) Serve() error { return nil }
func (s *SocketListener) Close() error { return nil }
func (s *SocketListener) Path() string { return "" }
