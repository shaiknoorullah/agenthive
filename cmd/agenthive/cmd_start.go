package main

import (
	"github.com/spf13/cobra"
)

// newStartCmd returns `agenthive start`, which constructs a daemon.Daemon
// from the on-disk config and blocks on its Run loop until SIGINT/SIGTERM.
func newStartCmd() *cobra.Command {
	panic("not implemented: agenthive.newStartCmd")
}
