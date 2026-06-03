// Command agenthive is the libp2p-backed agenthive daemon and its CLI
// surface.
//
// Subcommands (see plan L5):
//
//	agenthive init                              # generate identity
//	agenthive id                                # print multiaddrs
//	agenthive peers add <multiaddr>             # add peer
//	agenthive peers list                        # list peers
//	agenthive start                             # run the daemon (blocks)
//	agenthive hook PreToolUse                   # stdin->IPC->stdout
//	agenthive respond <action-id> <allow|deny>  # manual override
//
// Global flag --config-dir defaults to ~/.config/agenthive.
package main

import (
	"github.com/spf13/cobra"
)

// configDir is the value of the global --config-dir flag.
var configDir string

// newRootCmd constructs the cobra root and registers every subcommand.
// Tests can call this in-process via cobra.Command.SetIn/SetOut/SetErr.
func newRootCmd() *cobra.Command {
	panic("not implemented: agenthive.newRootCmd")
}

func main() {
	panic("not implemented: agenthive.main")
}
