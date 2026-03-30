package transport

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// PeerIdentity holds this peer's cryptographic identity.
// Contains private keys -- must be stored securely (0600 permissions).
type PeerIdentity struct {
	PeerID       string        `json:"peer_id"`
	Name         string        `json:"name"`
	PublicKey    []byte        `json:"public_key"`
	PrivateKey   []byte        `json:"private_key"`
	NoiseKeypair *NoiseKeypair `json:"noise_keypair"`
}

// PairedPeer holds a remote peer's public identity (no private keys).
type PairedPeer struct {
	PeerID          string `json:"peer_id"`
	Name            string `json:"name"`
	PublicKey       []byte `json:"public_key"`
	NoisePublicKey  []byte `json:"noise_public_key"`
	Addr            string `json:"addr,omitempty"`
	LinkType        string `json:"link_type,omitempty"`
	noisePrivateKey []byte // unexported, never serialized, never set from external
}

// GeneratePeerIdentity creates a new peer identity with Ed25519 and Noise keypairs.
func GeneratePeerIdentity(name string) (*PeerIdentity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}

	noiseKP, err := GenerateNoiseKeypair()
	if err != nil {
		return nil, fmt.Errorf("generate noise keypair: %w", err)
	}

	// PeerID is the hex-encoded first 16 bytes of the Ed25519 public key
	peerID := hex.EncodeToString(pub[:16])

	return &PeerIdentity{
		PeerID:       peerID,
		Name:         name,
		PublicKey:    pub,
		PrivateKey:   priv,
		NoiseKeypair: noiseKP,
	}, nil
}

// SaveToFile writes the identity to a JSON file with 0600 permissions.
func (pi *PeerIdentity) SaveToFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create identity dir: %w", err)
	}

	data, err := json.MarshalIndent(pi, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal identity: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write identity: %w", err)
	}
	return os.Rename(tmp, path)
}

// LoadPeerIdentity reads a PeerIdentity from a JSON file.
func LoadPeerIdentity(path string) (*PeerIdentity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read identity: %w", err)
	}

	var pi PeerIdentity
	if err := json.Unmarshal(data, &pi); err != nil {
		return nil, fmt.Errorf("unmarshal identity: %w", err)
	}
	return &pi, nil
}

// PairedPeerFromIdentity creates a PairedPeer from a PeerIdentity,
// containing only public keys.
func PairedPeerFromIdentity(identity *PeerIdentity) PairedPeer {
	return PairedPeer{
		PeerID:         identity.PeerID,
		Name:           identity.Name,
		PublicKey:      identity.PublicKey,
		NoisePublicKey: identity.NoiseKeypair.Public,
	}
}

// PeerStore manages the list of known paired peers.
// Safe for concurrent use.
type PeerStore struct {
	mu    sync.RWMutex
	path  string
	peers map[string]PairedPeer
}

// NewPeerStore creates a new PeerStore backed by the given file path.
func NewPeerStore(path string) *PeerStore {
	return &PeerStore{
		path:  path,
		peers: make(map[string]PairedPeer),
	}
}

func (ps *PeerStore) Add(peer PairedPeer) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.peers[peer.PeerID] = peer
}

func (ps *PeerStore) Get(peerID string) (PairedPeer, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	peer, ok := ps.peers[peerID]
	return peer, ok
}

func (ps *PeerStore) Remove(peerID string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.peers, peerID)
}

func (ps *PeerStore) List() []PairedPeer {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make([]PairedPeer, 0, len(ps.peers))
	for _, p := range ps.peers {
		result = append(result, p)
	}
	return result
}

func (ps *PeerStore) Save() error {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	dir := filepath.Dir(ps.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create peers dir: %w", err)
	}

	data, err := json.MarshalIndent(ps.peers, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal peers: %w", err)
	}

	tmp := ps.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write peers: %w", err)
	}
	return os.Rename(tmp, ps.path)
}

func (ps *PeerStore) Load() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	data, err := os.ReadFile(ps.path)
	if err != nil {
		return fmt.Errorf("read peers: %w", err)
	}

	peers := make(map[string]PairedPeer)
	if err := json.Unmarshal(data, &peers); err != nil {
		return fmt.Errorf("unmarshal peers: %w", err)
	}

	ps.peers = peers
	return nil
}
