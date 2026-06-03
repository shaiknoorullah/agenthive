package protocols

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// MaxFrameSize is the upper bound on a framed body in bytes. Frames whose
// declared length exceeds this value are rejected by ReadFramed before any
// body bytes are read, so a malicious or buggy peer cannot exhaust memory.
const MaxFrameSize = 16 * 1024 * 1024 // 16 MiB

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
// followed by the JSON body to w. The combined write is performed as a single
// Write call when possible to avoid partial frames on stream-oriented writers.
//
// WriteFramed returns an error if the encoded body exceeds MaxFrameSize.
func WriteFramed(w io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("protocols: marshal frame body: %w", err)
	}
	if len(body) > MaxFrameSize {
		return fmt.Errorf("protocols: frame body %d exceeds max %d", len(body), MaxFrameSize)
	}

	// Build header + body in one buffer so a single Write hits the wire.
	frame := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(body)))
	copy(frame[4:], body)

	if _, err := w.Write(frame); err != nil {
		return fmt.Errorf("protocols: write frame: %w", err)
	}
	return nil
}

// ReadFramed reads a 4-byte big-endian length prefix from r, then reads exactly
// that many bytes and JSON-decodes them into v. Frames whose declared length
// exceeds MaxFrameSize are rejected before any body bytes are consumed.
//
// ReadFramed returns io.ErrUnexpectedEOF if the prefix or body is truncated.
func ReadFramed(r io.Reader, v any) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return fmt.Errorf("protocols: read frame header: %w", err)
	}
	length := binary.BigEndian.Uint32(header[:])
	if length > MaxFrameSize {
		return fmt.Errorf("protocols: frame body %d exceeds max %d", length, MaxFrameSize)
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return fmt.Errorf("protocols: read frame body: %w", err)
	}

	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("protocols: unmarshal frame body: %w", err)
	}
	return nil
}
