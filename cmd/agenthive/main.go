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
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// configDir holds the resolved value of the global --config-dir flag. It is
// populated by cobra during flag parsing on the root command. Subcommands
// read this variable rather than re-parsing the flag themselves.
var configDir string

// version, commit, and date are populated by -ldflags at release-build
// time (see .goreleaser.yml). The default values are used for unreleased
// `go build` and `go install` invocations from a working tree.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// defaultConfigDir returns ~/.config/agenthive (or just .config/agenthive in
// the unlikely case the home directory cannot be resolved). It is called
// once at root construction so subcommands inherit a stable default even
// when no --config-dir is supplied.
func defaultConfigDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "agenthive")
	}
	return filepath.Join(".config", "agenthive")
}

// newRootCmd constructs the cobra root and registers every subcommand.
// Tests can call this in-process via cobra.Command.SetIn/SetOut/SetErr to
// run the CLI without exec.Command — the package goes out of its way to
// avoid touching os.Stdin / os.Stdout directly so this round-trips cleanly.
func newRootCmd() *cobra.Command {
	var showVersion bool

	root := &cobra.Command{
		Use:           "agenthive",
		Short:         "agenthive — libp2p-backed personal agent mesh",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(),
					"agenthive %s (commit %s, built %s)\n",
					version, commit, date)
				return nil
			}
			return cmd.Help()
		},
	}
	root.PersistentFlags().StringVar(&configDir, "config-dir", defaultConfigDir(),
		"path to the agenthive config directory")
	root.Flags().BoolVar(&showVersion, "version", false,
		"print version information and exit")

	root.AddCommand(newInitCmd())
	root.AddCommand(newIDCmd())
	root.AddCommand(newPeersCmd())
	root.AddCommand(newStartCmd())
	root.AddCommand(newHookCmd())
	root.AddCommand(newRespondCmd())
	root.AddCommand(newRoutesCmd())
	root.AddCommand(newTUICmd())

	return root
}

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "agenthive:", err)
		os.Exit(1)
	}
}
