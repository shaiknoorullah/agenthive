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
