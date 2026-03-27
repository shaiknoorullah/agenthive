package protocol

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// MessageType identifies the kind of message.
type MessageType string

const (
	MsgNotification   MessageType = "notification"
	MsgActionRequest  MessageType = "action_request"
	MsgActionResponse MessageType = "action_response"
	MsgConfigSync     MessageType = "config_sync"
	MsgHeartbeat      MessageType = "heartbeat"
	MsgPeerAnnounce   MessageType = "peer_announce"
)

// Valid returns true if the message type is one of the known types.
func (mt MessageType) Valid() bool {
	switch mt {
	case MsgNotification, MsgActionRequest, MsgActionResponse,
		MsgConfigSync, MsgHeartbeat, MsgPeerAnnounce:
		return true
	}
	return false
}

// Priority levels for notifications.
type Priority string

const (
	PriorityInfo     Priority = "info"
	PriorityWarning  Priority = "warning"
	PriorityCritical Priority = "critical"
)

// Valid returns true if the priority is one of the known levels.
func (p Priority) Valid() bool {
	switch p {
	case PriorityInfo, PriorityWarning, PriorityCritical:
		return true
	}
	return false
}

// Message is the envelope for all protocol messages.
type Message struct {
	ID        string      `json:"id"`
	Type      MessageType `json:"type"`
	SourceID  string      `json:"source_id"`
	TargetID  string      `json:"target_id,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   interface{} `json:"-"`
}

// NotificationPayload is sent when an agent produces a notification.
type NotificationPayload struct {
	Project  string   `json:"project"`
	Source   string   `json:"source"`
	Pane     string   `json:"pane,omitempty"`
	Session  string   `json:"session,omitempty"`
	Window   string   `json:"window,omitempty"`
	Message  string   `json:"message"`
	Priority Priority `json:"priority"`
}

// ActionRequestPayload is sent when an agent requests permission.
type ActionRequestPayload struct {
	RequestID string `json:"request_id"`
	Tool      string `json:"tool"`
	Command   string `json:"command"`
	Project   string `json:"project"`
	Source    string `json:"source"`
	Pane      string `json:"pane,omitempty"`
	TTL       int    `json:"ttl"`
}

// ActionResponsePayload carries the allow/deny decision.
type ActionResponsePayload struct {
	RequestID string `json:"request_id"`
	Decision  string `json:"decision"`
}

// ConfigSyncPayload carries CRDT delta state for synchronization.
type ConfigSyncPayload struct {
	Delta json.RawMessage `json:"delta"`
}

// HeartbeatPayload is sent periodically to report peer health.
type HeartbeatPayload struct {
	Uptime       int `json:"uptime"`
	AgentCount   int `json:"agent_count"`
	MessageCount int `json:"message_count"`
}

// PeerAnnouncePayload is sent when a peer joins or changes address.
type PeerAnnouncePayload struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	Addr      string `json:"addr,omitempty"`
	LinkType  string `json:"link_type,omitempty"`
}

// messageJSON is the wire format for JSON marshaling.
type messageJSON struct {
	ID        string          `json:"id"`
	Type      MessageType     `json:"type"`
	SourceID  string          `json:"source_id"`
	TargetID  string          `json:"target_id,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// MarshalJSON implements json.Marshaler.
//
//nolint:gocritic // value receiver required by json.Marshaler for non-pointer Message values
func (m Message) MarshalJSON() ([]byte, error) {
	payloadBytes, err := json.Marshal(m.Payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	return json.Marshal(messageJSON{
		ID:        m.ID,
		Type:      m.Type,
		SourceID:  m.SourceID,
		TargetID:  m.TargetID,
		Timestamp: m.Timestamp,
		Payload:   payloadBytes,
	})
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *Message) UnmarshalJSON(data []byte) error {
	var raw messageJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if !raw.Type.Valid() {
		return fmt.Errorf("unknown message type: %q", raw.Type)
	}

	m.ID = raw.ID
	m.Type = raw.Type
	m.SourceID = raw.SourceID
	m.TargetID = raw.TargetID
	m.Timestamp = raw.Timestamp

	switch raw.Type {
	case MsgNotification:
		var p NotificationPayload
		if err := json.Unmarshal(raw.Payload, &p); err != nil {
			return fmt.Errorf("unmarshal notification payload: %w", err)
		}
		m.Payload = p
	case MsgActionRequest:
		var p ActionRequestPayload
		if err := json.Unmarshal(raw.Payload, &p); err != nil {
			return fmt.Errorf("unmarshal action_request payload: %w", err)
		}
		m.Payload = p
	case MsgActionResponse:
		var p ActionResponsePayload
		if err := json.Unmarshal(raw.Payload, &p); err != nil {
			return fmt.Errorf("unmarshal action_response payload: %w", err)
		}
		m.Payload = p
	case MsgConfigSync:
		var p ConfigSyncPayload
		if err := json.Unmarshal(raw.Payload, &p); err != nil {
			return fmt.Errorf("unmarshal config_sync payload: %w", err)
		}
		m.Payload = p
	case MsgHeartbeat:
		var p HeartbeatPayload
		if err := json.Unmarshal(raw.Payload, &p); err != nil {
			return fmt.Errorf("unmarshal heartbeat payload: %w", err)
		}
		m.Payload = p
	case MsgPeerAnnounce:
		var p PeerAnnouncePayload
		if err := json.Unmarshal(raw.Payload, &p); err != nil {
			return fmt.Errorf("unmarshal peer_announce payload: %w", err)
		}
		m.Payload = p
	}

	return nil
}

// NewMessageID generates a unique message ID using crypto/rand.
func NewMessageID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}
