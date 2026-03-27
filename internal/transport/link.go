package transport

import (
	"encoding/json"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/protocol"
)

// Re-export protocol.MessageType for convenience within transport package.
// MsgPeerQuery and MsgPeerState are transport-specific extensions that must
// also be added to internal/protocol/messages.go when implementing Plan 2.
type MessageType = protocol.MessageType

// Re-export protocol message type constants for use in transport package.
const (
	MsgNotification   = protocol.MsgNotification
	MsgActionRequest  = protocol.MsgActionRequest
	MsgActionResponse = protocol.MsgActionResponse
	MsgConfigSync     = protocol.MsgConfigSync
	MsgHeartbeat      = protocol.MsgHeartbeat
	MsgPeerAnnounce   = protocol.MsgPeerAnnounce
)

// Transport-specific message types.
const (
	MsgPeerQuery MessageType = ""
	MsgPeerState MessageType = ""
)

// LinkStatus represents the current state of a link.
type LinkStatus string

const (
	StatusConnecting   LinkStatus = ""
	StatusConnected    LinkStatus = ""
	StatusDisconnected LinkStatus = ""
	StatusError        LinkStatus = ""
)

// Envelope is the framing wrapper for all messages sent over links.
// Serialized as a single JSON line (newline-delimited JSON).
type Envelope struct {
	Type      protocol.MessageType `json:"type"`
	ID        string               `json:"id"`
	From      string               `json:"from"`
	To        string               `json:"to,omitempty"`
	Timestamp time.Time            `json:"timestamp"`
	Payload   json.RawMessage      `json:"payload"`
}

// Link is the interface that all transport types implement.
type Link interface {
	// Send transmits an envelope to the remote peer.
	Send(env Envelope) error

	// Receive returns a channel that delivers inbound envelopes.
	Receive() <-chan Envelope

	// Close shuts down the link and releases resources.
	Close() error

	// Status returns the current link status.
	Status() LinkStatus

	// PeerID returns the identity of the remote peer.
	PeerID() string
}
