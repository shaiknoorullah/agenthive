package protocols

import (
	"io"
	"time"
)

// ActionRequest is sent over ProtoActionRequest to ask a remote peer to
// approve or deny a destructive/normal local action.
type ActionRequest struct {
	ActionID  string    `json:"action_id"`
	SessionID string    `json:"session_id"`
	ToolUseID string    `json:"tool_use_id"`
	ToolName  string    `json:"tool_name"`
	ToolInput string    `json:"tool_input"`
	Project   string    `json:"project,omitempty"`
	CWD       string    `json:"cwd,omitempty"`
	Timestamp time.Time `json:"ts"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ActionResponse is the reply to ActionRequest. Decision is one of
// "allow", "deny", or "allow-always".
type ActionResponse struct {
	ActionID  string `json:"action_id"`
	Decision  string `json:"decision"`
	DecidedBy string `json:"decided_by"`
}

// Notification is a one-way informational message sent over
// ProtoNotification.
type Notification struct {
	SessionID string    `json:"session_id"`
	Source    string    `json:"source"`
	Project   string    `json:"project,omitempty"`
	Priority  string    `json:"priority"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"ts"`
}

// PeerAnnounce is sent over ProtoPeerAnnounce when a peer wants to advertise
// its current multiaddrs to a directly-connected peer.
type PeerAnnounce struct {
	PeerID     string    `json:"peer_id"`
	Multiaddrs []string  `json:"multiaddrs"`
	Timestamp  time.Time `json:"ts"`
}

// StateDelta is the payload published on TopicState via GossipSub.
// Peers/Routes/Config are pre-marshaled LWWMap blobs.
type StateDelta struct {
	From   string `json:"from"`
	Peers  []byte `json:"peers"`
	Routes []byte `json:"routes"`
	Config []byte `json:"config"`
}

// WriteFramed encodes v as JSON and writes a 4-byte big-endian length prefix
// followed by the JSON body to w.
func WriteFramed(w io.Writer, v any) error {
	panic("not implemented: protocols.WriteFramed")
}

// ReadFramed reads a 4-byte big-endian length prefix from r, then reads that
// many bytes and JSON-decodes them into v. Frames larger than 16 MiB are
// rejected.
func ReadFramed(r io.Reader, v any) error {
	panic("not implemented: protocols.ReadFramed")
}
