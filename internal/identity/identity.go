package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Identity holds an Ed25519 key pair and peer metadata.
type Identity struct {
	Name       string `json:"name"`
	PeerID     string `json:"peer_id"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

// Generate creates a new Ed25519 identity with the given name.
func Generate(name string) (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}

	pubB64 := base64.StdEncoding.EncodeToString(pub)
	privB64 := base64.StdEncoding.EncodeToString(priv)

	return &Identity{
		Name:       name,
		PeerID:     PeerIDFromPublicKey(pubB64),
		PublicKey:  pubB64,
		PrivateKey: privB64,
	}, nil
}

// PeerIDFromPublicKey derives a deterministic peer ID from a base64-encoded
// Ed25519 public key. The ID is the first 16 hex characters of the SHA-256
// hash of the raw public key bytes.
func PeerIDFromPublicKey(publicKey string) string {
	raw, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:8])
}

// Sign signs a message with the identity's private key.
func (id *Identity) Sign(message []byte) ([]byte, error) {
	privBytes, err := base64.StdEncoding.DecodeString(id.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	priv := ed25519.PrivateKey(privBytes)
	return ed25519.Sign(priv, message), nil
}

// Verify checks a signature against a base64-encoded public key.
func Verify(publicKey string, message []byte, sig []byte) bool {
	pubBytes, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil {
		return false
	}
	pub := ed25519.PublicKey(pubBytes)
	return ed25519.Verify(pub, message, sig)
}

// SaveToFile writes the identity to a JSON file with 0600 permissions.
func (id *Identity) SaveToFile(path string) error {
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal identity: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write identity: %w", err)
	}
	return os.Rename(tmp, path)
}

// LoadFromFile reads an identity from a JSON file.
func LoadFromFile(path string) (*Identity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read identity file: %w", err)
	}
	var id Identity
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, fmt.Errorf("unmarshal identity: %w", err)
	}
	return &id, nil
}
