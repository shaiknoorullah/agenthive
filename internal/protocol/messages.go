package protocol

import (
	"encoding/json"
	"time"
)

// Suppress unused import warnings.
var _ = time.Now
var _ = json.Marshal

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

func (mt MessageType) Valid() bool { return false }

// Priority levels for notifications.
type Priority string

const (
	PriorityInfo     Priority = "info"
	PriorityWarning  Priority = "warning"
	PriorityCritical Priority = "critical"
)

func (p Priority) Valid() bool { return false }

// Message is the envelope for all protocol messages.
type Message struct {
	ID        string      `json:"id"`
	Type      MessageType `json:"type"`
	SourceID  string      `json:"source_id"`
	TargetID  string      `json:"target_id,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

type NotificationPayload struct {
	Project  string   `json:"project"`
	Source   string   `json:"source"`
	Pane     string   `json:"pane,omitempty"`
	Session  string   `json:"session,omitempty"`
	Window   string   `json:"window,omitempty"`
	Message  string   `json:"message"`
	Priority Priority `json:"priority"`
}

type ActionRequestPayload struct {
	RequestID string `json:"request_id"`
	Tool      string `json:"tool"`
	Command   string `json:"command"`
	Project   string `json:"project"`
	Source    string `json:"source"`
	Pane      string `json:"pane,omitempty"`
	TTL       int    `json:"ttl"`
}

type ActionResponsePayload struct {
	RequestID string `json:"request_id"`
	Decision  string `json:"decision"`
}

type ConfigSyncPayload struct {
	Delta json.RawMessage `json:"delta"`
}

type HeartbeatPayload struct {
	Uptime       int `json:"uptime"`
	AgentCount   int `json:"agent_count"`
	MessageCount int `json:"message_count"`
}

type PeerAnnouncePayload struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	Addr      string `json:"addr,omitempty"`
	LinkType  string `json:"link_type,omitempty"`
}

func NewMessageID() string { return "" }
