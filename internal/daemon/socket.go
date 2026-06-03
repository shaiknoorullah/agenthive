package daemon

import (
	"context"

	"github.com/shaiknoorullah/agenthive/internal/hooks"
)

// SocketServer accepts hook IPC connections on a Unix socket. One connection
// is one request and one response.
//
// Wire format on the socket: framed JSON.
//
//	Request:  {"kind":"action_request",  "payload": ActionRequest}
//	Response: {"kind":"action_response", "payload": ActionResponse}
//	          {"kind":"error",            "payload": {"message": "..."}}
//
// The socket file is created at path with mode 0600.
type SocketServer struct {
	path string
	gate *hooks.Gate
}

// NewSocketServer returns a SocketServer that listens on path and routes
// action requests through gate.
func NewSocketServer(path string, gate *hooks.Gate) *SocketServer {
	panic("not implemented: daemon.NewSocketServer")
}

// Run binds the Unix socket and accepts connections until ctx is done.
// It blocks until ctx.Done() is closed (or the listener errors out).
func (s *SocketServer) Run(ctx context.Context) error {
	panic("not implemented: daemon.SocketServer.Run")
}
