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
