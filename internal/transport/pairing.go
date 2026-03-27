package transport

// PeerIdentity holds this peer's cryptographic identity.
type PeerIdentity struct {
	PeerID       string        `json:"peer_id"`
	Name         string        `json:"name"`
	PublicKey    []byte        `json:"public_key"`
	PrivateKey   []byte        `json:"private_key"`
	NoiseKeypair *NoiseKeypair `json:"noise_keypair"`
}

// PairedPeer holds a remote peer's public identity (no private keys).
type PairedPeer struct {
	PeerID         string `json:"peer_id"`
	Name           string `json:"name"`
	PublicKey      []byte `json:"public_key"`
	NoisePublicKey []byte `json:"noise_public_key"`
	Addr           string `json:"addr,omitempty"`
	LinkType       string `json:"link_type,omitempty"`
	noisePrivateKey []byte // unexported, must never be set from external identity
}

// PeerStore manages the list of known paired peers.
type PeerStore struct{}

func GeneratePeerIdentity(name string) (*PeerIdentity, error)  { return nil, nil }
func (pi *PeerIdentity) SaveToFile(path string) error           { return nil }
func LoadPeerIdentity(path string) (*PeerIdentity, error)       { return nil, nil }
func PairedPeerFromIdentity(identity *PeerIdentity) PairedPeer  { return PairedPeer{} }
func NewPeerStore(path string) *PeerStore                        { return nil }
func (ps *PeerStore) Add(peer PairedPeer)                        {}
func (ps *PeerStore) Get(peerID string) (PairedPeer, bool)       { return PairedPeer{}, false }
func (ps *PeerStore) Remove(peerID string)                       {}
func (ps *PeerStore) List() []PairedPeer                          { return nil }
func (ps *PeerStore) Save() error                                 { return nil }
func (ps *PeerStore) Load() error                                  { return nil }
