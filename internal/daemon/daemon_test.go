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
	dir := t.TempDir()
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
	dir := t.TempDir()
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
	dir := t.TempDir()
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
	dir := t.TempDir()
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
	dir := t.TempDir()
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
