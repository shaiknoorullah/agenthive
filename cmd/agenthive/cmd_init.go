package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shaiknoorullah/agenthive/internal/identity"
)

// newInitCmd returns `agenthive init`, which generates a fresh Ed25519
// identity and persists it to <configDir>/identity.key.
//
// init is one-shot: if an identity already exists on disk it refuses to run
// rather than rotating the key silently (which would orphan every peer that
// had pinned this daemon's PeerID). Removing the key file is the operator's
// explicit choice — we don't make that decision for them.
func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Generate and persist a new agenthive identity",
		Long: "Generates a fresh Ed25519 keypair and writes it to " +
			"<config-dir>/identity.key with mode 0600. Refuses to run when " +
			"an identity already exists.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Detect an existing key up-front. identity.Load returns an
			// error wrapping os.ErrNotExist when the file is missing,
			// which is the only case in which we proceed.
			if _, err := identity.Load(configDir); err == nil {
				return fmt.Errorf("identity already exists in %s; remove identity.key to re-init", configDir)
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("inspect existing identity: %w", err)
			}

			priv, _, err := identity.Generate()
			if err != nil {
				return fmt.Errorf("generate identity: %w", err)
			}
			if err := identity.Save(configDir, priv); err != nil {
				return fmt.Errorf("save identity: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "identity written to %s\n", configDir)
			return nil
		},
	}
}
