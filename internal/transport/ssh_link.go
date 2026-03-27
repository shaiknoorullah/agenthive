package transport

// SSHLinkConfig holds configuration for an SSH link.
type SSHLinkConfig struct {
	RemoteUser   string
	RemoteHost   string
	RemotePort   int
	PeerID       string
	IdentityFile string
	UseAutossh   bool
}

// SSHLink is a link that uses SSH to run a remote relay subprocess.
type SSHLink struct{}

func (c *SSHLinkConfig) Validate() error                                      { return nil }
func (c *SSHLinkConfig) SSHArgs(relayCmd string) []string                     { return nil }
func NewSSHLink(cfg SSHLinkConfig) (*SSHLink, error)                           { return nil, nil }
func NewSSHLinkFromCommand(command string, args []string, peerID string) (*SSHLink, error) { return nil, nil }
func (sl *SSHLink) Send(env Envelope) error                                    { return nil }
func (sl *SSHLink) Receive() <-chan Envelope                                   { return nil }
func (sl *SSHLink) Close() error                                               { return nil }
func (sl *SSHLink) Status() LinkStatus                                         { return "" }
func (sl *SSHLink) PeerID() string                                             { return "" }
