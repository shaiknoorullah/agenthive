# Transport / Link Manager Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the transport layer for agenthive -- a pluggable link system that moves newline-delimited JSON messages between mesh peers via SSH tunnels (WAN) or direct TCP with Noise Protocol encryption (LAN). The Link Manager multiplexes outbound messages across all active links, aggregates inbound messages into a single stream for the message router, monitors link health via heartbeats, and queues messages to disk when links are down.

**Architecture:** A `Link` interface abstracts over transport types (SSH, TCP+Noise). The `LinkManager` holds a map of peer ID to active `Link`, broadcasts outbound messages to all (or routed) links, and fans inbound messages into a single channel. SSH links spawn `autossh`/`ssh` via `os/exec` and communicate over stdin/stdout of a remote `agenthive relay` subprocess. TCP links use `net.Listener`/`net.Dial` with a Noise Protocol (`flynn/noise`) handshake for encryption. A pairing ceremony exchanges Ed25519 peer identity keys over an existing SSH connection.

**Tech Stack:** Go 1.22+, `github.com/flynn/noise` (Noise Protocol), `crypto/ed25519` (peer identity), `github.com/stretchr/testify` (assertions), `encoding/json` (serialization), `os/exec` (SSH subprocess management).

**Testing strategy:** TDD throughout. Unit tests for each component with mocked links. Integration tests for the full link lifecycle (connect, send, receive, disconnect, reconnect). Race detector on all tests. No external SSH servers required -- tests use in-process pipe-based links.

**Dependencies:** Subsystem 1 (CRDT State Store) provides the `Timestamp` and `HLC` types used in heartbeat messages. Subsystem 2 (Core Daemon) will consume the `LinkManager` for message routing. This plan can be implemented in parallel with Subsystem 2 since the interface boundary (`Link` and `LinkManager`) is defined here.

---

## File Structure

```
agenthive/
  internal/
    transport/
      link.go                   # Link interface, LinkStatus, message envelope types
      link_test.go              # Tests for message envelope serialization
      pipe_link.go              # In-process pipe link (for testing)
      pipe_link_test.go         # Tests for pipe link
      ssh_link.go               # SSH/autossh link via os/exec
      ssh_link_test.go          # Tests for SSH link (mocked exec)
      noise.go                  # Noise Protocol handshake and encrypted conn wrapper
      noise_test.go             # Tests for Noise handshake + encrypted framing
      tcp_link.go               # Direct TCP + Noise Protocol link
      tcp_link_test.go          # Tests for TCP link (loopback)
      link_manager.go           # Manages active links, broadcasts/aggregates messages
      link_manager_test.go      # Tests for LinkManager lifecycle
      pairing.go                # Peer pairing ceremony (key exchange)
      pairing_test.go           # Tests for pairing key generation and exchange
```

---

## Task 1: Link Interface and Message Envelope -- Failing Tests

**Files:**
- Create: `internal/transport/link.go`
- Create: `internal/transport/link_test.go`

The Link interface is the abstraction boundary for all transport types. The message envelope wraps every JSON message with framing metadata for multiplexing.

- [ ] **Step 1: Write the failing tests**

Create `internal/transport/link_test.go`:

```go
package transport

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvelope_JSONRoundTrip_Notification(t *testing.T) {
	env := Envelope{
		Type:      MsgNotification,
		ID:        "a1b2c3d4",
		From:      "peer-server",
		To:        "",
		Timestamp: time.Date(2026, 3, 26, 14, 32, 0, 0, time.UTC),
		Payload:   json.RawMessage(`{"message":"Agent has finished","project":"api-server","source":"Claude"}`),
	}

	data, err := json.Marshal(env)
	require.NoError(t, err)

	var env2 Envelope
	err = json.Unmarshal(data, &env2)
	require.NoError(t, err)

	assert.Equal(t, MsgNotification, env2.Type)
	assert.Equal(t, "a1b2c3d4", env2.ID)
	assert.Equal(t, "peer-server", env2.From)
	assert.True(t, env2.Timestamp.Equal(env.Timestamp))
	assert.JSONEq(t, string(env.Payload), string(env2.Payload))
}

func TestEnvelope_JSONRoundTrip_Heartbeat(t *testing.T) {
	env := Envelope{
		Type:      MsgHeartbeat,
		ID:        "hb-001",
		From:      "peer-laptop",
		Timestamp: time.Date(2026, 3, 26, 14, 34, 0, 0, time.UTC),
		Payload:   json.RawMessage(`{"active_agents":3}`),
	}

	data, err := json.Marshal(env)
	require.NoError(t, err)

	var env2 Envelope
	err = json.Unmarshal(data, &env2)
	require.NoError(t, err)

	assert.Equal(t, MsgHeartbeat, env2.Type)
	assert.Equal(t, "peer-laptop", env2.From)
}

func TestEnvelope_JSONRoundTrip_ActionRequest(t *testing.T) {
	env := Envelope{
		Type:      MsgActionRequest,
		ID:        "e5f6g7h8",
		From:      "peer-server",
		To:        "peer-phone",
		Timestamp: time.Now().UTC(),
		Payload:   json.RawMessage(`{"message":"Agent wants to execute: rm -rf build/","actions":["allow","deny"],"timeout_seconds":300}`),
	}

	data, err := json.Marshal(env)
	require.NoError(t, err)

	var env2 Envelope
	err = json.Unmarshal(data, &env2)
	require.NoError(t, err)

	assert.Equal(t, MsgActionRequest, env2.Type)
	assert.Equal(t, "peer-phone", env2.To)
}

func TestEnvelope_JSONRoundTrip_ActionResponse(t *testing.T) {
	env := Envelope{
		Type:      MsgActionResponse,
		ID:        "resp-001",
		From:      "peer-phone",
		To:        "peer-server",
		Timestamp: time.Now().UTC(),
		Payload:   json.RawMessage(`{"request_id":"e5f6g7h8","action":"deny"}`),
	}

	data, err := json.Marshal(env)
	require.NoError(t, err)

	var env2 Envelope
	err = json.Unmarshal(data, &env2)
	require.NoError(t, err)

	assert.Equal(t, MsgActionResponse, env2.Type)
	assert.Equal(t, "peer-server", env2.To)
}

func TestEnvelope_JSONRoundTrip_ConfigSync(t *testing.T) {
	env := Envelope{
		Type:      MsgConfigSync,
		ID:        "sync-001",
		From:      "peer-phone",
		Timestamp: time.Now().UTC(),
		Payload:   json.RawMessage(`{"deltas":[{"collection":"routes","key":"r1"}]}`),
	}

	data, err := json.Marshal(env)
	require.NoError(t, err)

	var env2 Envelope
	err = json.Unmarshal(data, &env2)
	require.NoError(t, err)

	assert.Equal(t, MsgConfigSync, env2.Type)
}

func TestLinkStatus_StringValues(t *testing.T) {
	assert.Equal(t, "connecting", string(StatusConnecting))
	assert.Equal(t, "connected", string(StatusConnected))
	assert.Equal(t, "disconnected", string(StatusDisconnected))
	assert.Equal(t, "error", string(StatusError))
}

func TestMessageType_AllTypes(t *testing.T) {
	types := []MessageType{
		MsgNotification,
		MsgActionRequest,
		MsgActionResponse,
		MsgConfigSync,
		MsgPeerAnnounce,
		MsgHeartbeat,
		MsgPeerQuery,
		MsgPeerState,
	}
	// All types must be unique non-empty strings
	seen := make(map[MessageType]bool)
	for _, mt := range types {
		assert.NotEmpty(t, string(mt))
		assert.False(t, seen[mt], "duplicate message type: %s", mt)
		seen[mt] = true
	}
}

func TestEnvelopeLineDelimited(t *testing.T) {
	env := Envelope{
		Type:      MsgHeartbeat,
		ID:        "hb-002",
		From:      "peer-a",
		Timestamp: time.Now().UTC(),
		Payload:   json.RawMessage(`{}`),
	}

	data, err := json.Marshal(env)
	require.NoError(t, err)

	// The envelope JSON must not contain newlines (so it works with ndjson framing)
	assert.NotContains(t, string(data), "\n")
}
```

- [ ] **Step 2: Create minimal stubs**

Create `internal/transport/link.go`:

```go
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
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -run Test -v -count=1 2>&1 | head -30`
Expected: All tests FAIL (constants are empty strings, equality checks fail).

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/transport/link.go internal/transport/link_test.go
git commit -m "test: add failing tests for Link interface and Envelope types"
```

---

## Task 2: Link Interface and Message Envelope -- Implementation

**Files:**
- Modify: `internal/transport/link.go`

- [ ] **Step 1: Implement the types with correct constant values**

Replace `internal/transport/link.go`:

```go
package transport

import (
	"encoding/json"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/protocol"
)

// MessageType is an alias for protocol.MessageType.
// Transport-specific types (MsgPeerQuery, MsgPeerState) are defined as
// protocol.MessageType constants and must be added to protocol/messages.go.
type MessageType = protocol.MessageType

const (
	// Transport-specific message types (add to protocol/messages.go)
	MsgPeerQuery MessageType = "peer_query"
	MsgPeerState MessageType = "peer_state"
)

// LinkStatus represents the current state of a link.
type LinkStatus string

const (
	StatusConnecting   LinkStatus = "connecting"
	StatusConnected    LinkStatus = "connected"
	StatusDisconnected LinkStatus = "disconnected"
	StatusError        LinkStatus = "error"
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
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -v -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/transport/link.go
git commit -m "feat: implement Link interface, Envelope, MessageType, and LinkStatus types"
```

---

## Task 3: Pipe Link (Test Double) -- Failing Tests

**Files:**
- Create: `internal/transport/pipe_link.go`
- Create: `internal/transport/pipe_link_test.go`

The PipeLink is an in-process link implementation using Go channels. It is used as the test double for all higher-level tests (LinkManager, message routing) without requiring SSH or TCP. Two PipeLinks are created as a pair -- each one's send goes to the other's receive.

- [ ] **Step 1: Write the failing tests**

Create `internal/transport/pipe_link_test.go`:

```go
package transport

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipeLink_SendAndReceive(t *testing.T) {
	linkA, linkB := NewPipeLinkPair("peer-a", "peer-b")
	defer linkA.Close()
	defer linkB.Close()

	env := Envelope{
		Type:      MsgNotification,
		ID:        "msg-001",
		From:      "peer-a",
		Timestamp: time.Now().UTC(),
		Payload:   json.RawMessage(`{"message":"hello"}`),
	}

	err := linkA.Send(env)
	require.NoError(t, err)

	select {
	case received := <-linkB.Receive():
		assert.Equal(t, "msg-001", received.ID)
		assert.Equal(t, MsgNotification, received.Type)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestPipeLink_Bidirectional(t *testing.T) {
	linkA, linkB := NewPipeLinkPair("peer-a", "peer-b")
	defer linkA.Close()
	defer linkB.Close()

	// A -> B
	err := linkA.Send(Envelope{
		Type: MsgHeartbeat, ID: "a-to-b", From: "peer-a",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	// B -> A
	err = linkB.Send(Envelope{
		Type: MsgHeartbeat, ID: "b-to-a", From: "peer-b",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	select {
	case msg := <-linkB.Receive():
		assert.Equal(t, "a-to-b", msg.ID)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout: B did not receive from A")
	}

	select {
	case msg := <-linkA.Receive():
		assert.Equal(t, "b-to-a", msg.ID)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout: A did not receive from B")
	}
}

func TestPipeLink_Status(t *testing.T) {
	linkA, linkB := NewPipeLinkPair("peer-a", "peer-b")

	assert.Equal(t, StatusConnected, linkA.Status())
	assert.Equal(t, StatusConnected, linkB.Status())

	linkA.Close()
	assert.Equal(t, StatusDisconnected, linkA.Status())
}

func TestPipeLink_PeerID(t *testing.T) {
	linkA, linkB := NewPipeLinkPair("peer-a", "peer-b")
	defer linkA.Close()
	defer linkB.Close()

	// linkA's peer is peer-b (the remote end)
	assert.Equal(t, "peer-b", linkA.PeerID())
	assert.Equal(t, "peer-a", linkB.PeerID())
}

func TestPipeLink_SendAfterClose_ReturnsError(t *testing.T) {
	linkA, linkB := NewPipeLinkPair("peer-a", "peer-b")
	defer linkB.Close()

	linkA.Close()

	err := linkA.Send(Envelope{
		Type: MsgHeartbeat, ID: "late", From: "peer-a",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})
	assert.Error(t, err)
}

func TestPipeLink_CloseIsIdempotent(t *testing.T) {
	linkA, linkB := NewPipeLinkPair("peer-a", "peer-b")
	defer linkB.Close()

	err := linkA.Close()
	assert.NoError(t, err)
	err = linkA.Close()
	assert.NoError(t, err)
}

func TestPipeLink_MultipleMessages(t *testing.T) {
	linkA, linkB := NewPipeLinkPair("peer-a", "peer-b")
	defer linkA.Close()
	defer linkB.Close()

	for i := 0; i < 10; i++ {
		err := linkA.Send(Envelope{
			Type: MsgNotification, ID: "msg", From: "peer-a",
			Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
		})
		require.NoError(t, err)
	}

	received := 0
	for received < 10 {
		select {
		case <-linkB.Receive():
			received++
		case <-time.After(1 * time.Second):
			t.Fatalf("timeout after receiving %d messages", received)
		}
	}
	assert.Equal(t, 10, received)
}
```

- [ ] **Step 2: Create minimal stubs**

Create `internal/transport/pipe_link.go`:

```go
package transport

// PipeLink is an in-process link for testing. Two PipeLinks form a pair.
type PipeLink struct{}

func NewPipeLinkPair(peerIDA, peerIDB string) (*PipeLink, *PipeLink) { return nil, nil }
func (p *PipeLink) Send(env Envelope) error                          { return nil }
func (p *PipeLink) Receive() <-chan Envelope                          { return nil }
func (p *PipeLink) Close() error                                      { return nil }
func (p *PipeLink) Status() LinkStatus                                { return "" }
func (p *PipeLink) PeerID() string                                    { return "" }
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -run TestPipeLink -v -count=1 2>&1 | head -30`
Expected: All tests FAIL (nil channel, zero values).

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/transport/pipe_link.go internal/transport/pipe_link_test.go
git commit -m "test: add failing tests for PipeLink test double"
```

---

## Task 4: Pipe Link -- Implementation

**Files:**
- Modify: `internal/transport/pipe_link.go`

- [ ] **Step 1: Implement PipeLink**

Replace `internal/transport/pipe_link.go`:

```go
package transport

import (
	"errors"
	"sync"
)

// PipeLink is an in-process link for testing. Two PipeLinks form a pair:
// each one's Send delivers to the other's Receive channel.
type PipeLink struct {
	mu       sync.Mutex
	peerID   string
	sendCh   chan Envelope // outbound: this link writes here
	recvCh   chan Envelope // inbound: this link reads from here
	closed   bool
	closedCh chan struct{}
}

// NewPipeLinkPair creates two connected PipeLinks.
// linkA.Send() delivers to linkB.Receive() and vice versa.
func NewPipeLinkPair(peerIDA, peerIDB string) (*PipeLink, *PipeLink) {
	aToB := make(chan Envelope, 64)
	bToA := make(chan Envelope, 64)

	linkA := &PipeLink{
		peerID:   peerIDB, // A's remote peer is B
		sendCh:   aToB,
		recvCh:   bToA,
		closedCh: make(chan struct{}),
	}
	linkB := &PipeLink{
		peerID:   peerIDA, // B's remote peer is A
		sendCh:   bToA,
		recvCh:   aToB,
		closedCh: make(chan struct{}),
	}
	return linkA, linkB
}

func (p *PipeLink) Send(env Envelope) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return errors.New("link closed")
	}
	p.mu.Unlock()

	select {
	case p.sendCh <- env:
		return nil
	case <-p.closedCh:
		return errors.New("link closed")
	}
}

func (p *PipeLink) Receive() <-chan Envelope {
	return p.recvCh
}

func (p *PipeLink) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true
	close(p.closedCh)
	return nil
}

func (p *PipeLink) Status() LinkStatus {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return StatusDisconnected
	}
	return StatusConnected
}

func (p *PipeLink) PeerID() string {
	return p.peerID
}

// Compile-time interface check.
var _ Link = (*PipeLink)(nil)
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -run TestPipeLink -v -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/transport/pipe_link.go
git commit -m "feat: implement PipeLink in-process test double for Link interface"
```

---

## Task 5: Noise Protocol Handshake -- Failing Tests

**Files:**
- Create: `internal/transport/noise.go`
- Create: `internal/transport/noise_test.go`

The Noise Protocol provides authenticated encryption for direct TCP links between LAN peers. We use the `Noise_XX` handshake pattern with Ed25519 static keys, wrapping a `net.Conn` into an encrypted read/write stream. The `XX` pattern provides mutual authentication -- both sides learn the other's static key during the handshake.

- [ ] **Step 1: Add the noise dependency**

```bash
cd /home/devsupreme/agenthive && go get github.com/flynn/noise@latest
```

- [ ] **Step 2: Write the failing tests**

Create `internal/transport/noise_test.go`:

```go
package transport

import (
	"io"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoiseHandshake_MutualAuthentication(t *testing.T) {
	initiatorKey, err := GenerateNoiseKeypair()
	require.NoError(t, err)
	responderKey, err := GenerateNoiseKeypair()
	require.NoError(t, err)

	connI, connR := net.Pipe()
	defer connI.Close()
	defer connR.Close()

	var wg sync.WaitGroup
	var initiatorConn *NoiseConn
	var responderConn *NoiseConn
	var errI, errR error

	wg.Add(2)
	go func() {
		defer wg.Done()
		initiatorConn, errI = NoiseHandshakeInitiator(connI, initiatorKey, nil)
	}()
	go func() {
		defer wg.Done()
		responderConn, errR = NoiseHandshakeResponder(connR, responderKey, nil)
	}()
	wg.Wait()

	require.NoError(t, errI, "initiator handshake failed")
	require.NoError(t, errR, "responder handshake failed")

	// Both sides should know the other's public key
	assert.Equal(t, responderKey.Public, initiatorConn.RemoteStatic())
	assert.Equal(t, initiatorKey.Public, responderConn.RemoteStatic())
}

func TestNoiseConn_SendAndReceive(t *testing.T) {
	initiatorKey, _ := GenerateNoiseKeypair()
	responderKey, _ := GenerateNoiseKeypair()

	connI, connR := net.Pipe()
	defer connI.Close()
	defer connR.Close()

	var wg sync.WaitGroup
	var iConn, rConn *NoiseConn

	wg.Add(2)
	go func() {
		defer wg.Done()
		iConn, _ = NoiseHandshakeInitiator(connI, initiatorKey, nil)
	}()
	go func() {
		defer wg.Done()
		rConn, _ = NoiseHandshakeResponder(connR, responderKey, nil)
	}()
	wg.Wait()

	// Send from initiator to responder
	message := []byte("hello from initiator")
	err := iConn.WriteMessage(message)
	require.NoError(t, err)

	received, err := rConn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, message, received)

	// Send from responder to initiator
	reply := []byte("hello from responder")
	err = rConn.WriteMessage(reply)
	require.NoError(t, err)

	received2, err := iConn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, reply, received2)
}

func TestNoiseConn_LargeMessage(t *testing.T) {
	initiatorKey, _ := GenerateNoiseKeypair()
	responderKey, _ := GenerateNoiseKeypair()

	connI, connR := net.Pipe()
	defer connI.Close()
	defer connR.Close()

	var wg sync.WaitGroup
	var iConn, rConn *NoiseConn

	wg.Add(2)
	go func() {
		defer wg.Done()
		iConn, _ = NoiseHandshakeInitiator(connI, initiatorKey, nil)
	}()
	go func() {
		defer wg.Done()
		rConn, _ = NoiseHandshakeResponder(connR, responderKey, nil)
	}()
	wg.Wait()

	// Send a message larger than Noise's 65535 byte limit -- should be handled by chunking
	bigMsg := make([]byte, 100_000)
	for i := range bigMsg {
		bigMsg[i] = byte(i % 256)
	}

	err := iConn.WriteMessage(bigMsg)
	require.NoError(t, err)

	received, err := rConn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, bigMsg, received)
}

func TestNoiseConn_MultipleMessages(t *testing.T) {
	initiatorKey, _ := GenerateNoiseKeypair()
	responderKey, _ := GenerateNoiseKeypair()

	connI, connR := net.Pipe()
	defer connI.Close()
	defer connR.Close()

	var wg sync.WaitGroup
	var iConn, rConn *NoiseConn

	wg.Add(2)
	go func() {
		defer wg.Done()
		iConn, _ = NoiseHandshakeInitiator(connI, initiatorKey, nil)
	}()
	go func() {
		defer wg.Done()
		rConn, _ = NoiseHandshakeResponder(connR, responderKey, nil)
	}()
	wg.Wait()

	for i := 0; i < 20; i++ {
		msg := []byte("message number")
		err := iConn.WriteMessage(msg)
		require.NoError(t, err)

		received, err := rConn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, msg, received)
	}
}

func TestNoiseConn_ReadAfterRemoteClose(t *testing.T) {
	initiatorKey, _ := GenerateNoiseKeypair()
	responderKey, _ := GenerateNoiseKeypair()

	connI, connR := net.Pipe()

	var wg sync.WaitGroup
	var iConn, rConn *NoiseConn

	wg.Add(2)
	go func() {
		defer wg.Done()
		iConn, _ = NoiseHandshakeInitiator(connI, initiatorKey, nil)
	}()
	go func() {
		defer wg.Done()
		rConn, _ = NoiseHandshakeResponder(connR, responderKey, nil)
	}()
	wg.Wait()

	// Close the initiator side
	iConn.Close()
	connI.Close()

	// Responder should get an error on read
	_, err := rConn.ReadMessage()
	assert.Error(t, err)
	assert.ErrorIs(t, err, io.EOF)
}

func TestNoiseHandshake_PeerVerification_Accepted(t *testing.T) {
	initiatorKey, _ := GenerateNoiseKeypair()
	responderKey, _ := GenerateNoiseKeypair()

	connI, connR := net.Pipe()
	defer connI.Close()
	defer connR.Close()

	// Verifier that accepts the known key
	verifier := func(remoteKey []byte) error {
		if string(remoteKey) != string(responderKey.Public) {
			return io.ErrUnexpectedEOF
		}
		return nil
	}

	var wg sync.WaitGroup
	var errI error

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errI = NoiseHandshakeInitiator(connI, initiatorKey, verifier)
	}()
	go func() {
		defer wg.Done()
		NoiseHandshakeResponder(connR, responderKey, nil)
	}()
	wg.Wait()

	assert.NoError(t, errI)
}

func TestNoiseHandshake_PeerVerification_Rejected(t *testing.T) {
	initiatorKey, _ := GenerateNoiseKeypair()
	responderKey, _ := GenerateNoiseKeypair()

	connI, connR := net.Pipe()
	defer connI.Close()
	defer connR.Close()

	// Verifier that rejects all keys
	rejecter := func(remoteKey []byte) error {
		return errors.New("untrusted peer")
	}

	var wg sync.WaitGroup
	var errI error

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errI = NoiseHandshakeInitiator(connI, initiatorKey, rejecter)
	}()
	go func() {
		defer wg.Done()
		NoiseHandshakeResponder(connR, responderKey, nil)
	}()
	wg.Wait()

	assert.Error(t, errI)
	assert.Contains(t, errI.Error(), "untrusted")
}

func TestGenerateNoiseKeypair(t *testing.T) {
	kp1, err := GenerateNoiseKeypair()
	require.NoError(t, err)
	assert.Len(t, kp1.Private, 32)
	assert.Len(t, kp1.Public, 32)

	kp2, err := GenerateNoiseKeypair()
	require.NoError(t, err)

	// Two generated keypairs must be different
	assert.NotEqual(t, kp1.Private, kp2.Private)
	assert.NotEqual(t, kp1.Public, kp2.Public)
}
```

- [ ] **Step 3: Create minimal stubs**

Create `internal/transport/noise.go`:

```go
package transport

import (
	"errors"
	"io"
	"net"
)

// Suppress unused import warnings.
var _ = errors.New
var _ io.Reader
var _ net.Conn

// NoiseKeypair holds a Curve25519 keypair for Noise Protocol.
type NoiseKeypair struct {
	Private []byte
	Public  []byte
}

// NoiseConn wraps a net.Conn with Noise Protocol encryption.
type NoiseConn struct{}

// PeerVerifier is called during handshake with the remote peer's static public key.
// Return nil to accept, non-nil error to reject.
type PeerVerifier func(remoteStaticKey []byte) error

func GenerateNoiseKeypair() (*NoiseKeypair, error)                                              { return nil, nil }
func NoiseHandshakeInitiator(conn net.Conn, localKey *NoiseKeypair, verify PeerVerifier) (*NoiseConn, error) { return nil, nil }
func NoiseHandshakeResponder(conn net.Conn, localKey *NoiseKeypair, verify PeerVerifier) (*NoiseConn, error) { return nil, nil }
func (nc *NoiseConn) WriteMessage(msg []byte) error                                              { return nil }
func (nc *NoiseConn) ReadMessage() ([]byte, error)                                               { return nil, nil }
func (nc *NoiseConn) RemoteStatic() []byte                                                        { return nil }
func (nc *NoiseConn) Close() error                                                                { return nil }
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -run TestNoise -v -count=1 2>&1 | head -30`
Expected: All tests FAIL (nil returns, zero values).

- [ ] **Step 5: Commit failing tests**

```bash
git add internal/transport/noise.go internal/transport/noise_test.go
git commit -m "test: add failing tests for Noise Protocol handshake and encrypted framing"
```

---

## Task 6: Noise Protocol Handshake -- Implementation

**Files:**
- Modify: `internal/transport/noise.go`

Uses the `Noise_XX` handshake pattern (3-message handshake: both sides transmit their static keys). After handshake, messages are length-prefixed and encrypted with ChaCha20-Poly1305.

- [ ] **Step 1: Implement the Noise Protocol layer**

Replace `internal/transport/noise.go`:

```go
package transport

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/flynn/noise"
)

// NoiseKeypair holds a Curve25519 keypair for Noise Protocol.
type NoiseKeypair struct {
	Private []byte
	Public  []byte
}

// NoiseConn wraps a net.Conn with Noise Protocol encryption.
type NoiseConn struct {
	mu           sync.Mutex
	conn         net.Conn
	send         *noise.CipherState
	recv         *noise.CipherState
	remoteStatic []byte
}

// PeerVerifier is called during handshake with the remote peer's static public key.
// Return nil to accept, non-nil error to reject.
type PeerVerifier func(remoteStaticKey []byte) error

// maxNoisePayload is the maximum Noise message payload (before encryption overhead).
const maxNoisePayload = 65535 - 16 // 65519 bytes (minus AEAD tag)

// GenerateNoiseKeypair generates a new Curve25519 keypair for Noise Protocol.
func GenerateNoiseKeypair() (*NoiseKeypair, error) {
	kp, err := noise.DH25519.GenerateKeypair(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate noise keypair: %w", err)
	}
	return &NoiseKeypair{
		Private: kp.Private,
		Public:  kp.Public,
	}, nil
}

func noiseConfig(localKey *NoiseKeypair, initiator bool) noise.Config {
	return noise.Config{
		CipherSuite: noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256),
		Pattern:     noise.HandshakeXX,
		Initiator:   initiator,
		StaticKeypair: noise.DHKey{
			Private: localKey.Private,
			Public:  localKey.Public,
		},
	}
}

// NoiseHandshakeInitiator performs the initiator side of a Noise_XX handshake.
func NoiseHandshakeInitiator(conn net.Conn, localKey *NoiseKeypair, verify PeerVerifier) (*NoiseConn, error) {
	hs, err := noise.NewHandshakeState(noiseConfig(localKey, true))
	if err != nil {
		return nil, fmt.Errorf("noise handshake init: %w", err)
	}

	// Message 1: initiator -> responder (e)
	msg1, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("noise write msg1: %w", err)
	}
	if err := writeFrame(conn, msg1); err != nil {
		return nil, fmt.Errorf("noise send msg1: %w", err)
	}

	// Message 2: responder -> initiator (e, ee, s, es)
	msg2, err := readFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("noise read msg2: %w", err)
	}
	_, _, _, err = hs.ReadMessage(nil, msg2)
	if err != nil {
		return nil, fmt.Errorf("noise process msg2: %w", err)
	}

	// Message 3: initiator -> responder (s, se)
	msg3, csI, csR, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("noise write msg3: %w", err)
	}
	if err := writeFrame(conn, msg3); err != nil {
		return nil, fmt.Errorf("noise send msg3: %w", err)
	}

	remoteStatic := hs.PeerStatic()

	if verify != nil {
		if err := verify(remoteStatic); err != nil {
			return nil, err
		}
	}

	return &NoiseConn{
		conn:         conn,
		send:         csI,
		recv:         csR,
		remoteStatic: remoteStatic,
	}, nil
}

// NoiseHandshakeResponder performs the responder side of a Noise_XX handshake.
func NoiseHandshakeResponder(conn net.Conn, localKey *NoiseKeypair, verify PeerVerifier) (*NoiseConn, error) {
	hs, err := noise.NewHandshakeState(noiseConfig(localKey, false))
	if err != nil {
		return nil, fmt.Errorf("noise handshake init: %w", err)
	}

	// Message 1: initiator -> responder (e)
	msg1, err := readFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("noise read msg1: %w", err)
	}
	_, _, _, err = hs.ReadMessage(nil, msg1)
	if err != nil {
		return nil, fmt.Errorf("noise process msg1: %w", err)
	}

	// Message 2: responder -> initiator (e, ee, s, es)
	msg2, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("noise write msg2: %w", err)
	}
	if err := writeFrame(conn, msg2); err != nil {
		return nil, fmt.Errorf("noise send msg2: %w", err)
	}

	// Message 3: initiator -> responder (s, se)
	msg3, err := readFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("noise read msg3: %w", err)
	}
	_, csR, csI, err := hs.ReadMessage(nil, msg3)
	if err != nil {
		return nil, fmt.Errorf("noise process msg3: %w", err)
	}

	remoteStatic := hs.PeerStatic()

	if verify != nil {
		if err := verify(remoteStatic); err != nil {
			return nil, err
		}
	}

	return &NoiseConn{
		conn:         conn,
		send:         csR,
		recv:         csI,
		remoteStatic: remoteStatic,
	}, nil
}

// WriteMessage encrypts and sends a message. Handles chunking for messages
// larger than the Noise maximum payload size.
func (nc *NoiseConn) WriteMessage(msg []byte) error {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	// Write the total unencrypted message length first (4 bytes, big-endian)
	totalLen := make([]byte, 4)
	binary.BigEndian.PutUint32(totalLen, uint32(len(msg)))

	// Encrypt and send the length as a small Noise message
	encLen := nc.send.Encrypt(nil, nil, totalLen)
	if err := writeFrame(nc.conn, encLen); err != nil {
		return fmt.Errorf("noise write length: %w", err)
	}

	// Send the payload in chunks that fit within Noise's max payload
	for len(msg) > 0 {
		chunk := msg
		if len(chunk) > maxNoisePayload {
			chunk = msg[:maxNoisePayload]
		}
		msg = msg[len(chunk):]

		encrypted := nc.send.Encrypt(nil, nil, chunk)
		if err := writeFrame(nc.conn, encrypted); err != nil {
			return fmt.Errorf("noise write chunk: %w", err)
		}
	}

	return nil
}

// ReadMessage reads and decrypts a message.
func (nc *NoiseConn) ReadMessage() ([]byte, error) {
	// Read the encrypted length
	encLen, err := readFrame(nc.conn)
	if err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("noise read length: %w", err)
	}

	totalLenBytes, err := nc.recv.Decrypt(nil, nil, encLen)
	if err != nil {
		return nil, fmt.Errorf("noise decrypt length: %w", err)
	}
	totalLen := binary.BigEndian.Uint32(totalLenBytes)

	// Read chunks until we have the full message
	result := make([]byte, 0, totalLen)
	for uint32(len(result)) < totalLen {
		encChunk, err := readFrame(nc.conn)
		if err != nil {
			return nil, fmt.Errorf("noise read chunk: %w", err)
		}

		chunk, err := nc.recv.Decrypt(nil, nil, encChunk)
		if err != nil {
			return nil, fmt.Errorf("noise decrypt chunk: %w", err)
		}
		result = append(result, chunk...)
	}

	return result, nil
}

func (nc *NoiseConn) RemoteStatic() []byte {
	return nc.remoteStatic
}

func (nc *NoiseConn) Close() error {
	return nc.conn.Close()
}

// writeFrame writes a length-prefixed frame to the connection.
// Frame format: [4 bytes big-endian length][payload]
func writeFrame(conn net.Conn, data []byte) error {
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(data)
	return err
}

// readFrame reads a length-prefixed frame from the connection.
func readFrame(conn net.Conn) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(header)
	if length > 10*1024*1024 { // 10 MB sanity limit
		return nil, fmt.Errorf("frame too large: %d bytes", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}
	return data, nil
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -run TestNoise -v -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Run all transport tests**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -v -count=1`
Expected: All tests PASS (envelope tests + pipe link tests + noise tests).

- [ ] **Step 4: Commit**

```bash
git add internal/transport/noise.go
git commit -m "feat: implement Noise Protocol XX handshake with chunked encrypted framing"
```

---

## Task 7: TCP Link -- Failing Tests

**Files:**
- Create: `internal/transport/tcp_link.go`
- Create: `internal/transport/tcp_link_test.go`

The TCP link connects two peers on the same LAN using direct TCP with Noise Protocol encryption. It sends newline-delimited JSON envelopes over the encrypted connection.

- [ ] **Step 1: Write the failing tests**

Create `internal/transport/tcp_link_test.go`:

```go
package transport

import (
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTCPLink_ConnectAndSend(t *testing.T) {
	serverKey, _ := GenerateNoiseKeypair()
	clientKey, _ := GenerateNoiseKeypair()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	var serverLink *TCPLink
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := listener.Accept()
		require.NoError(t, err)
		serverLink, err = NewTCPLinkFromConn(conn, serverKey, "peer-client", false, nil)
		require.NoError(t, err)
	}()

	clientLink, err := DialTCPLink(listener.Addr().String(), clientKey, "peer-server", nil)
	require.NoError(t, err)
	wg.Wait()

	defer clientLink.Close()
	defer serverLink.Close()

	// Client sends to server
	env := Envelope{
		Type: MsgNotification, ID: "tcp-001", From: "peer-client",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{"msg":"hello"}`),
	}
	err = clientLink.Send(env)
	require.NoError(t, err)

	select {
	case received := <-serverLink.Receive():
		assert.Equal(t, "tcp-001", received.ID)
		assert.Equal(t, MsgNotification, received.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestTCPLink_Bidirectional(t *testing.T) {
	serverKey, _ := GenerateNoiseKeypair()
	clientKey, _ := GenerateNoiseKeypair()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	var serverLink *TCPLink
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, _ := listener.Accept()
		serverLink, _ = NewTCPLinkFromConn(conn, serverKey, "peer-client", false, nil)
	}()

	clientLink, err := DialTCPLink(listener.Addr().String(), clientKey, "peer-server", nil)
	require.NoError(t, err)
	wg.Wait()

	defer clientLink.Close()
	defer serverLink.Close()

	// Client -> Server
	clientLink.Send(Envelope{
		Type: MsgHeartbeat, ID: "c2s", From: "peer-client",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})

	// Server -> Client
	serverLink.Send(Envelope{
		Type: MsgHeartbeat, ID: "s2c", From: "peer-server",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})

	select {
	case msg := <-serverLink.Receive():
		assert.Equal(t, "c2s", msg.ID)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: server did not receive")
	}

	select {
	case msg := <-clientLink.Receive():
		assert.Equal(t, "s2c", msg.ID)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: client did not receive")
	}
}

func TestTCPLink_Status(t *testing.T) {
	serverKey, _ := GenerateNoiseKeypair()
	clientKey, _ := GenerateNoiseKeypair()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	var serverLink *TCPLink
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, _ := listener.Accept()
		serverLink, _ = NewTCPLinkFromConn(conn, serverKey, "peer-client", false, nil)
	}()

	clientLink, _ := DialTCPLink(listener.Addr().String(), clientKey, "peer-server", nil)
	wg.Wait()

	assert.Equal(t, StatusConnected, clientLink.Status())
	assert.Equal(t, StatusConnected, serverLink.Status())

	clientLink.Close()
	// Give the read goroutine time to detect the close
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, StatusDisconnected, clientLink.Status())
}

func TestTCPLink_PeerID(t *testing.T) {
	serverKey, _ := GenerateNoiseKeypair()
	clientKey, _ := GenerateNoiseKeypair()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	var serverLink *TCPLink
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, _ := listener.Accept()
		serverLink, _ = NewTCPLinkFromConn(conn, serverKey, "peer-client", false, nil)
	}()

	clientLink, _ := DialTCPLink(listener.Addr().String(), clientKey, "peer-server", nil)
	wg.Wait()

	defer clientLink.Close()
	defer serverLink.Close()

	assert.Equal(t, "peer-server", clientLink.PeerID())
	assert.Equal(t, "peer-client", serverLink.PeerID())
}

func TestTCPLink_MultipleMessages(t *testing.T) {
	serverKey, _ := GenerateNoiseKeypair()
	clientKey, _ := GenerateNoiseKeypair()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	var serverLink *TCPLink
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, _ := listener.Accept()
		serverLink, _ = NewTCPLinkFromConn(conn, serverKey, "peer-client", false, nil)
	}()

	clientLink, _ := DialTCPLink(listener.Addr().String(), clientKey, "peer-server", nil)
	wg.Wait()

	defer clientLink.Close()
	defer serverLink.Close()

	const count = 50
	for i := 0; i < count; i++ {
		clientLink.Send(Envelope{
			Type: MsgNotification, ID: "msg", From: "peer-client",
			Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
		})
	}

	received := 0
	for received < count {
		select {
		case <-serverLink.Receive():
			received++
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout after receiving %d/%d messages", received, count)
		}
	}
	assert.Equal(t, count, received)
}

func TestTCPLink_ImplementsLinkInterface(t *testing.T) {
	// Compile-time check is in tcp_link.go, but verify here too
	var _ Link = (*TCPLink)(nil)
}
```

- [ ] **Step 2: Create minimal stubs**

Create `internal/transport/tcp_link.go`:

```go
package transport

import "net"

// Suppress unused import.
var _ net.Conn

// TCPLink is a direct TCP connection with Noise Protocol encryption.
type TCPLink struct{}

func DialTCPLink(addr string, localKey *NoiseKeypair, remotePeerID string, verify PeerVerifier) (*TCPLink, error) {
	return nil, nil
}
func NewTCPLinkFromConn(conn net.Conn, localKey *NoiseKeypair, remotePeerID string, initiator bool, verify PeerVerifier) (*TCPLink, error) {
	return nil, nil
}
func (tl *TCPLink) Send(env Envelope) error    { return nil }
func (tl *TCPLink) Receive() <-chan Envelope    { return nil }
func (tl *TCPLink) Close() error               { return nil }
func (tl *TCPLink) Status() LinkStatus          { return "" }
func (tl *TCPLink) PeerID() string              { return "" }
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -run TestTCPLink -v -count=1 2>&1 | head -30`
Expected: All tests FAIL.

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/transport/tcp_link.go internal/transport/tcp_link_test.go
git commit -m "test: add failing tests for TCP + Noise Protocol link"
```

---

## Task 8: TCP Link -- Implementation

**Files:**
- Modify: `internal/transport/tcp_link.go`

- [ ] **Step 1: Implement TCPLink**

Replace `internal/transport/tcp_link.go`:

```go
package transport

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

// TCPLink is a direct TCP connection with Noise Protocol encryption.
// Implements the Link interface. Used for LAN peers where SSH overhead
// is unnecessary.
type TCPLink struct {
	mu       sync.Mutex
	nconn    *NoiseConn
	peerID   string
	recvCh   chan Envelope
	status   LinkStatus
	closedCh chan struct{}
	closed   bool
}

// DialTCPLink connects to a remote peer and performs a Noise handshake as initiator.
func DialTCPLink(addr string, localKey *NoiseKeypair, remotePeerID string, verify PeerVerifier) (*TCPLink, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tcp dial: %w", err)
	}
	return NewTCPLinkFromConn(conn, localKey, remotePeerID, true, verify)
}

// NewTCPLinkFromConn wraps an existing connection with Noise encryption.
// If initiator is true, performs the initiator side of the handshake.
func NewTCPLinkFromConn(conn net.Conn, localKey *NoiseKeypair, remotePeerID string, initiator bool, verify PeerVerifier) (*TCPLink, error) {
	var nconn *NoiseConn
	var err error

	if initiator {
		nconn, err = NoiseHandshakeInitiator(conn, localKey, verify)
	} else {
		nconn, err = NoiseHandshakeResponder(conn, localKey, verify)
	}
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("noise handshake: %w", err)
	}

	tl := &TCPLink{
		nconn:    nconn,
		peerID:   remotePeerID,
		recvCh:   make(chan Envelope, 64),
		status:   StatusConnected,
		closedCh: make(chan struct{}),
	}

	go tl.readLoop()
	return tl, nil
}

func (tl *TCPLink) readLoop() {
	defer func() {
		tl.mu.Lock()
		tl.status = StatusDisconnected
		tl.mu.Unlock()
		close(tl.recvCh)
	}()

	for {
		msg, err := tl.nconn.ReadMessage()
		if err != nil {
			return
		}

		var env Envelope
		if err := json.Unmarshal(msg, &env); err != nil {
			continue // skip malformed messages
		}

		select {
		case tl.recvCh <- env:
		case <-tl.closedCh:
			return
		}
	}
}

func (tl *TCPLink) Send(env Envelope) error {
	tl.mu.Lock()
	if tl.closed {
		tl.mu.Unlock()
		return fmt.Errorf("link closed")
	}
	tl.mu.Unlock()

	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	return tl.nconn.WriteMessage(data)
}

func (tl *TCPLink) Receive() <-chan Envelope {
	return tl.recvCh
}

func (tl *TCPLink) Close() error {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	if tl.closed {
		return nil
	}
	tl.closed = true
	tl.status = StatusDisconnected
	close(tl.closedCh)
	return tl.nconn.Close()
}

func (tl *TCPLink) Status() LinkStatus {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	return tl.status
}

func (tl *TCPLink) PeerID() string {
	return tl.peerID
}

// Compile-time interface check.
var _ Link = (*TCPLink)(nil)
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -run TestTCPLink -v -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/transport/tcp_link.go
git commit -m "feat: implement TCP + Noise Protocol encrypted link for LAN peers"
```

---

## Task 9: SSH Link -- Failing Tests

**Files:**
- Create: `internal/transport/ssh_link.go`
- Create: `internal/transport/ssh_link_test.go`

The SSH link spawns `ssh` (or `autossh`) via `os/exec` to run a remote `agenthive relay` subprocess. Messages are newline-delimited JSON over the subprocess's stdin/stdout. Tests use a mock command that simulates SSH behavior by echoing messages back.

- [ ] **Step 1: Write the failing tests**

Create `internal/transport/ssh_link_test.go`:

```go
package transport

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHLinkConfig_Validate_Valid(t *testing.T) {
	cfg := SSHLinkConfig{
		RemoteUser: "deploy",
		RemoteHost: "server-01",
		RemotePort: 22,
		PeerID:     "peer-server",
		IdentityFile: "/home/user/.ssh/id_ed25519",
	}
	assert.NoError(t, cfg.Validate())
}

func TestSSHLinkConfig_Validate_MissingHost(t *testing.T) {
	cfg := SSHLinkConfig{
		RemoteUser: "deploy",
		PeerID:     "peer-server",
	}
	assert.Error(t, cfg.Validate())
}

func TestSSHLinkConfig_Validate_MissingPeerID(t *testing.T) {
	cfg := SSHLinkConfig{
		RemoteUser: "deploy",
		RemoteHost: "server-01",
	}
	assert.Error(t, cfg.Validate())
}

func TestSSHLinkConfig_SSHArgs_Basic(t *testing.T) {
	cfg := SSHLinkConfig{
		RemoteUser: "deploy",
		RemoteHost: "server-01",
		RemotePort: 22,
		PeerID:     "peer-server",
	}

	args := cfg.SSHArgs("agenthive relay")
	// Must contain the remote destination
	assert.Contains(t, args, "deploy@server-01")
	// Must contain the relay command
	assert.Contains(t, args, "agenthive relay")
}

func TestSSHLinkConfig_SSHArgs_CustomPort(t *testing.T) {
	cfg := SSHLinkConfig{
		RemoteUser: "deploy",
		RemoteHost: "phone",
		RemotePort: 8022,
		PeerID:     "peer-phone",
	}

	args := cfg.SSHArgs("agenthive relay")
	// Must include -p 8022
	foundPort := false
	for i, arg := range args {
		if arg == "-p" && i+1 < len(args) && args[i+1] == "8022" {
			foundPort = true
		}
	}
	assert.True(t, foundPort, "custom port not found in args: %v", args)
}

func TestSSHLinkConfig_SSHArgs_IdentityFile(t *testing.T) {
	cfg := SSHLinkConfig{
		RemoteUser:   "deploy",
		RemoteHost:   "server-01",
		PeerID:       "peer-server",
		IdentityFile: "/home/user/.ssh/agenthive_key",
	}

	args := cfg.SSHArgs("agenthive relay")
	foundIdentity := false
	for i, arg := range args {
		if arg == "-i" && i+1 < len(args) && args[i+1] == "/home/user/.ssh/agenthive_key" {
			foundIdentity = true
		}
	}
	assert.True(t, foundIdentity, "identity file not found in args: %v", args)
}

func TestSSHLinkConfig_SSHArgs_IncludesKeepAlive(t *testing.T) {
	cfg := SSHLinkConfig{
		RemoteUser: "deploy",
		RemoteHost: "server-01",
		PeerID:     "peer-server",
	}

	args := cfg.SSHArgs("agenthive relay")
	joined := ""
	for _, a := range args {
		joined += a + " "
	}
	assert.Contains(t, joined, "ServerAliveInterval")
	assert.Contains(t, joined, "ServerAliveCountMax")
}

func TestSSHLink_ImplementsLinkInterface(t *testing.T) {
	var _ Link = (*SSHLink)(nil)
}

// TestSSHLink_WithCatProcess uses "cat" as a mock SSH subprocess to test
// the stdin/stdout message flow. "cat" echoes back everything sent to it,
// simulating a remote relay that reflects messages.
func TestSSHLink_WithCatProcess(t *testing.T) {
	link, err := NewSSHLinkFromCommand("cat", nil, "peer-remote")
	require.NoError(t, err)
	defer link.Close()

	assert.Equal(t, StatusConnected, link.Status())
	assert.Equal(t, "peer-remote", link.PeerID())

	env := Envelope{
		Type: MsgHeartbeat, ID: "echo-test", From: "peer-local",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	}

	err = link.Send(env)
	require.NoError(t, err)

	// "cat" echoes back the JSON line we sent, so we should receive it
	select {
	case received := <-link.Receive():
		assert.Equal(t, "echo-test", received.ID)
		assert.Equal(t, MsgHeartbeat, received.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for echoed message")
	}
}

func TestSSHLink_CloseTerminatesProcess(t *testing.T) {
	link, err := NewSSHLinkFromCommand("cat", nil, "peer-remote")
	require.NoError(t, err)

	err = link.Close()
	assert.NoError(t, err)
	assert.Equal(t, StatusDisconnected, link.Status())

	// Send after close should fail
	err = link.Send(Envelope{
		Type: MsgHeartbeat, ID: "late", From: "peer-local",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})
	assert.Error(t, err)
}
```

- [ ] **Step 2: Create minimal stubs**

Create `internal/transport/ssh_link.go`:

```go
package transport

// SSHLinkConfig holds configuration for an SSH link.
type SSHLinkConfig struct {
	RemoteUser   string
	RemoteHost   string
	RemotePort   int
	PeerID       string
	IdentityFile string
	UseAutossh   bool
}

// SSHLink is a link that uses SSH to run a remote relay subprocess.
type SSHLink struct{}

func (c *SSHLinkConfig) Validate() error                                      { return nil }
func (c *SSHLinkConfig) SSHArgs(relayCmd string) []string                     { return nil }
func NewSSHLink(cfg SSHLinkConfig) (*SSHLink, error)                           { return nil, nil }
func NewSSHLinkFromCommand(command string, args []string, peerID string) (*SSHLink, error) { return nil, nil }
func (sl *SSHLink) Send(env Envelope) error                                    { return nil }
func (sl *SSHLink) Receive() <-chan Envelope                                   { return nil }
func (sl *SSHLink) Close() error                                               { return nil }
func (sl *SSHLink) Status() LinkStatus                                         { return "" }
func (sl *SSHLink) PeerID() string                                             { return "" }
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -run TestSSHLink -v -count=1 2>&1 | head -30`
Expected: All tests FAIL.

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/transport/ssh_link.go internal/transport/ssh_link_test.go
git commit -m "test: add failing tests for SSH link with cat-based subprocess mock"
```

---

## Task 10: SSH Link -- Implementation

**Files:**
- Modify: `internal/transport/ssh_link.go`

- [ ] **Step 1: Implement SSHLink**

Replace `internal/transport/ssh_link.go`:

```go
package transport

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
)

// SSHLinkConfig holds configuration for an SSH link.
type SSHLinkConfig struct {
	RemoteUser   string // e.g., "deploy"
	RemoteHost   string // e.g., "server-01" or "phone"
	RemotePort   int    // e.g., 22 or 8022 (Termux)
	PeerID       string // remote peer identity
	IdentityFile string // path to SSH private key (optional)
	UseAutossh   bool   // use autossh instead of ssh
}

// Validate checks that required fields are set.
func (c *SSHLinkConfig) Validate() error {
	if c.RemoteHost == "" {
		return errors.New("ssh link: remote host is required")
	}
	if c.PeerID == "" {
		return errors.New("ssh link: peer ID is required")
	}
	return nil
}

// SSHArgs builds the argument list for the ssh command.
func (c *SSHLinkConfig) SSHArgs(relayCmd string) []string {
	args := []string{
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
	}

	if c.IdentityFile != "" {
		args = append(args, "-i", c.IdentityFile)
	}

	port := c.RemotePort
	if port == 0 {
		port = 22
	}
	if port != 22 {
		args = append(args, "-p", strconv.Itoa(port))
	}

	dest := c.RemoteHost
	if c.RemoteUser != "" {
		dest = c.RemoteUser + "@" + c.RemoteHost
	}
	args = append(args, dest)
	args = append(args, relayCmd)

	return args
}

// SSHLink is a link that uses SSH to run a remote relay subprocess.
// Messages are newline-delimited JSON over the subprocess stdin/stdout.
type SSHLink struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	peerID   string
	recvCh   chan Envelope
	status   LinkStatus
	closedCh chan struct{}
	closed   bool
}

// NewSSHLink creates a new SSH link by spawning an ssh subprocess.
func NewSSHLink(cfg SSHLinkConfig) (*SSHLink, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	command := "ssh"
	if cfg.UseAutossh {
		command = "autossh"
	}

	args := cfg.SSHArgs("agenthive relay")
	return NewSSHLinkFromCommand(command, args, cfg.PeerID)
}

// NewSSHLinkFromCommand creates a link from an arbitrary command.
// The command's stdin/stdout carry newline-delimited JSON messages.
// This is used by tests to inject "cat" or other mock commands.
func NewSSHLinkFromCommand(command string, args []string, peerID string) (*SSHLink, error) {
	cmd := exec.Command(command, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("ssh link stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("ssh link stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("ssh link start command: %w", err)
	}

	sl := &SSHLink{
		cmd:      cmd,
		stdin:    stdin,
		peerID:   peerID,
		recvCh:   make(chan Envelope, 64),
		status:   StatusConnected,
		closedCh: make(chan struct{}),
	}

	go sl.readLoop(stdout)
	go sl.waitLoop()

	return sl, nil
}

func (sl *SSHLink) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	// Allow large messages (up to 1 MB per line)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		var env Envelope
		if err := json.Unmarshal(scanner.Bytes(), &env); err != nil {
			continue // skip malformed lines
		}

		select {
		case sl.recvCh <- env:
		case <-sl.closedCh:
			return
		}
	}
}

func (sl *SSHLink) waitLoop() {
	sl.cmd.Wait()

	sl.mu.Lock()
	defer sl.mu.Unlock()
	if !sl.closed {
		sl.status = StatusError
	}
}

func (sl *SSHLink) Send(env Envelope) error {
	sl.mu.Lock()
	if sl.closed {
		sl.mu.Unlock()
		return errors.New("link closed")
	}
	sl.mu.Unlock()

	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	// Newline-delimited: append \n
	data = append(data, '\n')

	_, err = sl.stdin.Write(data)
	if err != nil {
		return fmt.Errorf("ssh link write: %w", err)
	}

	return nil
}

func (sl *SSHLink) Receive() <-chan Envelope {
	return sl.recvCh
}

func (sl *SSHLink) Close() error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	if sl.closed {
		return nil
	}
	sl.closed = true
	sl.status = StatusDisconnected
	close(sl.closedCh)

	sl.stdin.Close()
	// Kill the subprocess if it is still running
	if sl.cmd.Process != nil {
		sl.cmd.Process.Kill()
	}
	return nil
}

func (sl *SSHLink) Status() LinkStatus {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.status
}

func (sl *SSHLink) PeerID() string {
	return sl.peerID
}

// Compile-time interface check.
var _ Link = (*SSHLink)(nil)
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -run TestSSHLink -v -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/transport/ssh_link.go
git commit -m "feat: implement SSH link with subprocess stdin/stdout message transport"
```

---

## Task 11: Link Manager -- Failing Tests

**Files:**
- Create: `internal/transport/link_manager.go`
- Create: `internal/transport/link_manager_test.go`

The LinkManager holds all active links, broadcasts outbound messages to all (or targeted) links, and aggregates inbound messages from all links into a single channel. It monitors link health via heartbeats and tracks link status.

- [ ] **Step 1: Write the failing tests**

Create `internal/transport/link_manager_test.go`:

```go
package transport

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinkManager_AddAndRemoveLink(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	linkA, _ := NewPipeLinkPair("peer-local", "peer-a")
	defer linkA.Close()

	lm.AddLink(linkA)
	assert.Equal(t, 1, lm.LinkCount())

	peers := lm.ConnectedPeers()
	assert.Contains(t, peers, "peer-a")

	lm.RemoveLink("peer-a")
	assert.Equal(t, 0, lm.LinkCount())
}

func TestLinkManager_Broadcast(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	// Create two peer links
	linkA, remoteA := NewPipeLinkPair("peer-local", "peer-a")
	linkB, remoteB := NewPipeLinkPair("peer-local", "peer-b")
	defer linkA.Close()
	defer linkB.Close()
	defer remoteA.Close()
	defer remoteB.Close()

	lm.AddLink(linkA)
	lm.AddLink(linkB)

	env := Envelope{
		Type: MsgNotification, ID: "broadcast-001", From: "peer-local",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{"msg":"hello all"}`),
	}

	err := lm.Broadcast(env)
	require.NoError(t, err)

	// Both remote ends should receive the message
	select {
	case msg := <-remoteA.Receive():
		assert.Equal(t, "broadcast-001", msg.ID)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout: remote A did not receive broadcast")
	}

	select {
	case msg := <-remoteB.Receive():
		assert.Equal(t, "broadcast-001", msg.ID)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout: remote B did not receive broadcast")
	}
}

func TestLinkManager_SendTo(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	linkA, remoteA := NewPipeLinkPair("peer-local", "peer-a")
	linkB, remoteB := NewPipeLinkPair("peer-local", "peer-b")
	defer linkA.Close()
	defer linkB.Close()
	defer remoteA.Close()
	defer remoteB.Close()

	lm.AddLink(linkA)
	lm.AddLink(linkB)

	env := Envelope{
		Type: MsgActionResponse, ID: "targeted-001", From: "peer-local",
		To: "peer-a", Timestamp: time.Now().UTC(),
		Payload: json.RawMessage(`{"action":"allow"}`),
	}

	err := lm.SendTo("peer-a", env)
	require.NoError(t, err)

	// Only remote A should receive it
	select {
	case msg := <-remoteA.Receive():
		assert.Equal(t, "targeted-001", msg.ID)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout: remote A did not receive targeted message")
	}

	// Remote B should NOT receive it
	select {
	case <-remoteB.Receive():
		t.Fatal("remote B should not have received the targeted message")
	case <-time.After(100 * time.Millisecond):
		// Good -- nothing received
	}
}

func TestLinkManager_SendTo_UnknownPeer(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	err := lm.SendTo("nonexistent", Envelope{
		Type: MsgHeartbeat, ID: "lost", From: "peer-local",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})
	assert.Error(t, err)
}

func TestLinkManager_Inbound(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	linkA, remoteA := NewPipeLinkPair("peer-local", "peer-a")
	linkB, remoteB := NewPipeLinkPair("peer-local", "peer-b")
	defer linkA.Close()
	defer linkB.Close()
	defer remoteA.Close()
	defer remoteB.Close()

	lm.AddLink(linkA)
	lm.AddLink(linkB)

	// Send from remote A
	remoteA.Send(Envelope{
		Type: MsgNotification, ID: "from-a", From: "peer-a",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})

	// Send from remote B
	remoteB.Send(Envelope{
		Type: MsgNotification, ID: "from-b", From: "peer-b",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})

	// Both should arrive on the aggregated inbound channel
	received := make(map[string]bool)
	inbound := lm.Inbound()
	for i := 0; i < 2; i++ {
		select {
		case msg := <-inbound:
			received[msg.ID] = true
		case <-time.After(1 * time.Second):
			t.Fatalf("timeout waiting for inbound message %d", i)
		}
	}
	assert.True(t, received["from-a"])
	assert.True(t, received["from-b"])
}

func TestLinkManager_GetLinkStatus(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	linkA, _ := NewPipeLinkPair("peer-local", "peer-a")
	defer linkA.Close()

	lm.AddLink(linkA)

	status, ok := lm.GetLinkStatus("peer-a")
	assert.True(t, ok)
	assert.Equal(t, StatusConnected, status)

	_, ok = lm.GetLinkStatus("nonexistent")
	assert.False(t, ok)
}

func TestLinkManager_Close_ClosesAllLinks(t *testing.T) {
	lm := NewLinkManager("peer-local")

	linkA, _ := NewPipeLinkPair("peer-local", "peer-a")
	linkB, _ := NewPipeLinkPair("peer-local", "peer-b")

	lm.AddLink(linkA)
	lm.AddLink(linkB)

	err := lm.Close()
	assert.NoError(t, err)

	assert.Equal(t, StatusDisconnected, linkA.Status())
	assert.Equal(t, StatusDisconnected, linkB.Status())
}

func TestLinkManager_AddLink_DuplicatePeerReplacesOld(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	linkA1, _ := NewPipeLinkPair("peer-local", "peer-a")
	linkA2, _ := NewPipeLinkPair("peer-local", "peer-a")

	lm.AddLink(linkA1)
	lm.AddLink(linkA2) // should replace linkA1

	assert.Equal(t, 1, lm.LinkCount())
	// Old link should be closed
	assert.Equal(t, StatusDisconnected, linkA1.Status())
}

func TestLinkManager_BroadcastToNone(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	// Broadcasting with no links should not error
	err := lm.Broadcast(Envelope{
		Type: MsgHeartbeat, ID: "lonely", From: "peer-local",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})
	assert.NoError(t, err)
}
```

- [ ] **Step 2: Create minimal stubs**

Create `internal/transport/link_manager.go`:

```go
package transport

// LinkManager manages active links, broadcasts outbound messages,
// and aggregates inbound messages from all links.
type LinkManager struct{}

func NewLinkManager(localPeerID string) *LinkManager                        { return nil }
func (lm *LinkManager) AddLink(link Link)                                   {}
func (lm *LinkManager) RemoveLink(peerID string)                            {}
func (lm *LinkManager) LinkCount() int                                      { return 0 }
func (lm *LinkManager) ConnectedPeers() []string                            { return nil }
func (lm *LinkManager) Broadcast(env Envelope) error                        { return nil }
func (lm *LinkManager) SendTo(peerID string, env Envelope) error            { return nil }
func (lm *LinkManager) Inbound() <-chan Envelope                             { return nil }
func (lm *LinkManager) GetLinkStatus(peerID string) (LinkStatus, bool)      { return "", false }
func (lm *LinkManager) Close() error                                         { return nil }
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -run TestLinkManager -v -count=1 2>&1 | head -30`
Expected: All tests FAIL.

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/transport/link_manager.go internal/transport/link_manager_test.go
git commit -m "test: add failing tests for LinkManager broadcast, routing, and aggregation"
```

---

## Task 12: Link Manager -- Implementation

**Files:**
- Modify: `internal/transport/link_manager.go`

- [ ] **Step 1: Implement LinkManager**

Replace `internal/transport/link_manager.go`:

```go
package transport

import (
	"fmt"
	"sort"
	"sync"
)

// LinkManager manages active links, broadcasts outbound messages,
// and aggregates inbound messages from all links into a single channel.
// Safe for concurrent use.
type LinkManager struct {
	mu          sync.RWMutex
	localPeerID string
	links       map[string]Link          // peerID -> Link
	inbound     chan Envelope              // aggregated inbound from all links
	closedCh    chan struct{}
	closed      bool
	wg          sync.WaitGroup
}

// NewLinkManager creates a new LinkManager for the given local peer.
func NewLinkManager(localPeerID string) *LinkManager {
	return &LinkManager{
		localPeerID: localPeerID,
		links:       make(map[string]Link),
		inbound:     make(chan Envelope, 256),
		closedCh:    make(chan struct{}),
	}
}

// AddLink registers a new link. If a link to the same peer already exists,
// the old link is closed and replaced.
func (lm *LinkManager) AddLink(link Link) {
	peerID := link.PeerID()

	lm.mu.Lock()
	if old, exists := lm.links[peerID]; exists {
		old.Close()
	}
	lm.links[peerID] = link
	lm.mu.Unlock()

	// Start forwarding inbound messages from this link
	lm.wg.Add(1)
	go lm.forwardInbound(link)
}

func (lm *LinkManager) forwardInbound(link Link) {
	defer lm.wg.Done()

	recvCh := link.Receive()
	for {
		select {
		case env, ok := <-recvCh:
			if !ok {
				return
			}
			select {
			case lm.inbound <- env:
			case <-lm.closedCh:
				return
			}
		case <-lm.closedCh:
			return
		}
	}
}

// RemoveLink closes and removes the link to the specified peer.
func (lm *LinkManager) RemoveLink(peerID string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if link, exists := lm.links[peerID]; exists {
		link.Close()
		delete(lm.links, peerID)
	}
}

// LinkCount returns the number of active links.
func (lm *LinkManager) LinkCount() int {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return len(lm.links)
}

// ConnectedPeers returns the peer IDs of all active links, sorted.
func (lm *LinkManager) ConnectedPeers() []string {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	peers := make([]string, 0, len(lm.links))
	for pid := range lm.links {
		peers = append(peers, pid)
	}
	sort.Strings(peers)
	return peers
}

// Broadcast sends an envelope to all connected peers.
// Returns nil even if no links exist (broadcast to empty set is valid).
// Collects errors but does not stop on first failure.
func (lm *LinkManager) Broadcast(env Envelope) error {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	var lastErr error
	for _, link := range lm.links {
		if err := link.Send(env); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// SendTo sends an envelope to a specific peer.
// Returns an error if the peer is not connected.
func (lm *LinkManager) SendTo(peerID string, env Envelope) error {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	link, exists := lm.links[peerID]
	if !exists {
		return fmt.Errorf("no link to peer %q", peerID)
	}
	return link.Send(env)
}

// Inbound returns the channel that aggregates all inbound messages
// from all connected links.
func (lm *LinkManager) Inbound() <-chan Envelope {
	return lm.inbound
}

// GetLinkStatus returns the status of the link to the specified peer.
func (lm *LinkManager) GetLinkStatus(peerID string) (LinkStatus, bool) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	link, exists := lm.links[peerID]
	if !exists {
		return "", false
	}
	return link.Status(), true
}

// Close closes all links and stops the link manager.
func (lm *LinkManager) Close() error {
	lm.mu.Lock()
	if lm.closed {
		lm.mu.Unlock()
		return nil
	}
	lm.closed = true
	close(lm.closedCh)

	for _, link := range lm.links {
		link.Close()
	}
	lm.links = make(map[string]Link)
	lm.mu.Unlock()

	lm.wg.Wait()
	return nil
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -run TestLinkManager -v -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/transport/link_manager.go
git commit -m "feat: implement LinkManager with broadcast, targeted send, and inbound aggregation"
```

---

## Task 13: Peer Pairing -- Failing Tests

**Files:**
- Create: `internal/transport/pairing.go`
- Create: `internal/transport/pairing_test.go`

The pairing ceremony generates an Ed25519 peer identity and exchanges public keys with a remote peer. It produces a `PeerIdentity` (stored locally) and a `PairedPeer` (the remote peer's public key and metadata). This subsystem does NOT perform the actual SSH connection for key exchange -- it generates the identities and serializes them. The SSH-based exchange is handled by the CLI layer.

- [ ] **Step 1: Write the failing tests**

Create `internal/transport/pairing_test.go`:

```go
package transport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePeerIdentity(t *testing.T) {
	identity, err := GeneratePeerIdentity("my-server")
	require.NoError(t, err)

	assert.Equal(t, "my-server", identity.Name)
	assert.NotEmpty(t, identity.PeerID)
	assert.Len(t, identity.PublicKey, 32)
	assert.Len(t, identity.PrivateKey, 64)
	assert.NotEmpty(t, identity.NoiseKeypair.Public)
	assert.NotEmpty(t, identity.NoiseKeypair.Private)
}

func TestGeneratePeerIdentity_UniqueIDs(t *testing.T) {
	id1, _ := GeneratePeerIdentity("peer-1")
	id2, _ := GeneratePeerIdentity("peer-2")

	assert.NotEqual(t, id1.PeerID, id2.PeerID)
	assert.NotEqual(t, id1.PublicKey, id2.PublicKey)
}

func TestPeerIdentity_JSONRoundTrip(t *testing.T) {
	identity, _ := GeneratePeerIdentity("test-peer")

	data, err := json.Marshal(identity)
	require.NoError(t, err)

	var loaded PeerIdentity
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)

	assert.Equal(t, identity.PeerID, loaded.PeerID)
	assert.Equal(t, identity.Name, loaded.Name)
	assert.Equal(t, identity.PublicKey, loaded.PublicKey)
	assert.Equal(t, identity.PrivateKey, loaded.PrivateKey)
}

func TestPeerIdentity_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.json")

	identity, _ := GeneratePeerIdentity("test-peer")
	err := identity.SaveToFile(path)
	require.NoError(t, err)

	loaded, err := LoadPeerIdentity(path)
	require.NoError(t, err)

	assert.Equal(t, identity.PeerID, loaded.PeerID)
	assert.Equal(t, identity.Name, loaded.Name)
	assert.Equal(t, identity.PublicKey, loaded.PublicKey)
}

func TestPeerIdentity_SaveCreatesFileWithRestrictedPerms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.json")

	identity, _ := GeneratePeerIdentity("test-peer")
	identity.SaveToFile(path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	// File should be owner-only (0600)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestLoadPeerIdentity_NonexistentReturnsError(t *testing.T) {
	_, err := LoadPeerIdentity("/tmp/nonexistent-agenthive-identity.json")
	assert.Error(t, err)
}

func TestPairedPeer_FromIdentity(t *testing.T) {
	identity, _ := GeneratePeerIdentity("remote-server")

	paired := PairedPeerFromIdentity(identity)
	assert.Equal(t, identity.PeerID, paired.PeerID)
	assert.Equal(t, identity.Name, paired.Name)
	assert.Equal(t, identity.PublicKey, paired.PublicKey)
	assert.Equal(t, identity.NoiseKeypair.Public, paired.NoisePublicKey)
	// Must NOT contain the private key
	assert.Empty(t, paired.noisePrivateKey)
}

func TestPairedPeer_JSONRoundTrip(t *testing.T) {
	identity, _ := GeneratePeerIdentity("remote")
	paired := PairedPeerFromIdentity(identity)
	paired.Addr = "10.0.0.5:19222"
	paired.LinkType = "ssh"

	data, err := json.Marshal(paired)
	require.NoError(t, err)

	// Must not contain private key in JSON
	assert.NotContains(t, string(data), "private")

	var loaded PairedPeer
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)

	assert.Equal(t, paired.PeerID, loaded.PeerID)
	assert.Equal(t, paired.Addr, loaded.Addr)
	assert.Equal(t, paired.LinkType, loaded.LinkType)
}

func TestPeerStore_AddAndGet(t *testing.T) {
	dir := t.TempDir()
	store := NewPeerStore(filepath.Join(dir, "peers.json"))

	identity, _ := GeneratePeerIdentity("remote")
	paired := PairedPeerFromIdentity(identity)
	paired.Addr = "10.0.0.5:19222"

	store.Add(paired)

	got, ok := store.Get(paired.PeerID)
	assert.True(t, ok)
	assert.Equal(t, "10.0.0.5:19222", got.Addr)
}

func TestPeerStore_List(t *testing.T) {
	dir := t.TempDir()
	store := NewPeerStore(filepath.Join(dir, "peers.json"))

	id1, _ := GeneratePeerIdentity("peer-1")
	id2, _ := GeneratePeerIdentity("peer-2")

	store.Add(PairedPeerFromIdentity(id1))
	store.Add(PairedPeerFromIdentity(id2))

	peers := store.List()
	assert.Len(t, peers, 2)
}

func TestPeerStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peers.json")

	store := NewPeerStore(path)
	identity, _ := GeneratePeerIdentity("remote")
	paired := PairedPeerFromIdentity(identity)
	paired.Addr = "10.0.0.5:19222"
	store.Add(paired)

	err := store.Save()
	require.NoError(t, err)

	store2 := NewPeerStore(path)
	err = store2.Load()
	require.NoError(t, err)

	got, ok := store2.Get(paired.PeerID)
	assert.True(t, ok)
	assert.Equal(t, "10.0.0.5:19222", got.Addr)
}

func TestPeerStore_Remove(t *testing.T) {
	dir := t.TempDir()
	store := NewPeerStore(filepath.Join(dir, "peers.json"))

	identity, _ := GeneratePeerIdentity("remote")
	paired := PairedPeerFromIdentity(identity)
	store.Add(paired)

	store.Remove(paired.PeerID)

	_, ok := store.Get(paired.PeerID)
	assert.False(t, ok)
}
```

- [ ] **Step 2: Create minimal stubs**

Create `internal/transport/pairing.go`:

```go
package transport

// PeerIdentity holds this peer's cryptographic identity.
type PeerIdentity struct {
	PeerID       string        `json:"peer_id"`
	Name         string        `json:"name"`
	PublicKey    []byte        `json:"public_key"`
	PrivateKey   []byte        `json:"private_key"`
	NoiseKeypair *NoiseKeypair `json:"noise_keypair"`
}

// PairedPeer holds a remote peer's public identity (no private keys).
type PairedPeer struct {
	PeerID        string `json:"peer_id"`
	Name          string `json:"name"`
	PublicKey     []byte `json:"public_key"`
	NoisePublicKey []byte `json:"noise_public_key"`
	Addr          string `json:"addr,omitempty"`
	LinkType      string `json:"link_type,omitempty"`
	noisePrivateKey []byte // unexported, must never be set from external identity
}

// PeerStore manages the list of known paired peers.
type PeerStore struct{}

func GeneratePeerIdentity(name string) (*PeerIdentity, error)  { return nil, nil }
func (pi *PeerIdentity) SaveToFile(path string) error           { return nil }
func LoadPeerIdentity(path string) (*PeerIdentity, error)       { return nil, nil }
func PairedPeerFromIdentity(identity *PeerIdentity) PairedPeer  { return PairedPeer{} }
func NewPeerStore(path string) *PeerStore                        { return nil }
func (ps *PeerStore) Add(peer PairedPeer)                        {}
func (ps *PeerStore) Get(peerID string) (PairedPeer, bool)       { return PairedPeer{}, false }
func (ps *PeerStore) Remove(peerID string)                       {}
func (ps *PeerStore) List() []PairedPeer                          { return nil }
func (ps *PeerStore) Save() error                                 { return nil }
func (ps *PeerStore) Load() error                                  { return nil }
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -run TestGeneratePeerIdentity -v -count=1 2>&1 | head -20`
Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -run TestPeerStore -v -count=1 2>&1 | head -20`
Expected: All tests FAIL.

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/transport/pairing.go internal/transport/pairing_test.go
git commit -m "test: add failing tests for peer identity generation and paired peer store"
```

---

## Task 14: Peer Pairing -- Implementation

**Files:**
- Modify: `internal/transport/pairing.go`

- [ ] **Step 1: Implement pairing**

Replace `internal/transport/pairing.go`:

```go
package transport

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// PeerIdentity holds this peer's cryptographic identity.
// Contains private keys -- must be stored securely (0600 permissions).
type PeerIdentity struct {
	PeerID       string        `json:"peer_id"`
	Name         string        `json:"name"`
	PublicKey    []byte        `json:"public_key"`
	PrivateKey   []byte        `json:"private_key"`
	NoiseKeypair *NoiseKeypair `json:"noise_keypair"`
}

// PairedPeer holds a remote peer's public identity (no private keys).
type PairedPeer struct {
	PeerID         string `json:"peer_id"`
	Name           string `json:"name"`
	PublicKey      []byte `json:"public_key"`
	NoisePublicKey []byte `json:"noise_public_key"`
	Addr           string `json:"addr,omitempty"`
	LinkType       string `json:"link_type,omitempty"`
	noisePrivateKey []byte // unexported, never serialized, never set from external
}

// GeneratePeerIdentity creates a new peer identity with Ed25519 and Noise keypairs.
func GeneratePeerIdentity(name string) (*PeerIdentity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}

	noiseKP, err := GenerateNoiseKeypair()
	if err != nil {
		return nil, fmt.Errorf("generate noise keypair: %w", err)
	}

	// PeerID is the hex-encoded first 16 bytes of the Ed25519 public key
	peerID := hex.EncodeToString(pub[:16])

	return &PeerIdentity{
		PeerID:       peerID,
		Name:         name,
		PublicKey:    pub,
		PrivateKey:   priv,
		NoiseKeypair: noiseKP,
	}, nil
}

// SaveToFile writes the identity to a JSON file with 0600 permissions.
func (pi *PeerIdentity) SaveToFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create identity dir: %w", err)
	}

	data, err := json.MarshalIndent(pi, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal identity: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write identity: %w", err)
	}
	return os.Rename(tmp, path)
}

// LoadPeerIdentity reads a PeerIdentity from a JSON file.
func LoadPeerIdentity(path string) (*PeerIdentity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read identity: %w", err)
	}

	var pi PeerIdentity
	if err := json.Unmarshal(data, &pi); err != nil {
		return nil, fmt.Errorf("unmarshal identity: %w", err)
	}
	return &pi, nil
}

// PairedPeerFromIdentity creates a PairedPeer from a PeerIdentity,
// containing only public keys.
func PairedPeerFromIdentity(identity *PeerIdentity) PairedPeer {
	return PairedPeer{
		PeerID:         identity.PeerID,
		Name:           identity.Name,
		PublicKey:      identity.PublicKey,
		NoisePublicKey: identity.NoiseKeypair.Public,
	}
}

// PeerStore manages the list of known paired peers.
// Safe for concurrent use.
type PeerStore struct {
	mu    sync.RWMutex
	path  string
	peers map[string]PairedPeer
}

// NewPeerStore creates a new PeerStore backed by the given file path.
func NewPeerStore(path string) *PeerStore {
	return &PeerStore{
		path:  path,
		peers: make(map[string]PairedPeer),
	}
}

func (ps *PeerStore) Add(peer PairedPeer) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.peers[peer.PeerID] = peer
}

func (ps *PeerStore) Get(peerID string) (PairedPeer, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	peer, ok := ps.peers[peerID]
	return peer, ok
}

func (ps *PeerStore) Remove(peerID string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.peers, peerID)
}

func (ps *PeerStore) List() []PairedPeer {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make([]PairedPeer, 0, len(ps.peers))
	for _, p := range ps.peers {
		result = append(result, p)
	}
	return result
}

func (ps *PeerStore) Save() error {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	dir := filepath.Dir(ps.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create peers dir: %w", err)
	}

	data, err := json.MarshalIndent(ps.peers, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal peers: %w", err)
	}

	tmp := ps.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write peers: %w", err)
	}
	return os.Rename(tmp, ps.path)
}

func (ps *PeerStore) Load() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	data, err := os.ReadFile(ps.path)
	if err != nil {
		return fmt.Errorf("read peers: %w", err)
	}

	peers := make(map[string]PairedPeer)
	if err := json.Unmarshal(data, &peers); err != nil {
		return fmt.Errorf("unmarshal peers: %w", err)
	}

	ps.peers = peers
	return nil
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/transport/ -run "TestGeneratePeerIdentity|TestPeerIdentity|TestPairedPeer|TestPeerStore|TestLoadPeerIdentity" -v -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/transport/pairing.go
git commit -m "feat: implement peer identity generation, pairing, and peer store"
```

---

## Task 15: Run Full Test Suite and Verify

**Files:** None (verification only)

- [ ] **Step 1: Run all transport tests with race detector**

Run: `cd /home/devsupreme/agenthive && go test -race -v -count=1 ./internal/transport/...`
Expected: All tests PASS. Zero race conditions.

- [ ] **Step 2: Verify all Link implementations satisfy the interface**

Run: `cd /home/devsupreme/agenthive && go vet ./internal/transport/...`
Expected: No issues.

- [ ] **Step 3: Check test coverage**

Run: `cd /home/devsupreme/agenthive && go test -race -coverprofile=transport_coverage.out ./internal/transport/ && go tool cover -func=transport_coverage.out | tail -1`
Expected: Coverage > 75% for the transport package.

- [ ] **Step 4: Run both packages together (verify no conflicts with crdt package)**

Run: `cd /home/devsupreme/agenthive && go test -race -v -count=1 ./internal/...`
Expected: All tests across both `crdt` and `transport` packages PASS.

- [ ] **Step 5: Commit coverage config**

```bash
echo "transport_coverage.out" >> /home/devsupreme/agenthive/.gitignore
git add .gitignore
git commit -m "chore: add transport coverage output to gitignore"
```

---

## Summary

| Task | Component | Type | Est. Time |
|------|-----------|------|-----------|
| 1 | Link interface + Envelope failing tests | Test | 4 min |
| 2 | Link interface + Envelope implementation | Code | 2 min |
| 3 | PipeLink (test double) failing tests | Test | 4 min |
| 4 | PipeLink implementation | Code | 4 min |
| 5 | Noise Protocol failing tests | Test | 5 min |
| 6 | Noise Protocol implementation | Code | 5 min |
| 7 | TCP Link failing tests | Test | 5 min |
| 8 | TCP Link implementation | Code | 5 min |
| 9 | SSH Link failing tests | Test | 5 min |
| 10 | SSH Link implementation | Code | 5 min |
| 11 | Link Manager failing tests | Test | 5 min |
| 12 | Link Manager implementation | Code | 5 min |
| 13 | Peer Pairing failing tests | Test | 5 min |
| 14 | Peer Pairing implementation | Code | 5 min |
| 15 | Full suite verification | Verify | 5 min |
| | **Total** | | **~69 min** |
