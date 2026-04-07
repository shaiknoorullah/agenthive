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
		RemoteUser:   "deploy",
		RemoteHost:   "server-01",
		RemotePort:   22,
		PeerID:       "peer-server",
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
