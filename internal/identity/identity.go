// Package identity manages the daemon's persistent libp2p Ed25519 keypair.
//
// The keypair is written to <configDir>/identity.key with mode 0600 and the
// containing directory is created with mode 0700. Load returns
// (nil, os.ErrNotExist) when the key file does not yet exist.
package identity

import (
	"github.com/libp2p/go-libp2p/core/crypto"
)

// KeyFile is the on-disk file name of the persisted identity key.
const KeyFile = "identity.key"

// Generate creates a fresh Ed25519 keypair.
func Generate() (crypto.PrivKey, crypto.PubKey, error) {
	panic("not implemented: identity.Generate")
}

// Save persists priv to <configDir>/identity.key with mode 0600.
// configDir is created with mode 0700 if missing.
func Save(configDir string, priv crypto.PrivKey) error {
	panic("not implemented: identity.Save")
}

// Load reads <configDir>/identity.key and unmarshals it.
// Returns (nil, os.ErrNotExist) if missing.
func Load(configDir string) (crypto.PrivKey, error) {
	panic("not implemented: identity.Load")
}
