package transport

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
)

// SSHLinkConfig holds configuration for an SSH link.
type SSHLinkConfig struct {
	RemoteUser   string // e.g., "deploy"
	RemoteHost   string // e.g., "server-01" or "phone"
	RemotePort   int    // e.g., 22 or 8022 (Termux)
	PeerID       string // remote peer identity
	IdentityFile string // path to SSH private key (optional)
	UseAutossh   bool   // use autossh instead of ssh
}

// Validate checks that required fields are set.
func (c *SSHLinkConfig) Validate() error {
	if c.RemoteHost == "" {
		return errors.New("ssh link: remote host is required")
	}
	if c.PeerID == "" {
		return errors.New("ssh link: peer ID is required")
	}
	return nil
}

// SSHArgs builds the argument list for the ssh command.
func (c *SSHLinkConfig) SSHArgs(relayCmd string) []string {
	args := []string{
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
	}

	if c.IdentityFile != "" {
		args = append(args, "-i", c.IdentityFile)
	}

	port := c.RemotePort
	if port == 0 {
		port = 22
	}
	if port != 22 {
		args = append(args, "-p", strconv.Itoa(port))
	}

	dest := c.RemoteHost
	if c.RemoteUser != "" {
		dest = c.RemoteUser + "@" + c.RemoteHost
	}
	args = append(args, dest, relayCmd)

	return args
}

// SSHLink is a link that uses SSH to run a remote relay subprocess.
// Messages are newline-delimited JSON over the subprocess stdin/stdout.
type SSHLink struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	peerID   string
	recvCh   chan Envelope
	status   LinkStatus
	closedCh chan struct{}
	closed   bool
}

// NewSSHLink creates a new SSH link by spawning an ssh subprocess.
func NewSSHLink(cfg SSHLinkConfig) (*SSHLink, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	command := "ssh"
	if cfg.UseAutossh {
		command = "autossh"
	}

	args := cfg.SSHArgs("agenthive relay")
	return NewSSHLinkFromCommand(command, args, cfg.PeerID)
}

// NewSSHLinkFromCommand creates a link from an arbitrary command.
// The command's stdin/stdout carry newline-delimited JSON messages.
// This is used by tests to inject "cat" or other mock commands.
func NewSSHLinkFromCommand(command string, args []string, peerID string) (*SSHLink, error) {
	cmd := exec.Command(command, args...) //nolint:noctx // long-lived subprocess managed via Close()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("ssh link stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("ssh link stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("ssh link start command: %w", err)
	}

	sl := &SSHLink{
		cmd:      cmd,
		stdin:    stdin,
		peerID:   peerID,
		recvCh:   make(chan Envelope, 64),
		status:   StatusConnected,
		closedCh: make(chan struct{}),
	}

	go sl.readLoop(stdout)
	go sl.waitLoop()

	return sl, nil
}

func (sl *SSHLink) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	// Allow large messages (up to 1 MB per line)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		var env Envelope
		if err := json.Unmarshal(scanner.Bytes(), &env); err != nil {
			continue // skip malformed lines
		}

		select {
		case sl.recvCh <- env:
		case <-sl.closedCh:
			return
		}
	}
}

func (sl *SSHLink) waitLoop() {
	_ = sl.cmd.Wait()

	sl.mu.Lock()
	defer sl.mu.Unlock()
	if !sl.closed {
		sl.status = StatusError
	}
}

func (sl *SSHLink) Send(env Envelope) error {
	sl.mu.Lock()
	if sl.closed {
		sl.mu.Unlock()
		return errors.New("link closed")
	}
	sl.mu.Unlock()

	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	// Newline-delimited: append \n
	data = append(data, '\n')

	_, err = sl.stdin.Write(data)
	if err != nil {
		return fmt.Errorf("ssh link write: %w", err)
	}

	return nil
}

func (sl *SSHLink) Receive() <-chan Envelope {
	return sl.recvCh
}

func (sl *SSHLink) Close() error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	if sl.closed {
		return nil
	}
	sl.closed = true
	sl.status = StatusDisconnected
	close(sl.closedCh)

	_ = sl.stdin.Close()
	// Kill the subprocess if it is still running
	if sl.cmd.Process != nil {
		_ = sl.cmd.Process.Kill()
	}
	return nil
}

func (sl *SSHLink) Status() LinkStatus {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.status
}

func (sl *SSHLink) PeerID() string {
	return sl.peerID
}

// Compile-time interface check.
var _ Link = (*SSHLink)(nil)
