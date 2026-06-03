package main

import (
	"github.com/spf13/cobra"
)

// newPeersCmd returns the `agenthive peers` command tree with `add` and
// `list` subcommands.
func newPeersCmd() *cobra.Command {
	panic("not implemented: agenthive.newPeersCmd")
}

// newPeersAddCmd returns `agenthive peers add <multiaddr>`, which parses the
// argument as a multiaddr and writes it into the StateStore.
func newPeersAddCmd() *cobra.Command {
	panic("not implemented: agenthive.newPeersAddCmd")
}

// newPeersListCmd returns `agenthive peers list`, which prints the peers
// known to the StateStore as a table.
func newPeersListCmd() *cobra.Command {
	panic("not implemented: agenthive.newPeersListCmd")
}
