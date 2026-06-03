// Package identity manages the daemon's persistent libp2p Ed25519 keypair.
//
// The keypair is written to <configDir>/identity.key with mode 0600 and the
// containing directory is created with mode 0700. Load returns
// (nil, os.ErrNotExist) when the key file does not yet exist.
//
// The on-disk format is the raw output of crypto.MarshalPrivateKey (a libp2p
// protobuf envelope). Save writes atomically via a temp file + rename so a
// crash mid-write never leaves a half-written identity behind.
package identity

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// KeyFile is the on-disk file name of the persisted identity key.
const KeyFile = "identity.key"

// dirPerm is the mode used when creating the config directory.
const dirPerm os.FileMode = 0o700

// filePerm is the mode used when writing the key file.
const filePerm os.FileMode = 0o600

// Generate creates a fresh Ed25519 keypair using crypto/rand as the entropy
// source. Ed25519 is the libp2p-recommended default — short peer IDs, fast,
// no parameter choices to get wrong.
func Generate() (crypto.PrivKey, crypto.PubKey, error) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("identity: generate ed25519 key: %w", err)
	}
	return priv, pub, nil
}

// Save persists priv to <configDir>/identity.key with mode 0600.
// configDir is created with mode 0700 if missing.
//
// Writes are atomic: the marshaled key is written to a temp file in the same
// directory, fsynced, then renamed into place. An existing key file is
// overwritten.
func Save(configDir string, priv crypto.PrivKey) error {
	if priv == nil {
		return errors.New("identity: Save called with nil private key")
	}
	if configDir == "" {
		return errors.New("identity: Save called with empty configDir")
	}

	if err := os.MkdirAll(configDir, dirPerm); err != nil {
		return fmt.Errorf("identity: create config dir %q: %w", configDir, err)
	}
	// MkdirAll respects the umask, which on most systems would mask off
	// group/world bits anyway — but we want a hard guarantee of 0700 on the
	// final directory regardless of umask.
	if err := os.Chmod(configDir, dirPerm); err != nil {
		return fmt.Errorf("identity: chmod config dir %q: %w", configDir, err)
	}

	data, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("identity: marshal private key: %w", err)
	}

	finalPath := filepath.Join(configDir, KeyFile)
	tmp, err := os.CreateTemp(configDir, KeyFile+".tmp-*")
	if err != nil {
		return fmt.Errorf("identity: create temp key file: %w", err)
	}
	tmpPath := tmp.Name()

	// On any error past this point, clean up the temp file.
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}

	if err := tmp.Chmod(filePerm); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("identity: chmod temp key file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("identity: write temp key file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("identity: sync temp key file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("identity: close temp key file: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		cleanup()
		return fmt.Errorf("identity: rename temp key file into place: %w", err)
	}
	return nil
}

// Load reads <configDir>/identity.key and unmarshals it into a libp2p
// PrivKey.
//
// Returns (nil, err) where errors.Is(err, os.ErrNotExist) is true when the
// key file (or its containing directory) does not exist. Any other error
// (corrupt file, permission denied, etc.) is returned wrapped.
func Load(configDir string) (crypto.PrivKey, error) {
	if configDir == "" {
		return nil, errors.New("identity: Load called with empty configDir")
	}
	path := filepath.Join(configDir, KeyFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Preserve os.ErrNotExist semantics for the caller. Wrap so the
			// path is visible while keeping errors.Is(err, os.ErrNotExist)
			// true.
			return nil, fmt.Errorf("identity: read key file %q: %w", path, err)
		}
		return nil, fmt.Errorf("identity: read key file %q: %w", path, err)
	}

	priv, err := crypto.UnmarshalPrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("identity: unmarshal private key from %q: %w", path, err)
	}
	return priv, nil
}
