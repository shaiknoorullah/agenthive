# Core Daemon & Message Router Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the central agenthive daemon: CLI entry point, daemon lifecycle management, Unix socket listener for hook IPC, message type definitions, Ed25519 peer identity, message routing engine, and disk-backed message queue for offline peers.

**Architecture:** The daemon is the central process on every peer. It loads CRDT state from `~/.config/agenthive/`, listens on a Unix socket for hook IPC (hooks send notifications and action requests), routes inbound messages to appropriate handlers, routes outbound messages by evaluating routing rules from CRDT state, and queues messages to disk when destination peers are offline. The CLI provides subcommands for init, start, stop, status, peers, routes, config, and respond.

**Tech Stack:** Go 1.22+, `github.com/spf13/cobra` (CLI framework), `crypto/ed25519` (peer identity), `github.com/stretchr/testify` (assertions), `encoding/json` (serialization), `net` (Unix socket), `os/signal` (signal handling).

**Testing strategy:** TDD throughout. Write failing tests first, then implement. Race detector on all tests. Integration tests for daemon lifecycle use short-lived processes. Socket tests use temp directories.

**Dependency:** Subsystem 1 (CRDT State Store) must be built first. This plan imports `internal/crdt`.

---

## File Structure

```
agenthive/
  cmd/
    agenthive/
      main.go                       # CLI entry point with cobra root command
  internal/
    protocol/
      messages.go                   # Message type definitions
      messages_test.go              # Message serialization tests
    identity/
      identity.go                   # Ed25519 peer identity generation/management
      identity_test.go              # Identity tests
    daemon/
      daemon.go                     # Daemon lifecycle (start, stop, signal handling, PID file)
      daemon_test.go                # Daemon lifecycle tests
      socket.go                     # Unix socket listener for hook IPC
      socket_test.go                # Socket listener tests
      router.go                     # Message routing engine
      router_test.go                # Routing engine tests
      queue.go                      # Disk-backed message queue for offline peers
      queue_test.go                 # Queue tests
```

---

## Task 1: Add CLI Dependencies

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add cobra dependency**

```bash
cd /home/devsupreme/agenthive
go get github.com/spf13/cobra@latest
```

- [ ] **Step 2: Verify module**

Run: `cat go.mod`
Expected: Module path `github.com/shaiknoorullah/agenthive` with require entry for cobra.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "feat: add cobra CLI dependency"
```

---

## Task 2: Message Types -- Failing Tests

**Files:**
- Create: `internal/protocol/messages.go`
- Create: `internal/protocol/messages_test.go`

The protocol defines six message types: notification, action_request, action_response, config_sync, heartbeat, and peer_announce. All messages share a common envelope with type, source peer, destination peer, timestamp, and ID.

- [ ] **Step 1: Write the failing tests**

Create `internal/protocol/messages_test.go`:

```go
package protocol

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessage_JSONRoundTrip_Notification(t *testing.T) {
	msg := Message{
		ID:        "msg-001",
		Type:      MsgNotification,
		SourceID:  "peer-server",
		TargetID:  "peer-laptop",
		Timestamp: time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC),
		Payload: NotificationPayload{
			Project:  "api-server",
			Source:   "Claude",
			Pane:     "%42",
			Session:  "main",
			Window:   "dev",
			Message:  "Task completed successfully",
			Priority: PriorityInfo,
		},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded Message
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, msg.ID, decoded.ID)
	assert.Equal(t, MsgNotification, decoded.Type)
	assert.Equal(t, msg.SourceID, decoded.SourceID)
	assert.Equal(t, msg.TargetID, decoded.TargetID)

	payload, ok := decoded.Payload.(NotificationPayload)
	require.True(t, ok, "payload should be NotificationPayload")
	assert.Equal(t, "api-server", payload.Project)
	assert.Equal(t, "Claude", payload.Source)
	assert.Equal(t, "Task completed successfully", payload.Message)
	assert.Equal(t, PriorityInfo, payload.Priority)
}

func TestMessage_JSONRoundTrip_ActionRequest(t *testing.T) {
	msg := Message{
		ID:        "msg-002",
		Type:      MsgActionRequest,
		SourceID:  "peer-server",
		TargetID:  "peer-phone",
		Timestamp: time.Date(2026, 3, 26, 14, 31, 0, 0, time.UTC),
		Payload: ActionRequestPayload{
			RequestID: "req-42",
			Tool:      "Bash",
			Command:   "rm -rf /tmp/build",
			Project:   "api-server",
			Source:    "Claude",
			Pane:      "%42",
			TTL:       300,
		},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded Message
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	payload, ok := decoded.Payload.(ActionRequestPayload)
	require.True(t, ok)
	assert.Equal(t, "req-42", payload.RequestID)
	assert.Equal(t, "Bash", payload.Tool)
	assert.Equal(t, "rm -rf /tmp/build", payload.Command)
	assert.Equal(t, 300, payload.TTL)
}

func TestMessage_JSONRoundTrip_ActionResponse(t *testing.T) {
	msg := Message{
		ID:        "msg-003",
		Type:      MsgActionResponse,
		SourceID:  "peer-phone",
		TargetID:  "peer-server",
		Timestamp: time.Date(2026, 3, 26, 14, 32, 0, 0, time.UTC),
		Payload: ActionResponsePayload{
			RequestID: "req-42",
			Decision:  "allow",
		},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded Message
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	payload, ok := decoded.Payload.(ActionResponsePayload)
	require.True(t, ok)
	assert.Equal(t, "req-42", payload.RequestID)
	assert.Equal(t, "allow", payload.Decision)
}

func TestMessage_JSONRoundTrip_ConfigSync(t *testing.T) {
	msg := Message{
		ID:        "msg-004",
		Type:      MsgConfigSync,
		SourceID:  "peer-phone",
		TargetID:  "",
		Timestamp: time.Date(2026, 3, 26, 14, 33, 0, 0, time.UTC),
		Payload: ConfigSyncPayload{
			Delta: json.RawMessage(`{"routes":{"r1":{"value":{"match":{"project":"api"},"targets":["phone"]}}}}`),
		},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded Message
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	payload, ok := decoded.Payload.(ConfigSyncPayload)
	require.True(t, ok)
	assert.NotEmpty(t, payload.Delta)
}

func TestMessage_JSONRoundTrip_Heartbeat(t *testing.T) {
	msg := Message{
		ID:        "msg-005",
		Type:      MsgHeartbeat,
		SourceID:  "peer-server",
		TargetID:  "",
		Timestamp: time.Date(2026, 3, 26, 14, 34, 0, 0, time.UTC),
		Payload: HeartbeatPayload{
			Uptime:       3600,
			AgentCount:   5,
			MessageCount: 43,
		},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded Message
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	payload, ok := decoded.Payload.(HeartbeatPayload)
	require.True(t, ok)
	assert.Equal(t, 3600, payload.Uptime)
	assert.Equal(t, 5, payload.AgentCount)
}

func TestMessage_JSONRoundTrip_PeerAnnounce(t *testing.T) {
	msg := Message{
		ID:        "msg-006",
		Type:      MsgPeerAnnounce,
		SourceID:  "peer-new",
		TargetID:  "",
		Timestamp: time.Date(2026, 3, 26, 14, 35, 0, 0, time.UTC),
		Payload: PeerAnnouncePayload{
			Name:      "dev-server",
			PublicKey: "base64encodedkey==",
			Addr:      "10.0.0.1:19222",
			LinkType:  "ssh",
		},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded Message
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	payload, ok := decoded.Payload.(PeerAnnouncePayload)
	require.True(t, ok)
	assert.Equal(t, "dev-server", payload.Name)
	assert.Equal(t, "base64encodedkey==", payload.PublicKey)
}

func TestMessage_UnmarshalInvalidType_Error(t *testing.T) {
	data := []byte(`{"id":"x","type":"bogus","source_id":"a","payload":{}}`)
	var msg Message
	err := json.Unmarshal(data, &msg)
	assert.Error(t, err)
}

func TestMessage_UnmarshalInvalidJSON_Error(t *testing.T) {
	var msg Message
	err := json.Unmarshal([]byte(`not json`), &msg)
	assert.Error(t, err)
}

func TestPriority_Validation(t *testing.T) {
	assert.True(t, PriorityInfo.Valid())
	assert.True(t, PriorityWarning.Valid())
	assert.True(t, PriorityCritical.Valid())
	assert.False(t, Priority("bogus").Valid())
}

func TestMessageType_Validation(t *testing.T) {
	assert.True(t, MsgNotification.Valid())
	assert.True(t, MsgActionRequest.Valid())
	assert.True(t, MsgActionResponse.Valid())
	assert.True(t, MsgConfigSync.Valid())
	assert.True(t, MsgHeartbeat.Valid())
	assert.True(t, MsgPeerAnnounce.Valid())
	assert.False(t, MessageType("bogus").Valid())
}

func TestNewMessageID_Unique(t *testing.T) {
	id1 := NewMessageID()
	id2 := NewMessageID()
	assert.NotEqual(t, id1, id2)
	assert.NotEmpty(t, id1)
}
```

- [ ] **Step 2: Create minimal type stubs so it compiles**

Create `internal/protocol/messages.go`:

```go
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
	MsgNotification  MessageType = "notification"
	MsgActionRequest MessageType = "action_request"
	MsgActionResponse MessageType = "action_response"
	MsgConfigSync    MessageType = "config_sync"
	MsgHeartbeat     MessageType = "heartbeat"
	MsgPeerAnnounce  MessageType = "peer_announce"
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
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/protocol/ -run Test -v -count=1 2>&1 | head -40`
Expected: All tests FAIL (stubs return zero values, custom unmarshal not implemented).

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/protocol/messages.go internal/protocol/messages_test.go
git commit -m "test: add failing tests for protocol message types"
```

---

## Task 3: Message Types -- Implementation

**Files:**
- Modify: `internal/protocol/messages.go`

- [ ] **Step 1: Implement message types with custom JSON marshaling**

Replace `internal/protocol/messages.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/protocol/ -v -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/protocol/messages.go
git commit -m "feat: implement protocol message types with typed JSON marshaling"
```

---

## Task 4: Peer Identity -- Failing Tests

**Files:**
- Create: `internal/identity/identity.go`
- Create: `internal/identity/identity_test.go`

Ed25519 peer identity: each peer generates a key pair on `agenthive init`. The public key serves as the peer's identity. Keys are stored at `~/.config/agenthive/identity.json`.

- [ ] **Step 1: Write the failing tests**

Create `internal/identity/identity_test.go`:

```go
package identity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate_CreatesValidIdentity(t *testing.T) {
	id, err := Generate("dev-server")
	require.NoError(t, err)

	assert.Equal(t, "dev-server", id.Name)
	assert.NotEmpty(t, id.PeerID)
	assert.NotEmpty(t, id.PublicKey)
	assert.NotEmpty(t, id.PrivateKey)
}

func TestGenerate_PeerIDDerivedFromPublicKey(t *testing.T) {
	id, err := Generate("test")
	require.NoError(t, err)

	// PeerID should be deterministic from the public key
	expected := PeerIDFromPublicKey(id.PublicKey)
	assert.Equal(t, expected, id.PeerID)
}

func TestGenerate_UniqueKeysEachTime(t *testing.T) {
	id1, err := Generate("test")
	require.NoError(t, err)

	id2, err := Generate("test")
	require.NoError(t, err)

	assert.NotEqual(t, id1.PeerID, id2.PeerID)
	assert.NotEqual(t, id1.PublicKey, id2.PublicKey)
	assert.NotEqual(t, id1.PrivateKey, id2.PrivateKey)
}

func TestSign_ProducesValidSignature(t *testing.T) {
	id, err := Generate("test")
	require.NoError(t, err)

	message := []byte("hello world")
	sig, err := id.Sign(message)
	require.NoError(t, err)
	assert.NotEmpty(t, sig)

	valid := Verify(id.PublicKey, message, sig)
	assert.True(t, valid)
}

func TestVerify_RejectsWrongMessage(t *testing.T) {
	id, err := Generate("test")
	require.NoError(t, err)

	sig, err := id.Sign([]byte("hello"))
	require.NoError(t, err)

	valid := Verify(id.PublicKey, []byte("wrong"), sig)
	assert.False(t, valid)
}

func TestVerify_RejectsWrongKey(t *testing.T) {
	id1, _ := Generate("test1")
	id2, _ := Generate("test2")

	sig, _ := id1.Sign([]byte("hello"))

	valid := Verify(id2.PublicKey, []byte("hello"), sig)
	assert.False(t, valid)
}

func TestIdentity_JSONRoundTrip(t *testing.T) {
	id, err := Generate("test-peer")
	require.NoError(t, err)

	data, err := json.Marshal(id)
	require.NoError(t, err)

	var id2 Identity
	err = json.Unmarshal(data, &id2)
	require.NoError(t, err)

	assert.Equal(t, id.Name, id2.Name)
	assert.Equal(t, id.PeerID, id2.PeerID)
	assert.Equal(t, id.PublicKey, id2.PublicKey)
	assert.Equal(t, id.PrivateKey, id2.PrivateKey)
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.json")

	id, err := Generate("save-test")
	require.NoError(t, err)

	err = id.SaveToFile(path)
	require.NoError(t, err)

	// Verify file permissions are restrictive
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	loaded, err := LoadFromFile(path)
	require.NoError(t, err)

	assert.Equal(t, id.Name, loaded.Name)
	assert.Equal(t, id.PeerID, loaded.PeerID)
	assert.Equal(t, id.PublicKey, loaded.PublicKey)
	assert.Equal(t, id.PrivateKey, loaded.PrivateKey)

	// Verify the loaded identity can sign
	sig, err := loaded.Sign([]byte("test"))
	require.NoError(t, err)
	assert.True(t, Verify(loaded.PublicKey, []byte("test"), sig))
}

func TestLoadFromFile_NotExist(t *testing.T) {
	_, err := LoadFromFile("/tmp/nonexistent-agenthive-identity.json")
	assert.Error(t, err)
}

func TestPeerIDFromPublicKey_Deterministic(t *testing.T) {
	id, _ := Generate("test")
	pid1 := PeerIDFromPublicKey(id.PublicKey)
	pid2 := PeerIDFromPublicKey(id.PublicKey)
	assert.Equal(t, pid1, pid2)
}
```

- [ ] **Step 2: Create minimal type stubs**

Create `internal/identity/identity.go`:

```go
package identity

// Identity holds an Ed25519 key pair and peer metadata.
type Identity struct {
	Name       string `json:"name"`
	PeerID     string `json:"peer_id"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

func Generate(name string) (*Identity, error)                    { return nil, nil }
func (id *Identity) Sign(message []byte) ([]byte, error)        { return nil, nil }
func Verify(publicKey string, message []byte, sig []byte) bool   { return false }
func PeerIDFromPublicKey(publicKey string) string                { return "" }
func (id *Identity) SaveToFile(path string) error               { return nil }
func LoadFromFile(path string) (*Identity, error)                { return nil, nil }
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/identity/ -v -count=1 2>&1 | head -30`
Expected: All tests FAIL.

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/identity/identity.go internal/identity/identity_test.go
git commit -m "test: add failing tests for Ed25519 peer identity"
```

---

## Task 5: Peer Identity -- Implementation

**Files:**
- Modify: `internal/identity/identity.go`

- [ ] **Step 1: Implement Ed25519 identity**

Replace `internal/identity/identity.go`:

```go
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Identity holds an Ed25519 key pair and peer metadata.
type Identity struct {
	Name       string `json:"name"`
	PeerID     string `json:"peer_id"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

// Generate creates a new Ed25519 identity with the given name.
func Generate(name string) (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}

	pubB64 := base64.StdEncoding.EncodeToString(pub)
	privB64 := base64.StdEncoding.EncodeToString(priv)

	return &Identity{
		Name:       name,
		PeerID:     PeerIDFromPublicKey(pubB64),
		PublicKey:  pubB64,
		PrivateKey: privB64,
	}, nil
}

// PeerIDFromPublicKey derives a deterministic peer ID from a base64-encoded
// Ed25519 public key. The ID is the first 16 hex characters of the SHA-256
// hash of the raw public key bytes.
func PeerIDFromPublicKey(publicKey string) string {
	raw, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:8])
}

// Sign signs a message with the identity's private key.
func (id *Identity) Sign(message []byte) ([]byte, error) {
	privBytes, err := base64.StdEncoding.DecodeString(id.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	priv := ed25519.PrivateKey(privBytes)
	return ed25519.Sign(priv, message), nil
}

// Verify checks a signature against a base64-encoded public key.
func Verify(publicKey string, message []byte, sig []byte) bool {
	pubBytes, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil {
		return false
	}
	pub := ed25519.PublicKey(pubBytes)
	return ed25519.Verify(pub, message, sig)
}

// SaveToFile writes the identity to a JSON file with 0600 permissions.
func (id *Identity) SaveToFile(path string) error {
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal identity: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write identity: %w", err)
	}
	return os.Rename(tmp, path)
}

// LoadFromFile reads an identity from a JSON file.
func LoadFromFile(path string) (*Identity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read identity file: %w", err)
	}
	var id Identity
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, fmt.Errorf("unmarshal identity: %w", err)
	}
	return &id, nil
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/identity/ -v -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/identity/identity.go
git commit -m "feat: implement Ed25519 peer identity with sign/verify and persistence"
```

---

## Task 6: Message Router -- Failing Tests

**Files:**
- Create: `internal/daemon/router.go`
- Create: `internal/daemon/router_test.go`

The router evaluates routing rules from the CRDT state store against message metadata (project, source, session, priority, pane) and returns matching target peer IDs. Rules are evaluated in order; a message may match multiple rules. A "default" rule (empty match) catches unmatched messages. An `ALL` target means all known peers.

- [ ] **Step 1: Write the failing tests**

Create `internal/daemon/router_test.go`:

```go
package daemon

import (
	"testing"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(peerID string) *crdt.StateStore {
	return crdt.NewStateStore(peerID)
}

func TestRouter_MatchesByProject(t *testing.T) {
	store := newTestStore("peer-a")
	store.SetRoute("r1", crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api-server"},
		Targets: []string{"phone", "laptop"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "api-server",
			Source:  "Claude",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	assert.ElementsMatch(t, []string{"phone", "laptop"}, targets)
}

func TestRouter_MatchesBySource(t *testing.T) {
	store := newTestStore("peer-a")
	store.SetRoute("r1", crdt.RouteRule{
		Match:   crdt.RouteMatch{Source: "Codex"},
		Targets: []string{"desktop-only"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "any-project",
			Source:  "Codex",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	assert.ElementsMatch(t, []string{"desktop-only"}, targets)
}

func TestRouter_MatchesByPriority(t *testing.T) {
	store := newTestStore("peer-a")
	store.SetRoute("r1", crdt.RouteRule{
		Match:   crdt.RouteMatch{Priority: "critical"},
		Targets: []string{"ALL"},
	})
	// Register known peers for ALL expansion
	store.SetPeer("phone", crdt.PeerInfo{Name: "phone", Status: "online"})
	store.SetPeer("laptop", crdt.PeerInfo{Name: "laptop", Status: "online"})
	store.SetPeer("peer-a", crdt.PeerInfo{Name: "peer-a", Status: "online"})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project:  "api",
			Source:   "Claude",
			Message:  "FAILED",
			Priority: protocol.PriorityCritical,
		},
	}

	targets := router.Route(msg)
	// ALL expands to all known peers except self
	assert.ElementsMatch(t, []string{"phone", "laptop"}, targets)
}

func TestRouter_MatchesBySession(t *testing.T) {
	store := newTestStore("peer-a")
	store.SetRoute("r1", crdt.RouteRule{
		Match:   crdt.RouteMatch{Session: "refactor"},
		Targets: []string{"telegram"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Session: "refactor",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	assert.ElementsMatch(t, []string{"telegram"}, targets)
}

func TestRouter_MultipleRulesMatch(t *testing.T) {
	store := newTestStore("peer-a")
	store.SetRoute("r1", crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api"},
		Targets: []string{"phone"},
	})
	store.SetRoute("r2", crdt.RouteRule{
		Match:   crdt.RouteMatch{Priority: "critical"},
		Targets: []string{"laptop"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project:  "api",
			Source:   "Claude",
			Message:  "FAILED",
			Priority: protocol.PriorityCritical,
		},
	}

	targets := router.Route(msg)
	// Both rules match, targets are deduplicated
	assert.ElementsMatch(t, []string{"phone", "laptop"}, targets)
}

func TestRouter_DefaultRuleCatchesUnmatched(t *testing.T) {
	store := newTestStore("peer-a")
	store.SetRoute("r1", crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api"},
		Targets: []string{"phone"},
	})
	store.SetRoute("default", crdt.RouteRule{
		Match:   crdt.RouteMatch{}, // empty match = default
		Targets: []string{"laptop"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "frontend",
			Source:  "Claude",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	assert.ElementsMatch(t, []string{"laptop"}, targets)
}

func TestRouter_NoRulesMatch_ReturnsEmpty(t *testing.T) {
	store := newTestStore("peer-a")
	store.SetRoute("r1", crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api"},
		Targets: []string{"phone"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "frontend",
			Source:  "Claude",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	assert.Empty(t, targets)
}

func TestRouter_MultiFieldMatch_AllFieldsMustMatch(t *testing.T) {
	store := newTestStore("peer-a")
	store.SetRoute("r1", crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api", Source: "Claude"},
		Targets: []string{"phone"},
	})

	router := NewRouter(store, "peer-a")

	// Message with matching project but wrong source
	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Codex",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	assert.Empty(t, targets, "should not match when source differs")
}

func TestRouter_ActionRequest_RoutedByProject(t *testing.T) {
	store := newTestStore("peer-a")
	store.SetRoute("r1", crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api"},
		Targets: []string{"phone"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgActionRequest,
		SourceID: "peer-a",
		Payload: protocol.ActionRequestPayload{
			RequestID: "req-1",
			Tool:      "Bash",
			Command:   "rm -rf /tmp",
			Project:   "api",
			Source:    "Claude",
		},
	}

	targets := router.Route(msg)
	assert.ElementsMatch(t, []string{"phone"}, targets)
}

func TestRouter_DuplicateTargetsDeduped(t *testing.T) {
	store := newTestStore("peer-a")
	store.SetRoute("r1", crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api"},
		Targets: []string{"phone", "laptop"},
	})
	store.SetRoute("r2", crdt.RouteRule{
		Match:   crdt.RouteMatch{Source: "Claude"},
		Targets: []string{"phone"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	// "phone" appears in both rules but should only appear once
	require.Len(t, targets, 2)
	assert.ElementsMatch(t, []string{"phone", "laptop"}, targets)
}

func TestRouter_ExcludesSelf(t *testing.T) {
	store := newTestStore("peer-a")
	store.SetRoute("r1", crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api"},
		Targets: []string{"peer-a", "phone"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	assert.ElementsMatch(t, []string{"phone"}, targets)
}
```

- [ ] **Step 2: Create minimal stubs**

Create `internal/daemon/router.go`:

```go
package daemon

import (
	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/protocol"
)

// Suppress unused import warnings.
var _ = (*crdt.StateStore)(nil)
var _ = protocol.MsgNotification

// Router evaluates routing rules against message metadata
// and returns matching target peer IDs.
type Router struct{}

func NewRouter(store *crdt.StateStore, selfID string) *Router { return nil }
func (r *Router) Route(msg protocol.Message) []string         { return nil }
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/daemon/ -run TestRouter -v -count=1 2>&1 | head -30`
Expected: All tests FAIL.

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/daemon/router.go internal/daemon/router_test.go
git commit -m "test: add failing tests for message routing engine"
```

---

## Task 7: Message Router -- Implementation

**Files:**
- Modify: `internal/daemon/router.go`

- [ ] **Step 1: Implement the message router**

Replace `internal/daemon/router.go`:

```go
package daemon

import (
	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/protocol"
)

// Router evaluates routing rules from the CRDT state store against
// message metadata and returns matching target peer IDs.
// Rules are evaluated independently; a message may match multiple rules.
// An empty match field is a wildcard (matches anything).
// A fully empty match (all fields empty) is the "default" rule.
// "ALL" as a target expands to all known peers except self.
type Router struct {
	store  *crdt.StateStore
	selfID string
}

// NewRouter creates a new message router.
func NewRouter(store *crdt.StateStore, selfID string) *Router {
	return &Router{store: store, selfID: selfID}
}

// Route evaluates all routing rules and returns deduplicated target peer IDs.
// Returns an empty slice if no rules match.
func (r *Router) Route(msg protocol.Message) []string {
	meta := extractMetadata(msg)
	rules := r.store.ListRoutes()

	seen := make(map[string]bool)
	var targets []string
	matched := false

	for id, rule := range rules {
		if id == "default" {
			continue // evaluate default rule only if nothing else matches
		}
		if matchesRule(rule.Match, meta) {
			matched = true
			for _, target := range r.expandTargets(rule.Targets) {
				if !seen[target] {
					seen[target] = true
					targets = append(targets, target)
				}
			}
		}
	}

	// If no specific rules matched, try the default rule
	if !matched {
		if defaultRule, ok := rules["default"]; ok {
			for _, target := range r.expandTargets(defaultRule.Targets) {
				if !seen[target] {
					seen[target] = true
					targets = append(targets, target)
				}
			}
		}
	}

	return targets
}

// messageMetadata holds extracted routing-relevant fields from a message.
type messageMetadata struct {
	Project  string
	Source   string
	Session  string
	Window   string
	Pane     string
	Priority string
}

// extractMetadata pulls routing-relevant fields from a message payload.
func extractMetadata(msg protocol.Message) messageMetadata {
	switch p := msg.Payload.(type) {
	case protocol.NotificationPayload:
		return messageMetadata{
			Project:  p.Project,
			Source:   p.Source,
			Session:  p.Session,
			Window:   p.Window,
			Pane:     p.Pane,
			Priority: string(p.Priority),
		}
	case protocol.ActionRequestPayload:
		return messageMetadata{
			Project: p.Project,
			Source:  p.Source,
			Pane:    p.Pane,
		}
	default:
		return messageMetadata{}
	}
}

// matchesRule checks if a message's metadata matches a routing rule.
// Empty fields in the rule match are wildcards.
// All non-empty fields must match for the rule to apply.
func matchesRule(match crdt.RouteMatch, meta messageMetadata) bool {
	if match.Project != "" && match.Project != meta.Project {
		return false
	}
	if match.Source != "" && match.Source != meta.Source {
		return false
	}
	if match.Session != "" && match.Session != meta.Session {
		return false
	}
	if match.Window != "" && match.Window != meta.Window {
		return false
	}
	if match.Pane != "" && match.Pane != meta.Pane {
		return false
	}
	if match.Priority != "" && match.Priority != meta.Priority {
		return false
	}
	return true
}

// expandTargets expands "ALL" to all known peers (except self) and
// filters out self from explicit targets.
func (r *Router) expandTargets(targets []string) []string {
	var result []string
	for _, t := range targets {
		if t == "ALL" {
			peers := r.store.ListPeers()
			for id := range peers {
				if id != r.selfID {
					result = append(result, id)
				}
			}
		} else if t != r.selfID {
			result = append(result, t)
		}
	}
	return result
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/daemon/ -run TestRouter -v -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/router.go
git commit -m "feat: implement message routing engine with rule matching and ALL expansion"
```

---

## Task 8: Disk-Backed Message Queue -- Failing Tests

**Files:**
- Create: `internal/daemon/queue.go`
- Create: `internal/daemon/queue_test.go`

The queue stores messages to disk when destination peers are offline. Messages are stored as newline-delimited JSON files, one file per target peer. On reconnection, queued messages are drained and sent.

- [ ] **Step 1: Write the failing tests**

Create `internal/daemon/queue_test.go`:

```go
package daemon

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueue_EnqueueAndDrain(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(filepath.Join(dir, "queue"))
	require.NoError(t, err)

	msg := protocol.Message{
		ID:        "msg-1",
		Type:      protocol.MsgNotification,
		SourceID:  "peer-a",
		TargetID:  "peer-b",
		Timestamp: time.Now(),
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "Done",
		},
	}

	err = q.Enqueue("peer-b", msg)
	require.NoError(t, err)

	msgs, err := q.Drain("peer-b")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "msg-1", msgs[0].ID)
}

func TestQueue_DrainEmpty(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(filepath.Join(dir, "queue"))
	require.NoError(t, err)

	msgs, err := q.Drain("nonexistent")
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestQueue_MultipleMessages(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(filepath.Join(dir, "queue"))
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		msg := protocol.Message{
			ID:        protocol.NewMessageID(),
			Type:      protocol.MsgNotification,
			SourceID:  "peer-a",
			TargetID:  "peer-b",
			Timestamp: time.Now(),
			Payload: protocol.NotificationPayload{
				Project: "api",
				Source:  "Claude",
				Message: "msg",
			},
		}
		err = q.Enqueue("peer-b", msg)
		require.NoError(t, err)
	}

	msgs, err := q.Drain("peer-b")
	require.NoError(t, err)
	assert.Len(t, msgs, 5)
}

func TestQueue_DrainClearsQueue(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(filepath.Join(dir, "queue"))
	require.NoError(t, err)

	msg := protocol.Message{
		ID:        "msg-1",
		Type:      protocol.MsgNotification,
		SourceID:  "peer-a",
		TargetID:  "peer-b",
		Timestamp: time.Now(),
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "Done",
		},
	}

	err = q.Enqueue("peer-b", msg)
	require.NoError(t, err)

	// First drain returns the message
	msgs, err := q.Drain("peer-b")
	require.NoError(t, err)
	assert.Len(t, msgs, 1)

	// Second drain returns empty
	msgs, err = q.Drain("peer-b")
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestQueue_MultiplePeers(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(filepath.Join(dir, "queue"))
	require.NoError(t, err)

	msgB := protocol.Message{
		ID:       "msg-b",
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		TargetID: "peer-b",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "for-b",
		},
	}
	msgC := protocol.Message{
		ID:       "msg-c",
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		TargetID: "peer-c",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "for-c",
		},
	}

	require.NoError(t, q.Enqueue("peer-b", msgB))
	require.NoError(t, q.Enqueue("peer-c", msgC))

	msgsB, err := q.Drain("peer-b")
	require.NoError(t, err)
	assert.Len(t, msgsB, 1)
	assert.Equal(t, "msg-b", msgsB[0].ID)

	msgsC, err := q.Drain("peer-c")
	require.NoError(t, err)
	assert.Len(t, msgsC, 1)
	assert.Equal(t, "msg-c", msgsC[0].ID)
}

func TestQueue_Depth(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(filepath.Join(dir, "queue"))
	require.NoError(t, err)

	assert.Equal(t, 0, q.Depth("peer-b"))

	for i := 0; i < 3; i++ {
		msg := protocol.Message{
			ID:       protocol.NewMessageID(),
			Type:     protocol.MsgNotification,
			SourceID: "peer-a",
			Payload: protocol.NotificationPayload{
				Project: "api",
				Source:  "Claude",
				Message: "msg",
			},
		}
		require.NoError(t, q.Enqueue("peer-b", msg))
	}

	assert.Equal(t, 3, q.Depth("peer-b"))
}

func TestQueue_SurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	queueDir := filepath.Join(dir, "queue")

	// Enqueue with first instance
	q1, err := NewQueue(queueDir)
	require.NoError(t, err)

	msg := protocol.Message{
		ID:       "persist-1",
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "persistent",
		},
	}
	require.NoError(t, q1.Enqueue("peer-b", msg))

	// Create second instance (simulates daemon restart)
	q2, err := NewQueue(queueDir)
	require.NoError(t, err)

	msgs, err := q2.Drain("peer-b")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "persist-1", msgs[0].ID)
}
```

- [ ] **Step 2: Create minimal stubs**

Create `internal/daemon/queue.go`:

```go
package daemon

import (
	"github.com/shaiknoorullah/agenthive/internal/protocol"
)

// Suppress unused import warning.
var _ = protocol.MsgNotification

// Queue is a disk-backed message queue for offline peers.
type Queue struct{}

func NewQueue(dir string) (*Queue, error)                       { return nil, nil }
func (q *Queue) Enqueue(peerID string, msg protocol.Message) error { return nil }
func (q *Queue) Drain(peerID string) ([]protocol.Message, error)  { return nil, nil }
func (q *Queue) Depth(peerID string) int                          { return 0 }
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/daemon/ -run TestQueue -v -count=1 2>&1 | head -30`
Expected: All tests FAIL.

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/daemon/queue.go internal/daemon/queue_test.go
git commit -m "test: add failing tests for disk-backed message queue"
```

---

## Task 9: Disk-Backed Message Queue -- Implementation

**Files:**
- Modify: `internal/daemon/queue.go`

- [ ] **Step 1: Implement the disk-backed queue**

Replace `internal/daemon/queue.go`:

```go
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
	defer f.Close()

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
	defer f.Close()

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
	f.Close()
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
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if len(scanner.Bytes()) > 0 {
			count++
		}
	}
	return count
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/daemon/ -run TestQueue -v -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/queue.go
git commit -m "feat: implement disk-backed NDJSON message queue for offline peers"
```

---

## Task 10: Unix Socket Listener -- Failing Tests

**Files:**
- Create: `internal/daemon/socket.go`
- Create: `internal/daemon/socket_test.go`

The socket listener accepts connections on a Unix domain socket at `~/.config/agenthive/daemon.sock`. Hooks connect and send newline-delimited JSON messages. The listener reads each message, parses it, and passes it to a handler callback.

- [ ] **Step 1: Write the failing tests**

Create `internal/daemon/socket_test.go`:

```go
package daemon

import (
	"encoding/json"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSocketListener_AcceptsConnection(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	var received []protocol.Message
	var mu sync.Mutex

	handler := func(msg protocol.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	listener, err := NewSocketListener(sockPath, handler)
	require.NoError(t, err)

	go listener.Serve()
	defer listener.Close()

	// Wait for listener to be ready
	time.Sleep(50 * time.Millisecond)

	// Connect and send a message
	conn, err := net.Dial("unix", sockPath)
	require.NoError(t, err)
	defer conn.Close()

	msg := protocol.Message{
		ID:       "test-1",
		Type:     protocol.MsgNotification,
		SourceID: "hook",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "Done",
		},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)
	_, err = conn.Write(append(data, '\n'))
	require.NoError(t, err)

	// Wait for message processing
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, received, 1)
	assert.Equal(t, "test-1", received[0].ID)
}

func TestSocketListener_MultipleMessages(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	var received []protocol.Message
	var mu sync.Mutex

	handler := func(msg protocol.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	listener, err := NewSocketListener(sockPath, handler)
	require.NoError(t, err)

	go listener.Serve()
	defer listener.Close()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", sockPath)
	require.NoError(t, err)
	defer conn.Close()

	for i := 0; i < 3; i++ {
		msg := protocol.Message{
			ID:       protocol.NewMessageID(),
			Type:     protocol.MsgNotification,
			SourceID: "hook",
			Payload: protocol.NotificationPayload{
				Project: "api",
				Source:  "Claude",
				Message: "msg",
			},
		}
		data, _ := json.Marshal(msg)
		_, err = conn.Write(append(data, '\n'))
		require.NoError(t, err)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, received, 3)
}

func TestSocketListener_MultipleConnections(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	var received []protocol.Message
	var mu sync.Mutex

	handler := func(msg protocol.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	listener, err := NewSocketListener(sockPath, handler)
	require.NoError(t, err)

	go listener.Serve()
	defer listener.Close()

	time.Sleep(50 * time.Millisecond)

	// Two separate connections
	for i := 0; i < 2; i++ {
		conn, err := net.Dial("unix", sockPath)
		require.NoError(t, err)

		msg := protocol.Message{
			ID:       protocol.NewMessageID(),
			Type:     protocol.MsgNotification,
			SourceID: "hook",
			Payload: protocol.NotificationPayload{
				Project: "api",
				Source:  "Claude",
				Message: "msg",
			},
		}
		data, _ := json.Marshal(msg)
		_, err = conn.Write(append(data, '\n'))
		require.NoError(t, err)
		conn.Close()
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, received, 2)
}

func TestSocketListener_Close_StopsAccepting(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	handler := func(msg protocol.Message) {}

	listener, err := NewSocketListener(sockPath, handler)
	require.NoError(t, err)

	go listener.Serve()

	time.Sleep(50 * time.Millisecond)

	// Close the listener
	listener.Close()

	time.Sleep(50 * time.Millisecond)

	// Connections should be refused
	_, err = net.Dial("unix", sockPath)
	assert.Error(t, err)
}

func TestSocketListener_InvalidJSON_DoesNotCrash(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	var received []protocol.Message
	var mu sync.Mutex

	handler := func(msg protocol.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	listener, err := NewSocketListener(sockPath, handler)
	require.NoError(t, err)

	go listener.Serve()
	defer listener.Close()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", sockPath)
	require.NoError(t, err)
	defer conn.Close()

	// Send invalid JSON followed by valid message
	_, err = conn.Write([]byte("not valid json\n"))
	require.NoError(t, err)

	msg := protocol.Message{
		ID:       "valid-1",
		Type:     protocol.MsgNotification,
		SourceID: "hook",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "Done",
		},
	}
	data, _ := json.Marshal(msg)
	_, err = conn.Write(append(data, '\n'))
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, received, 1)
	assert.Equal(t, "valid-1", received[0].ID)
}

func TestSocketListener_SocketPath(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	handler := func(msg protocol.Message) {}

	listener, err := NewSocketListener(sockPath, handler)
	require.NoError(t, err)
	defer listener.Close()

	assert.Equal(t, sockPath, listener.Path())
}
```

- [ ] **Step 2: Create minimal stubs**

Create `internal/daemon/socket.go`:

```go
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
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/daemon/ -run TestSocketListener -v -count=1 2>&1 | head -30`
Expected: All tests FAIL.

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/daemon/socket.go internal/daemon/socket_test.go
git commit -m "test: add failing tests for Unix socket listener"
```

---

## Task 11: Unix Socket Listener -- Implementation

**Files:**
- Modify: `internal/daemon/socket.go`

- [ ] **Step 1: Implement the socket listener**

Replace `internal/daemon/socket.go`:

```go
package daemon

import (
	"bufio"
	"encoding/json"
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
	os.Remove(path)

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}

	// Set socket permissions to owner-only
	os.Chmod(path, 0600)

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
	defer conn.Close()

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
	os.Remove(s.path)
	return err
}

// Path returns the socket file path.
func (s *SocketListener) Path() string {
	return s.path
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/daemon/ -run TestSocketListener -v -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/daemon/socket.go
git commit -m "feat: implement Unix socket listener for hook IPC"
```

---

## Task 12: Daemon Lifecycle -- Failing Tests

**Files:**
- Create: `internal/daemon/daemon.go`
- Create: `internal/daemon/daemon_test.go`

The daemon manages its lifecycle: start (load state, start socket listener, write PID file), stop (close socket, save state, remove PID file), and signal handling (SIGTERM, SIGINT for graceful shutdown).

- [ ] **Step 1: Write the failing tests**

Create `internal/daemon/daemon_test.go`:

```go
package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemon_NewWithConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	d, err := NewDaemon(cfg)
	require.NoError(t, err)
	assert.NotNil(t, d)
}

func TestDaemon_StartAndStop(t *testing.T) {
	dir := t.TempDir()
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Start()
	}()

	// Wait for daemon to be ready
	d.WaitReady()

	// PID file should exist
	pidPath := filepath.Join(dir, "daemon.pid")
	pidData, err := os.ReadFile(pidPath)
	require.NoError(t, err)
	pid, err := strconv.Atoi(string(pidData))
	require.NoError(t, err)
	assert.Equal(t, os.Getpid(), pid)

	// Socket file should exist
	sockPath := filepath.Join(dir, "daemon.sock")
	_, err = os.Stat(sockPath)
	assert.NoError(t, err)

	// Stop the daemon
	err = d.Stop()
	require.NoError(t, err)

	// PID file should be removed
	_, err = os.Stat(pidPath)
	assert.True(t, os.IsNotExist(err))
}

func TestDaemon_Status_NotRunning(t *testing.T) {
	dir := t.TempDir()
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	status := DaemonStatus(cfg)
	assert.False(t, status.Running)
}

func TestDaemon_Status_Running(t *testing.T) {
	dir := t.TempDir()
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	go d.Start()
	d.WaitReady()
	defer d.Stop()

	status := DaemonStatus(cfg)
	assert.True(t, status.Running)
	assert.Equal(t, os.Getpid(), status.PID)
}

func TestDaemon_SavesStateOnStop(t *testing.T) {
	dir := t.TempDir()
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	go d.Start()
	d.WaitReady()

	// Modify state through the store
	d.Store().SetConfig("test-key", "test-value")

	err = d.Stop()
	require.NoError(t, err)

	// State file should exist
	statePath := filepath.Join(dir, "state.json")
	_, err = os.Stat(statePath)
	assert.NoError(t, err)
}

func TestDaemon_LoadsStateOnStart(t *testing.T) {
	dir := t.TempDir()
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	// First daemon: set config and stop
	d1, err := NewDaemon(cfg)
	require.NoError(t, err)
	go d1.Start()
	d1.WaitReady()
	d1.Store().SetConfig("persist-key", "persist-value")
	require.NoError(t, d1.Stop())

	// Second daemon: should load persisted state
	d2, err := NewDaemon(cfg)
	require.NoError(t, err)
	go d2.Start()
	d2.WaitReady()
	defer d2.Stop()

	val, ok := d2.Store().GetConfig("persist-key")
	assert.True(t, ok)
	assert.Equal(t, "persist-value", val)
}

func TestDaemon_StaleSocketCleaned(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "daemon.sock")

	// Create a stale socket file
	os.WriteFile(sockPath, []byte("stale"), 0600)

	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	go d.Start()
	d.WaitReady()
	defer d.Stop()

	// Daemon should have started despite stale socket
	status := DaemonStatus(cfg)
	assert.True(t, status.Running)
}

func TestDaemon_IdentityCreatedOnInit(t *testing.T) {
	dir := t.TempDir()
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	go d.Start()
	d.WaitReady()
	defer d.Stop()

	// Identity file should exist
	idPath := filepath.Join(dir, "identity.json")
	_, err = os.Stat(idPath)
	assert.NoError(t, err)

	assert.NotEmpty(t, d.PeerID())
}
```

- [ ] **Step 2: Create minimal stubs**

Create `internal/daemon/daemon.go`:

```go
package daemon

import (
	"github.com/shaiknoorullah/agenthive/internal/crdt"
)

// DaemonConfig holds configuration for the daemon.
type DaemonConfig struct {
	ConfigDir string
	PeerName  string
}

// Status holds the current daemon status.
type Status struct {
	Running bool
	PID     int
}

// Daemon is the central agenthive process.
type Daemon struct{}

func NewDaemon(cfg DaemonConfig) (*Daemon, error)         { return nil, nil }
func (d *Daemon) Start() error                             { return nil }
func (d *Daemon) Stop() error                              { return nil }
func (d *Daemon) WaitReady()                               {}
func (d *Daemon) Store() *crdt.StateStore                  { return nil }
func (d *Daemon) PeerID() string                           { return "" }
func DaemonStatus(cfg DaemonConfig) Status                 { return Status{} }
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/daemon/ -run TestDaemon -v -count=1 2>&1 | head -30`
Expected: All tests FAIL.

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/daemon/daemon.go internal/daemon/daemon_test.go
git commit -m "test: add failing tests for daemon lifecycle management"
```

---

## Task 13: Daemon Lifecycle -- Implementation

**Files:**
- Modify: `internal/daemon/daemon.go`

- [ ] **Step 1: Implement daemon lifecycle**

Replace `internal/daemon/daemon.go`:

```go
package daemon

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/identity"
)

// DaemonConfig holds configuration for the daemon.
type DaemonConfig struct {
	ConfigDir string
	PeerName  string
}

// Status holds the current daemon status.
type Status struct {
	Running bool
	PID     int
	PeerID  string
}

// Daemon is the central agenthive process.
// It manages the CRDT state store, Unix socket listener, message router,
// message queue, and peer identity.
type Daemon struct {
	cfg      DaemonConfig
	store    *crdt.StateStore
	identity *identity.Identity
	socket   *SocketListener
	router   *Router
	queue    *Queue
	ready    chan struct{}
	stop     chan struct{}
	stopped  chan struct{}
	mu       sync.Mutex
}

// NewDaemon creates a new daemon instance.
func NewDaemon(cfg DaemonConfig) (*Daemon, error) {
	if err := os.MkdirAll(cfg.ConfigDir, 0700); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}

	return &Daemon{
		cfg:     cfg,
		ready:   make(chan struct{}),
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}, nil
}

// Start initializes and runs the daemon.
// Blocks until Stop is called or a signal is received.
func (d *Daemon) Start() error {
	defer close(d.stopped)

	// Load or create identity
	idPath := filepath.Join(d.cfg.ConfigDir, "identity.json")
	id, err := identity.LoadFromFile(idPath)
	if err != nil {
		id, err = identity.Generate(d.cfg.PeerName)
		if err != nil {
			return fmt.Errorf("generate identity: %w", err)
		}
		if err := id.SaveToFile(idPath); err != nil {
			return fmt.Errorf("save identity: %w", err)
		}
	}
	d.identity = id

	// Initialize CRDT state store
	d.store = crdt.NewStateStore(id.PeerID)
	statePath := filepath.Join(d.cfg.ConfigDir, "state.json")
	if err := d.store.LoadFromFile(statePath); err != nil {
		log.Printf("warning: could not load state: %v", err)
	}

	// Initialize message queue
	queueDir := filepath.Join(d.cfg.ConfigDir, "queue")
	d.queue, err = NewQueue(queueDir)
	if err != nil {
		return fmt.Errorf("create message queue: %w", err)
	}

	// Initialize router
	d.router = NewRouter(d.store, id.PeerID)

	// Start socket listener
	sockPath := filepath.Join(d.cfg.ConfigDir, "daemon.sock")
	d.socket, err = NewSocketListener(sockPath, d.handleMessage)
	if err != nil {
		return fmt.Errorf("create socket listener: %w", err)
	}

	// Write PID file
	pidPath := filepath.Join(d.cfg.ConfigDir, "daemon.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		d.socket.Close()
		return fmt.Errorf("write PID file: %w", err)
	}

	// Signal that we are ready
	close(d.ready)

	// Serve socket in background
	go d.socket.Serve()

	// Wait for stop signal
	<-d.stop

	return nil
}

// Stop gracefully shuts down the daemon.
func (d *Daemon) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Signal the main loop to stop
	select {
	case <-d.stop:
		// already stopped
		return nil
	default:
		close(d.stop)
	}

	// Wait for Start to return
	<-d.stopped

	// Close socket listener
	if d.socket != nil {
		d.socket.Close()
	}

	// Save CRDT state
	if d.store != nil {
		statePath := filepath.Join(d.cfg.ConfigDir, "state.json")
		if err := d.store.SaveToFile(statePath); err != nil {
			log.Printf("warning: could not save state: %v", err)
		}
	}

	// Remove PID file
	pidPath := filepath.Join(d.cfg.ConfigDir, "daemon.pid")
	os.Remove(pidPath)

	return nil
}

// WaitReady blocks until the daemon is initialized and accepting connections.
func (d *Daemon) WaitReady() {
	<-d.ready
}

// Store returns the CRDT state store.
func (d *Daemon) Store() *crdt.StateStore {
	return d.store
}

// PeerID returns the daemon's peer identity ID.
func (d *Daemon) PeerID() string {
	if d.identity == nil {
		return ""
	}
	return d.identity.PeerID
}

// handleMessage processes a message received from the socket.
func (d *Daemon) handleMessage(msg protocol.Message) {
	// Route the message to target peers
	targets := d.router.Route(msg)

	for _, target := range targets {
		// Check if peer is online (has a link)
		peer, ok := d.store.GetPeer(target)
		if !ok || peer.Status != "online" {
			// Queue for offline peer
			if err := d.queue.Enqueue(target, msg); err != nil {
				log.Printf("error queuing message for %s: %v", target, err)
			}
			continue
		}

		// TODO: forward to link manager (implemented in transport subsystem)
		// For now, queue the message
		if err := d.queue.Enqueue(target, msg); err != nil {
			log.Printf("error queuing message for %s: %v", target, err)
		}
	}
}

// DaemonStatus checks whether a daemon is running for the given config.
func DaemonStatus(cfg DaemonConfig) Status {
	pidPath := filepath.Join(cfg.ConfigDir, "daemon.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return Status{Running: false}
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return Status{Running: false}
	}

	// Check if process is alive
	proc, err := os.FindProcess(pid)
	if err != nil {
		return Status{Running: false}
	}

	// On Unix, FindProcess always succeeds. Send signal 0 to check if alive.
	if err := proc.Signal(os.Signal(nil)); err != nil {
		// Process is not running, clean up stale PID file
		os.Remove(pidPath)
		return Status{Running: false}
	}

	return Status{Running: true, PID: pid}
}
```

- [ ] **Step 2: Add missing import for protocol package**

The `handleMessage` method references `protocol.Message` but the import is implicit through the `MessageHandler` type. Verify the daemon.go file compiles by checking that the `MessageHandler` type parameter in `NewSocketListener` is compatible.

Run: `cd /home/devsupreme/agenthive && go build ./internal/daemon/ 2>&1`

If there is a compile error about the protocol import, add it:

```go
import (
	// ... existing imports ...
	"github.com/shaiknoorullah/agenthive/internal/protocol"
)
```

Note: The `handleMessage` method signature `func(msg protocol.Message)` requires the protocol import. Ensure the import block in `daemon.go` includes the protocol package.

- [ ] **Step 3: Run tests to verify they pass**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/daemon/ -run TestDaemon -v -count=1`
Expected: All tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/daemon/daemon.go
git commit -m "feat: implement daemon lifecycle with state persistence and identity management"
```

---

## Task 14: CLI Entry Point -- Failing Tests

**Files:**
- Create: `cmd/agenthive/main.go`

The CLI provides subcommands: init, start, stop, status, peers, routes, config, respond. Uses cobra for command dispatch. This task sets up the command structure with stub implementations.

- [ ] **Step 1: Create the CLI entry point**

Create `cmd/agenthive/main.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/daemon"
	"github.com/shaiknoorullah/agenthive/internal/identity"
	"github.com/spf13/cobra"
)

func defaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "agenthive")
	}
	return filepath.Join(home, ".config", "agenthive")
}

func main() {
	var configDir string

	rootCmd := &cobra.Command{
		Use:   "agenthive",
		Short: "A self-hosted, encrypted mesh for AI agent notification and control",
		Long: `agenthive turns every terminal into a command center for your AI agents.
Get notifications, approve actions, and control your coding agents from anywhere.`,
	}

	rootCmd.PersistentFlags().StringVar(&configDir, "config-dir", defaultConfigDir(), "configuration directory")

	// agenthive init
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize peer identity and configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("name")
			if name == "" {
				hostname, err := os.Hostname()
				if err != nil {
					name = "unnamed"
				} else {
					name = hostname
				}
			}

			if err := os.MkdirAll(configDir, 0700); err != nil {
				return fmt.Errorf("create config directory: %w", err)
			}

			idPath := filepath.Join(configDir, "identity.json")
			if _, err := os.Stat(idPath); err == nil {
				return fmt.Errorf("identity already exists at %s (use --force to regenerate)", idPath)
			}

			id, err := identity.Generate(name)
			if err != nil {
				return fmt.Errorf("generate identity: %w", err)
			}

			if err := id.SaveToFile(idPath); err != nil {
				return fmt.Errorf("save identity: %w", err)
			}

			fmt.Printf("Initialized agenthive peer:\n")
			fmt.Printf("  Name:     %s\n", id.Name)
			fmt.Printf("  Peer ID:  %s\n", id.PeerID)
			fmt.Printf("  Config:   %s\n", configDir)
			return nil
		},
	}
	initCmd.Flags().String("name", "", "peer name (defaults to hostname)")

	// agenthive start
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the agenthive daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			idPath := filepath.Join(configDir, "identity.json")
			id, err := identity.LoadFromFile(idPath)
			if err != nil {
				return fmt.Errorf("load identity (run 'agenthive init' first): %w", err)
			}

			cfg := daemon.DaemonConfig{
				ConfigDir: configDir,
				PeerName:  id.Name,
			}

			d, err := daemon.NewDaemon(cfg)
			if err != nil {
				return fmt.Errorf("create daemon: %w", err)
			}

			fmt.Printf("Starting agenthive daemon (peer: %s)...\n", id.Name)
			return d.Start()
		},
	}

	// agenthive stop
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the agenthive daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := daemon.DaemonConfig{ConfigDir: configDir}
			status := daemon.DaemonStatus(cfg)
			if !status.Running {
				fmt.Println("Daemon is not running.")
				return nil
			}

			proc, err := os.FindProcess(status.PID)
			if err != nil {
				return fmt.Errorf("find daemon process: %w", err)
			}

			if err := proc.Signal(os.Interrupt); err != nil {
				return fmt.Errorf("send interrupt signal: %w", err)
			}

			fmt.Printf("Sent stop signal to daemon (PID %d).\n", status.PID)
			return nil
		},
	}

	// agenthive status
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := daemon.DaemonConfig{ConfigDir: configDir}
			status := daemon.DaemonStatus(cfg)
			if status.Running {
				fmt.Printf("Daemon is running (PID %d).\n", status.PID)
			} else {
				fmt.Println("Daemon is not running.")
			}
			return nil
		},
	}

	// agenthive peers
	peersCmd := &cobra.Command{
		Use:   "peers",
		Short: "List connected peers",
		RunE: func(cmd *cobra.Command, args []string) error {
			store := crdt.NewStateStore("local")
			statePath := filepath.Join(configDir, "state.json")
			if err := store.LoadFromFile(statePath); err != nil {
				return fmt.Errorf("load state: %w", err)
			}

			peers := store.ListPeers()
			if len(peers) == 0 {
				fmt.Println("No peers configured.")
				return nil
			}

			for id, peer := range peers {
				fmt.Printf("  %s  %-12s  %s\n", statusIcon(peer.Status), id, peer.Name)
			}
			return nil
		},
	}

	// agenthive routes
	routesCmd := &cobra.Command{
		Use:   "routes",
		Short: "List or manage routing rules",
	}

	routesListCmd := &cobra.Command{
		Use:   "list",
		Short: "List routing rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			store := crdt.NewStateStore("local")
			statePath := filepath.Join(configDir, "state.json")
			if err := store.LoadFromFile(statePath); err != nil {
				return fmt.Errorf("load state: %w", err)
			}

			routes := store.ListRoutes()
			if len(routes) == 0 {
				fmt.Println("No routing rules configured.")
				return nil
			}

			for id, rule := range routes {
				fmt.Printf("  %-20s -> %v\n", formatMatch(id, rule.Match), rule.Targets)
			}
			return nil
		},
	}

	routesCmd.AddCommand(routesListCmd)

	// agenthive config
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Show or set configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			store := crdt.NewStateStore("local")
			statePath := filepath.Join(configDir, "state.json")
			if err := store.LoadFromFile(statePath); err != nil {
				return fmt.Errorf("load state: %w", err)
			}

			if len(args) == 0 {
				// Show all config as JSON
				data, _ := json.MarshalIndent(store.ConfigMap(), "", "  ")
				fmt.Println(string(data))
				return nil
			}
			return nil
		},
	}

	// agenthive respond
	respondCmd := &cobra.Command{
		Use:   "respond <allow|deny>:<request-id>",
		Short: "Respond to an agent action request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse "allow:req-42" or "deny:req-42"
			fmt.Printf("Response registered: %s\n", args[0])
			return nil
		},
	}

	rootCmd.AddCommand(initCmd, startCmd, stopCmd, statusCmd, peersCmd, routesCmd, configCmd, respondCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func statusIcon(status string) string {
	switch status {
	case "online":
		return "*"
	case "offline":
		return "o"
	default:
		return "?"
	}
}

func formatMatch(id string, match crdt.RouteMatch) string {
	if match.Project == "" && match.Source == "" && match.Session == "" && match.Priority == "" {
		return id + " (default)"
	}
	parts := []string{}
	if match.Project != "" {
		parts = append(parts, "project:"+match.Project)
	}
	if match.Source != "" {
		parts = append(parts, "source:"+match.Source)
	}
	if match.Session != "" {
		parts = append(parts, "session:"+match.Session)
	}
	if match.Priority != "" {
		parts = append(parts, "priority:"+match.Priority)
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}
```

- [ ] **Step 2: Verify the CLI compiles**

Run: `cd /home/devsupreme/agenthive && go build -o /dev/null ./cmd/agenthive/`
Expected: Compiles without errors.

- [ ] **Step 3: Verify help output**

Run: `cd /home/devsupreme/agenthive && go run ./cmd/agenthive/ --help`
Expected: Shows root help with subcommands: init, start, stop, status, peers, routes, config, respond.

Run: `cd /home/devsupreme/agenthive && go run ./cmd/agenthive/ init --help`
Expected: Shows init help with --name flag.

- [ ] **Step 4: Commit**

```bash
git add cmd/agenthive/main.go
git commit -m "feat: implement CLI entry point with cobra subcommand dispatch"
```

---

## Task 15: Run Full Test Suite and Verify

**Files:** None (verification only)

- [ ] **Step 1: Run all unit tests with race detector**

Run: `cd /home/devsupreme/agenthive && go test -race -v -count=1 ./internal/protocol/... ./internal/identity/... ./internal/daemon/...`
Expected: All tests PASS. Zero race conditions.

- [ ] **Step 2: Run all tests including CRDT subsystem**

Run: `cd /home/devsupreme/agenthive && go test -race -v -count=1 ./...`
Expected: All tests PASS. Zero race conditions.

- [ ] **Step 3: Verify CLI binary builds**

Run: `cd /home/devsupreme/agenthive && go build -o /tmp/agenthive-test ./cmd/agenthive/ && /tmp/agenthive-test --help && rm /tmp/agenthive-test`
Expected: Binary builds and help output displays correctly.

- [ ] **Step 4: Check test coverage**

Run: `cd /home/devsupreme/agenthive && go test -race -coverprofile=coverage.out ./internal/protocol/... ./internal/identity/... ./internal/daemon/... && go tool cover -func=coverage.out | tail -1`
Expected: Coverage > 75% across the new packages.

- [ ] **Step 5: Commit coverage artifacts to gitignore (if not already present)**

```bash
grep -q "coverage.out" /home/devsupreme/agenthive/.gitignore || echo "coverage.out" >> /home/devsupreme/agenthive/.gitignore
git add .gitignore
git commit -m "chore: ensure coverage output in gitignore"
```

---

## Summary

| Task | Component | Type | Est. Time |
|------|-----------|------|-----------|
| 1 | CLI dependency (cobra) | Setup | 2 min |
| 2 | Message types failing tests | Test | 5 min |
| 3 | Message types implementation | Code | 5 min |
| 4 | Peer identity failing tests | Test | 5 min |
| 5 | Peer identity implementation | Code | 5 min |
| 6 | Message router failing tests | Test | 5 min |
| 7 | Message router implementation | Code | 5 min |
| 8 | Message queue failing tests | Test | 5 min |
| 9 | Message queue implementation | Code | 5 min |
| 10 | Socket listener failing tests | Test | 5 min |
| 11 | Socket listener implementation | Code | 5 min |
| 12 | Daemon lifecycle failing tests | Test | 5 min |
| 13 | Daemon lifecycle implementation | Code | 5 min |
| 14 | CLI entry point | Code | 5 min |
| 15 | Full suite verification | Verify | 5 min |
| | **Total** | | **~77 min** |
