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
