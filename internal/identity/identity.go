package identity

// Identity holds an Ed25519 key pair and peer metadata.
type Identity struct {
	Name       string `json:"name"`
	PeerID     string `json:"peer_id"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

func Generate(name string) (*Identity, error)                  { return nil, nil }
func (id *Identity) Sign(message []byte) ([]byte, error)       { return nil, nil }
func Verify(publicKey string, message []byte, sig []byte) bool { return false }
func PeerIDFromPublicKey(publicKey string) string              { return "" }
func (id *Identity) SaveToFile(path string) error              { return nil }
func LoadFromFile(path string) (*Identity, error)              { return nil, nil }
