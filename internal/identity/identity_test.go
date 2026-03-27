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
