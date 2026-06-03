package identity_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	pb "github.com/libp2p/go-libp2p/core/crypto/pb"
	"github.com/stretchr/testify/require"

	"github.com/shaiknoorullah/agenthive/internal/identity"
)

func TestGenerate_ProducesEd25519(t *testing.T) {
	t.Parallel()

	priv, pub, err := identity.Generate()
	require.NoError(t, err)
	require.NotNil(t, priv)
	require.NotNil(t, pub)

	require.Equal(t, pb.KeyType(crypto.Ed25519), priv.Type())
	require.Equal(t, pb.KeyType(crypto.Ed25519), pub.Type())

	// Public key matches the private key.
	require.True(t, priv.GetPublic().Equals(pub))
}

func TestSaveLoad_Roundtrip(t *testing.T) {
	t.Parallel()

	priv, _, err := identity.Generate()
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), "agenthive")
	require.NoError(t, identity.Save(dir, priv))

	loaded, err := identity.Load(dir)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Same key type and equal bytes.
	require.Equal(t, priv.Type(), loaded.Type())
	require.True(t, priv.Equals(loaded), "loaded key must equal saved key")

	origBytes, err := crypto.MarshalPrivateKey(priv)
	require.NoError(t, err)
	loadedBytes, err := crypto.MarshalPrivateKey(loaded)
	require.NoError(t, err)
	require.Equal(t, origBytes, loadedBytes)
}

func TestSave_DirectoryPermissionsAre0700(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes do not apply on Windows")
	}
	t.Parallel()

	priv, _, err := identity.Generate()
	require.NoError(t, err)

	// Ensure the directory does NOT exist yet so Save has to create it.
	dir := filepath.Join(t.TempDir(), "nested", "agenthive")
	require.NoError(t, identity.Save(dir, priv))

	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "config dir must be 0700")
}

func TestSave_FilePermissionsAre0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes do not apply on Windows")
	}
	t.Parallel()

	priv, _, err := identity.Generate()
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, identity.Save(dir, priv))

	info, err := os.Stat(filepath.Join(dir, identity.KeyFile))
	require.NoError(t, err)
	require.False(t, info.IsDir())
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "key file must be 0600")
}

func TestLoad_MissingReturnsNotExist(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "no-such-dir")
	priv, err := identity.Load(dir)
	require.Nil(t, priv)
	require.Error(t, err)
	require.True(t, errors.Is(err, os.ErrNotExist), "expected os.ErrNotExist, got %v", err)

	// Directory exists, but file does not.
	emptyDir := t.TempDir()
	priv, err = identity.Load(emptyDir)
	require.Nil(t, priv)
	require.Error(t, err)
	require.True(t, errors.Is(err, os.ErrNotExist), "expected os.ErrNotExist, got %v", err)
}

func TestSave_OverwritesExistingKey(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	first, _, err := identity.Generate()
	require.NoError(t, err)
	require.NoError(t, identity.Save(dir, first))

	second, _, err := identity.Generate()
	require.NoError(t, err)
	require.NoError(t, identity.Save(dir, second))

	loaded, err := identity.Load(dir)
	require.NoError(t, err)
	require.True(t, loaded.Equals(second))
	require.False(t, loaded.Equals(first))

	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, identity.KeyFile))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}
