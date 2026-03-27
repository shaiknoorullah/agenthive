package daemon

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/identity"
	"github.com/shaiknoorullah/agenthive/internal/protocol"
)

// DaemonConfig holds configuration for the daemon.
type DaemonConfig struct {
	ConfigDir string
	PeerName  string
}

// Status holds the current daemon status.
type Status struct {
	Running bool
	PID     int
	PeerID  string
}

// Daemon is the central agenthive process.
// It manages the CRDT state store, Unix socket listener, message router,
// message queue, and peer identity.
type Daemon struct {
	cfg      DaemonConfig
	store    *crdt.StateStore
	identity *identity.Identity
	socket   *SocketListener
	router   *Router
	queue    *Queue
	ready    chan struct{}
	stop     chan struct{}
	stopped  chan struct{}
	mu       sync.Mutex
}

// NewDaemon creates a new daemon instance.
func NewDaemon(cfg DaemonConfig) (*Daemon, error) {
	if err := os.MkdirAll(cfg.ConfigDir, 0700); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}

	return &Daemon{
		cfg:     cfg,
		ready:   make(chan struct{}),
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}, nil
}

// Start initializes and runs the daemon.
// Blocks until Stop is called or a signal is received.
func (d *Daemon) Start() error {
	defer close(d.stopped)

	// Load or create identity
	idPath := filepath.Join(d.cfg.ConfigDir, "identity.json")
	id, err := identity.LoadFromFile(idPath)
	if err != nil {
		id, err = identity.Generate(d.cfg.PeerName)
		if err != nil {
			return fmt.Errorf("generate identity: %w", err)
		}
		if err := id.SaveToFile(idPath); err != nil {
			return fmt.Errorf("save identity: %w", err)
		}
	}
	d.identity = id

	// Initialize CRDT state store
	d.store = crdt.NewStateStore(id.PeerID)
	statePath := filepath.Join(d.cfg.ConfigDir, "state.json")
	if err := d.store.LoadFromFile(statePath); err != nil {
		log.Printf("warning: could not load state: %v", err)
	}

	// Initialize message queue
	queueDir := filepath.Join(d.cfg.ConfigDir, "queue")
	d.queue, err = NewQueue(queueDir)
	if err != nil {
		return fmt.Errorf("create message queue: %w", err)
	}

	// Initialize router
	d.router = NewRouter(d.store, id.PeerID)

	// Start socket listener
	sockPath := filepath.Join(d.cfg.ConfigDir, "daemon.sock")
	d.socket, err = NewSocketListener(sockPath, d.handleMessage)
	if err != nil {
		return fmt.Errorf("create socket listener: %w", err)
	}

	// Write PID file
	pidPath := filepath.Join(d.cfg.ConfigDir, "daemon.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		_ = d.socket.Close() // best-effort cleanup
		return fmt.Errorf("write PID file: %w", err)
	}

	// Signal that we are ready
	close(d.ready)

	// Serve socket in background
	go func() {
		if err := d.socket.Serve(); err != nil {
			log.Printf("socket serve error: %v", err)
		}
	}()

	// Wait for stop signal
	<-d.stop

	return nil
}

// Stop gracefully shuts down the daemon.
func (d *Daemon) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Signal the main loop to stop
	select {
	case <-d.stop:
		// already stopped
		return nil
	default:
		close(d.stop)
	}

	// Wait for Start to return
	<-d.stopped

	// Close socket listener
	if d.socket != nil {
		if err := d.socket.Close(); err != nil {
			log.Printf("warning: could not close socket: %v", err)
		}
	}

	// Save CRDT state
	if d.store != nil {
		statePath := filepath.Join(d.cfg.ConfigDir, "state.json")
		if err := d.store.SaveToFile(statePath); err != nil {
			log.Printf("warning: could not save state: %v", err)
		}
	}

	// Remove PID file
	pidPath := filepath.Join(d.cfg.ConfigDir, "daemon.pid")
	_ = os.Remove(pidPath) // best-effort cleanup

	return nil
}

// WaitReady blocks until the daemon is initialized and accepting connections.
func (d *Daemon) WaitReady() {
	<-d.ready
}

// Store returns the CRDT state store.
func (d *Daemon) Store() *crdt.StateStore {
	return d.store
}

// PeerID returns the daemon's peer identity ID.
func (d *Daemon) PeerID() string {
	if d.identity == nil {
		return ""
	}
	return d.identity.PeerID
}

// handleMessage processes a message received from the socket.
func (d *Daemon) handleMessage(msg protocol.Message) {
	// Route the message to target peers
	targets := d.router.Route(msg)

	for _, target := range targets {
		// Check if peer is online (has a link)
		peer, ok := d.store.GetPeer(target)
		if !ok || peer.Status != "online" {
			// Queue for offline peer
			if err := d.queue.Enqueue(target, msg); err != nil {
				log.Printf("error queuing message for %s: %v", target, err)
			}
			continue
		}

		// TODO: forward to link manager (implemented in transport subsystem)
		// For now, queue the message
		if err := d.queue.Enqueue(target, msg); err != nil {
			log.Printf("error queuing message for %s: %v", target, err)
		}
	}
}

// DaemonStatus checks whether a daemon is running for the given config.
func DaemonStatus(cfg DaemonConfig) Status {
	pidPath := filepath.Join(cfg.ConfigDir, "daemon.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return Status{Running: false}
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return Status{Running: false}
	}

	// Check if process is alive
	proc, err := os.FindProcess(pid)
	if err != nil {
		return Status{Running: false}
	}

	// On Unix, FindProcess always succeeds. Send signal 0 to check if alive.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// Process is not running, clean up stale PID file
		_ = os.Remove(pidPath) // best-effort cleanup
		return Status{Running: false}
	}

	return Status{Running: true, PID: pid}
}
