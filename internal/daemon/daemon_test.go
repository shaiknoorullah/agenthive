package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/protocol"
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
	dir := shortTempDir(t)
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	go func() {
		_ = d.Start() // error checked via WaitReady and Stop
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
	dir := shortTempDir(t)
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	go func() { _ = d.Start() }()
	d.WaitReady()
	defer func() { _ = d.Stop() }()

	status := DaemonStatus(cfg)
	assert.True(t, status.Running)
	assert.Equal(t, os.Getpid(), status.PID)
}

func TestDaemon_SavesStateOnStop(t *testing.T) {
	dir := shortTempDir(t)
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	go func() { _ = d.Start() }()
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
	dir := shortTempDir(t)
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	// First daemon: set config and stop
	d1, err := NewDaemon(cfg)
	require.NoError(t, err)
	go func() { _ = d1.Start() }()
	d1.WaitReady()
	d1.Store().SetConfig("persist-key", "persist-value")
	require.NoError(t, d1.Stop())

	// Second daemon: should load persisted state
	d2, err := NewDaemon(cfg)
	require.NoError(t, err)
	go func() { _ = d2.Start() }()
	d2.WaitReady()
	defer func() { _ = d2.Stop() }()

	val, ok := d2.Store().GetConfig("persist-key")
	assert.True(t, ok)
	assert.Equal(t, "persist-value", val)
}

func TestDaemon_StaleSocketCleaned(t *testing.T) {
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, "daemon.sock")

	// Create a stale socket file
	require.NoError(t, os.WriteFile(sockPath, []byte("stale"), 0600))

	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	go func() { _ = d.Start() }()
	d.WaitReady()
	defer func() { _ = d.Stop() }()

	// Daemon should have started despite stale socket
	status := DaemonStatus(cfg)
	assert.True(t, status.Running)
}

func TestDaemon_IdentityCreatedOnInit(t *testing.T) {
	dir := shortTempDir(t)
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	go func() { _ = d.Start() }()
	d.WaitReady()
	defer func() { _ = d.Stop() }()

	// Identity file should exist
	idPath := filepath.Join(dir, "identity.json")
	_, err = os.Stat(idPath)
	assert.NoError(t, err)

	assert.NotEmpty(t, d.PeerID())
}

func TestDaemon_PeerID_NilIdentity(t *testing.T) {
	dir := t.TempDir()
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	// Before Start(), identity is nil so PeerID should return ""
	assert.Equal(t, "", d.PeerID())
}

func TestDaemon_Status_GarbagePIDFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")
	require.NoError(t, os.WriteFile(pidPath, []byte("not-a-number"), 0600))

	cfg := DaemonConfig{ConfigDir: dir, PeerName: "test"}
	status := DaemonStatus(cfg)
	assert.False(t, status.Running)
}

func TestDaemon_Status_NonExistentPID(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")
	// Use a PID that almost certainly doesn't exist
	require.NoError(t, os.WriteFile(pidPath, []byte("4999999"), 0600))

	cfg := DaemonConfig{ConfigDir: dir, PeerName: "test"}
	status := DaemonStatus(cfg)
	assert.False(t, status.Running)
	// Stale PID file should be cleaned up
	_, err := os.Stat(pidPath)
	assert.True(t, os.IsNotExist(err))
}

func TestDaemon_HandleMessage_RoutesToOnlineAndOfflinePeers(t *testing.T) {
	dir := shortTempDir(t)
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	go func() { _ = d.Start() }()
	d.WaitReady()
	defer func() { _ = d.Stop() }()

	// Register peers: one online, one offline
	d.store.SetPeer("phone", &crdt.PeerInfo{Name: "phone", Status: "online"})
	d.store.SetPeer("tablet", &crdt.PeerInfo{Name: "tablet", Status: "offline"})

	// Add a route that matches everything and targets both peers
	d.store.SetRoute("catch-all", &crdt.RouteRule{
		Match:   crdt.RouteMatch{},
		Targets: []string{"phone", "tablet"},
	})

	msg := protocol.Message{
		ID:       "hm-1",
		Type:     protocol.MsgNotification,
		SourceID: d.PeerID(),
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "test",
		},
	}

	d.handleMessage(msg)

	// Both peers should have the message queued
	msgsPhone, err := d.queue.Drain("phone")
	require.NoError(t, err)
	assert.Len(t, msgsPhone, 1)
	assert.Equal(t, "hm-1", msgsPhone[0].ID)

	msgsTablet, err := d.queue.Drain("tablet")
	require.NoError(t, err)
	assert.Len(t, msgsTablet, 1)
	assert.Equal(t, "hm-1", msgsTablet[0].ID)
}

func TestDaemon_HandleMessage_NoMatchingRoutes(t *testing.T) {
	dir := shortTempDir(t)
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	go func() { _ = d.Start() }()
	d.WaitReady()
	defer func() { _ = d.Stop() }()

	// Route only matches project "api"
	d.store.SetRoute("r1", &crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api"},
		Targets: []string{"phone"},
	})

	msg := protocol.Message{
		ID:       "hm-2",
		Type:     protocol.MsgNotification,
		SourceID: d.PeerID(),
		Payload: protocol.NotificationPayload{
			Project: "frontend",
			Source:  "Claude",
			Message: "test",
		},
	}

	d.handleMessage(msg)

	// No messages should be queued
	msgs, err := d.queue.Drain("phone")
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestDaemon_HandleMessage_UnknownPeer(t *testing.T) {
	dir := shortTempDir(t)
	cfg := DaemonConfig{
		ConfigDir: dir,
		PeerName:  "test-peer",
	}

	d, err := NewDaemon(cfg)
	require.NoError(t, err)

	go func() { _ = d.Start() }()
	d.WaitReady()
	defer func() { _ = d.Stop() }()

	// Route targets a peer not in the store
	d.store.SetRoute("r1", &crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api"},
		Targets: []string{"ghost-peer"},
	})

	msg := protocol.Message{
		ID:       "hm-3",
		Type:     protocol.MsgNotification,
		SourceID: d.PeerID(),
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "test",
		},
	}

	d.handleMessage(msg)

	// Unknown peer -> store.GetPeer returns !ok -> queued for offline
	msgs, err := d.queue.Drain("ghost-peer")
	require.NoError(t, err)
	assert.Len(t, msgs, 1)
}
