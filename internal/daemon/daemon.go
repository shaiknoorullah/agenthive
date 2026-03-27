package daemon

import (
	"github.com/shaiknoorullah/agenthive/internal/crdt"
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
}

// Daemon is the central agenthive process.
type Daemon struct{}

func NewDaemon(cfg DaemonConfig) (*Daemon, error)         { return nil, nil }
func (d *Daemon) Start() error                             { return nil }
func (d *Daemon) Stop() error                              { return nil }
func (d *Daemon) WaitReady()                               {}
func (d *Daemon) Store() *crdt.StateStore                  { return nil }
func (d *Daemon) PeerID() string                           { return "" }
func DaemonStatus(cfg DaemonConfig) Status                 { return Status{} }
