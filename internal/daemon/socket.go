package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/hooks"
	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// Envelope kinds carried on the Unix socket. The wire format is framed JSON
// (a 4-byte big-endian length prefix followed by the JSON body, identical to
// the libp2p stream framing in package protocols) where each envelope is a
// SocketEnvelope with a discriminator (Kind) and a raw JSON payload sized to
// the discriminator.
//
// The list_* request kinds power the TUI's snapshot polling (cmd_tui). Each
// request kind has a matching response kind so the client can tell a response
// to its query from an unrelated envelope on the same fd.
const (
	KindActionRequest  = "action_request"
	KindActionResponse = "action_response"
	KindError          = "error"

	KindListPeers           = "list_peers"
	KindListPeersResponse   = "list_peers_response"
	KindListRoutes          = "list_routes"
	KindListRoutesResponse  = "list_routes_response"
	KindListActions         = "list_actions"
	KindListActionsResponse = "list_actions_response"
	KindListLogs            = "list_logs"
	KindListLogsResponse    = "list_logs_response"
)

// SocketEnvelope is the on-wire frame used between cmd/agenthive's hook
// subcommand and the running daemon. Payload is left raw so the server can
// decode it lazily based on Kind without having to declare a union type.
type SocketEnvelope struct {
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

// SocketError is the body of a kind:"error" envelope. The hook subcommand
// treats any error envelope as "fall back to Claude's built-in prompt",
// never as a hard failure.
type SocketError struct {
	Message string `json:"message"`
}

// PeerEntry is a single (id, info) pair in a ListPeersResponse. We do not
// expose the underlying map[string]PeerInfo directly so the wire format has a
// stable, ordered shape the TUI can render row by row.
type PeerEntry struct {
	ID   string        `json:"id"`
	Info crdt.PeerInfo `json:"info"`
}

// ListPeersResponse is the body of a list_peers_response envelope.
type ListPeersResponse struct {
	Peers []PeerEntry `json:"peers"`
}

// RouteEntry is a single (id, rule) pair in a ListRoutesResponse.
type RouteEntry struct {
	ID   string         `json:"id"`
	Rule crdt.RouteRule `json:"rule"`
}

// ListRoutesResponse is the body of a list_routes_response envelope.
type ListRoutesResponse struct {
	Routes []RouteEntry `json:"routes"`
}

// ActionEntry is a single pending action in a ListActionsResponse. Action
// carries the full ActionRequest so the TUI can render the tool name + input
// without a second round-trip.
type ActionEntry struct {
	Action protocols.ActionRequest `json:"action"`
}

// ListActionsResponse is the body of a list_actions_response envelope.
type ListActionsResponse struct {
	Actions []ActionEntry `json:"actions"`
}

// ListLogsRequest is the body of a list_logs request envelope. Limit is the
// maximum number of recent log lines to return; <=0 means "use the server's
// default cap" (currently 500).
type ListLogsRequest struct {
	Limit int `json:"limit,omitempty"`
}

// ListLogsResponse is the body of a list_logs_response envelope. Lines are
// raw JSONL strings (each line is one logEnvelope as written by LogSurface).
// The TUI parses them per-row so a single malformed line cannot crash the
// view.
type ListLogsResponse struct {
	Lines []string `json:"lines"`
}

// defaultLogLimit caps the number of lines returned by list_logs when the
// caller does not specify Limit. The TUI's logs tab paints at most ~50 rows
// at a time but we hand back enough headroom to scroll without re-querying.
const defaultLogLimit = 500

// QueryBackend is the read-only surface SocketServer needs in order to answer
// list_* envelopes. The Daemon implements it; tests can substitute a stub.
type QueryBackend interface {
	ListPeers() []PeerEntry
	ListRoutes() []RouteEntry
	ListActions() ([]ActionEntry, error)
	TailLogLines(limit int) ([]string, error)
}

// SocketServer accepts hook IPC connections on a Unix socket. One connection
// is one request and one response.
//
// Wire format on the socket: framed JSON.
//
//	Request:  {"kind":"action_request",  "payload": ActionRequest}
//	Response: {"kind":"action_response", "payload": ActionResponse}
//	          {"kind":"error",            "payload": {"message": "..."}}
//
// The socket file is created at path with mode 0600. If a stale socket file is
// already present (e.g. from a previous crash), it is removed before binding.
// On graceful shutdown via the supplied context, the socket file is unlinked
// so subsequent restarts can bind cleanly.
type SocketServer struct {
	path  string
	gate  *hooks.Gate
	query QueryBackend
}

// NewSocketServer returns a SocketServer that listens on path and routes
// action_request envelopes through gate. query may be nil — in that case
// list_* envelopes return an error envelope. Production callers pass the
// Daemon itself (which implements QueryBackend) so the TUI can query state.
func NewSocketServer(path string, gate *hooks.Gate) *SocketServer {
	return &SocketServer{path: path, gate: gate}
}

// SetQueryBackend installs the read-only state backend used to answer list_*
// envelopes. The Daemon calls this after construction so the SocketServer
// can survive being built before the daemon has finished wiring itself up.
// Safe to call before Run.
func (s *SocketServer) SetQueryBackend(q QueryBackend) {
	s.query = q
}

// Run binds the Unix socket and accepts connections until ctx is done.
//
// Each accepted connection is handled in its own goroutine via handleConn,
// which reads exactly one framed envelope, dispatches it through the gate,
// and writes the resulting envelope back. Connection-level errors are logged
// and never bubble up: a misbehaving client must not be able to kill the
// daemon. A bind failure (port-in-use, missing parent directory, permission
// denied, etc.) is returned to the caller so the daemon can react.
//
// On ctx.Done() the listener is closed (which unblocks Accept) and Run waits
// for any in-flight handler goroutines to return before unlinking the socket
// file and returning nil.
func (s *SocketServer) Run(ctx context.Context) error {
	// Best-effort: clear any leftover socket file from a previous crash so
	// net.Listen does not fail with EADDRINUSE on restart.
	if err := removeStaleSocket(s.path); err != nil {
		return fmt.Errorf("daemon: remove stale socket: %w", err)
	}

	listener, err := net.Listen("unix", s.path)
	if err != nil {
		return fmt.Errorf("daemon: listen unix %s: %w", s.path, err)
	}

	// Apply 0600 permissions so only the owner can dial. net.Listen on
	// linux/darwin honours umask, which is usually 0022 → mode 0755 — not
	// what we want. Chmod after bind is racy in theory (a hostile process
	// could connect in the gap), but the parent directory is the config dir
	// which we control, so the race window is closed in practice.
	if err := os.Chmod(s.path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(s.path)
		return fmt.Errorf("daemon: chmod socket: %w", err)
	}

	// Close the listener when ctx fires. The Accept loop will then return an
	// error (which we treat as a clean shutdown signal).
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-stop:
		}
		_ = listener.Close()
	}()
	defer close(stop)

	var wg sync.WaitGroup
	for {
		conn, err := listener.Accept()
		if err != nil {
			// Wait for active handlers before cleaning up — they hold
			// references to the gate and we want their writes to drain so
			// clients don't see a partial frame on shutdown.
			wg.Wait()
			_ = os.Remove(s.path)
			if ctx.Err() != nil {
				return nil
			}
			// A non-context error means the listener crashed for a reason
			// we did not signal. Surface it.
			return fmt.Errorf("daemon: socket accept: %w", err)
		}
		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			s.handleConn(ctx, c)
		}(conn)
	}
}

// handleConn drives one request/response cycle on a single accepted
// connection. It always closes c. It never returns an error: protocol-level
// problems are reported back to the client as kind:"error" envelopes and
// transport-level problems (closed connection, malformed framing) are
// logged.
//
// The connection inherits the server's context, so a server shutdown will
// unblock any in-flight Gate.Handle waiting on the queue.
func (s *SocketServer) handleConn(ctx context.Context, c net.Conn) {
	// Best-effort close: a Unix-domain connection that the kernel has already
	// torn down (peer process exited mid-write) will surface ErrClosed here,
	// which is not actionable. We don't propagate the error because the
	// surrounding goroutine has no caller to return it to.
	defer func() { _ = c.Close() }()

	var env SocketEnvelope
	if err := protocols.ReadFramed(c, &env); err != nil {
		// A client that dialled and immediately closed shows up as EOF
		// here. Don't spam the log for that case.
		if !errors.Is(err, net.ErrClosed) {
			log.Printf("daemon: socket: read frame: %v", err)
		}
		return
	}

	switch env.Kind {
	case KindActionRequest:
		var req protocols.ActionRequest
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			s.writeError(c, fmt.Sprintf("decode action_request: %v", err))
			return
		}
		s.handleActionRequest(ctx, c, req)
	case KindListPeers:
		s.handleListPeers(c)
	case KindListRoutes:
		s.handleListRoutes(c)
	case KindListActions:
		s.handleListActions(c)
	case KindListLogs:
		var req ListLogsRequest
		// Empty payload is fine — defaults apply.
		if len(env.Payload) > 0 && string(env.Payload) != "null" {
			if err := json.Unmarshal(env.Payload, &req); err != nil {
				s.writeError(c, fmt.Sprintf("decode list_logs: %v", err))
				return
			}
		}
		s.handleListLogs(c, req)
	default:
		s.writeError(c, fmt.Sprintf("unknown envelope kind %q", env.Kind))
	}
}

// handleActionRequest routes a single action_request through the gate and
// writes the resulting action_response (or error) envelope back to the
// client. A gate error — including context.Canceled when the server is
// shutting down — is translated into a kind:"error" envelope so the client
// can fall back to Claude's built-in prompt rather than hanging.
func (s *SocketServer) handleActionRequest(ctx context.Context, c net.Conn, req protocols.ActionRequest) {
	resp, err := s.gate.Handle(ctx, req)
	if err != nil {
		s.writeError(c, fmt.Sprintf("gate: %v", err))
		return
	}
	out := SocketEnvelope{Kind: KindActionResponse}
	body, err := json.Marshal(resp)
	if err != nil {
		// Practically unreachable — ActionResponse has only string fields.
		s.writeError(c, fmt.Sprintf("encode action_response: %v", err))
		return
	}
	out.Payload = body
	if err := protocols.WriteFramed(c, out); err != nil {
		log.Printf("daemon: socket: write response: %v", err)
	}
}

// handleListPeers writes a list_peers_response envelope built from the
// query backend's current snapshot.
func (s *SocketServer) handleListPeers(c net.Conn) {
	if s.query == nil {
		s.writeError(c, "list_peers: query backend not installed")
		return
	}
	resp := ListPeersResponse{Peers: s.query.ListPeers()}
	s.writeResponse(c, KindListPeersResponse, resp)
}

// handleListRoutes writes a list_routes_response envelope.
func (s *SocketServer) handleListRoutes(c net.Conn) {
	if s.query == nil {
		s.writeError(c, "list_routes: query backend not installed")
		return
	}
	resp := ListRoutesResponse{Routes: s.query.ListRoutes()}
	s.writeResponse(c, KindListRoutesResponse, resp)
}

// handleListActions writes a list_actions_response envelope.
func (s *SocketServer) handleListActions(c net.Conn) {
	if s.query == nil {
		s.writeError(c, "list_actions: query backend not installed")
		return
	}
	actions, err := s.query.ListActions()
	if err != nil {
		s.writeError(c, fmt.Sprintf("list_actions: %v", err))
		return
	}
	s.writeResponse(c, KindListActionsResponse, ListActionsResponse{Actions: actions})
}

// handleListLogs writes a list_logs_response envelope. The TailLogLines call
// is capped at defaultLogLimit when req.Limit is non-positive.
func (s *SocketServer) handleListLogs(c net.Conn, req ListLogsRequest) {
	if s.query == nil {
		s.writeError(c, "list_logs: query backend not installed")
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultLogLimit
	}
	lines, err := s.query.TailLogLines(limit)
	if err != nil {
		s.writeError(c, fmt.Sprintf("list_logs: %v", err))
		return
	}
	s.writeResponse(c, KindListLogsResponse, ListLogsResponse{Lines: lines})
}

// writeResponse marshals payload as JSON and writes a framed envelope of the
// supplied kind. A marshal error or a transport-level write error is logged
// but never returned — the connection is about to close anyway.
func (s *SocketServer) writeResponse(c net.Conn, kind string, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		s.writeError(c, fmt.Sprintf("encode %s: %v", kind, err))
		return
	}
	if err := protocols.WriteFramed(c, SocketEnvelope{Kind: kind, Payload: body}); err != nil {
		log.Printf("daemon: socket: write %s: %v", kind, err)
	}
}

// writeError sends a kind:"error" envelope and logs the underlying message.
// Errors writing the envelope itself are logged but not surfaced — the
// connection is about to close anyway.
func (s *SocketServer) writeError(c net.Conn, msg string) {
	body, err := json.Marshal(SocketError{Message: msg})
	if err != nil {
		log.Printf("daemon: socket: marshal error envelope: %v", err)
		return
	}
	if err := protocols.WriteFramed(c, SocketEnvelope{Kind: KindError, Payload: body}); err != nil {
		log.Printf("daemon: socket: write error envelope: %v", err)
	}
}

// removeStaleSocket removes a leftover unix-socket file (or, for crash-recovery
// convenience, a regular file at the same path) so net.Listen does not fail
// with EADDRINUSE on restart. Parent-directory creation is the daemon's
// responsibility, not this package's — see Run for why we deliberately surface
// bind failures up to the caller.
func removeStaleSocket(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	// On linux a unix socket reports ModeSocket; on darwin too. A regular
	// file does not — but we remove regular files at the socket path too:
	// the alternative (leaving a stale regular file) bricks the daemon
	// until the user manually deletes it.
	if info.Mode()&os.ModeSocket != 0 || info.Mode().IsRegular() {
		return os.Remove(path)
	}
	return nil
}
