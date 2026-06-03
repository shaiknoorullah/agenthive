package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/identity"
)

// peersStateFile is the persisted CRDT state file used by `peers add` and
// `peers list` when operating without a running daemon. It matches the path
// the daemon writes to on shutdown, so the CLI and the daemon share state
// across restarts.
const peersStateFile = "state.json"

// newPeersCmd returns the `agenthive peers` command tree with `add` and
// `list` subcommands.
func newPeersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "peers",
		Short: "Manage known agenthive peers",
		Long: "Subcommands for inspecting and editing the local peer set. " +
			"Edits go to the CRDT state file at <config-dir>/state.json; the " +
			"running daemon (if any) will pick them up on next restart.",
	}
	cmd.AddCommand(newPeersAddCmd())
	cmd.AddCommand(newPeersListCmd())
	return cmd
}

// loadPeerState constructs a StateStore rooted at the on-disk state.json.
// The PeerID is derived from the persisted identity when available so any
// local writes inherit a real HLC peer; without an identity the StateStore
// still works but with a placeholder peer ID.
func loadPeerState() (*crdt.StateStore, error) {
	peerID := "cli"
	if priv, err := identity.Load(configDir); err == nil {
		if pid, perr := peer.IDFromPrivateKey(priv); perr == nil {
			peerID = pid.String()
		}
	}
	state := crdt.NewStateStore(peerID)
	if err := state.LoadFromFile(filepath.Join(configDir, peersStateFile)); err != nil {
		return nil, fmt.Errorf("load peer state: %w", err)
	}
	return state, nil
}

// savePeerState persists state to disk. The directory is created with mode
// 0700 by the StateStore helper.
func savePeerState(state *crdt.StateStore) error {
	if err := state.SaveToFile(filepath.Join(configDir, peersStateFile)); err != nil {
		return fmt.Errorf("save peer state: %w", err)
	}
	return nil
}

// newPeersAddCmd returns `agenthive peers add <multiaddr>`, which parses the
// argument as a libp2p multiaddr containing a /p2p/<peerid> suffix and
// writes a PeerInfo entry into the CRDT state file. The next daemon start
// will dial the new peer automatically.
func newPeersAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <multiaddr>",
		Short: "Add a peer by multiaddr",
		Long: "Parses the supplied multiaddr (must include a /p2p/<peerid> " +
			"component) and records it in the local CRDT state. The running " +
			"daemon picks up the new peer on next restart; live add is a " +
			"follow-up.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			addrStr := args[0]
			addr, err := ma.NewMultiaddr(addrStr)
			if err != nil {
				return fmt.Errorf("parse multiaddr: %w", err)
			}

			// SplitLast pulls the trailing /p2p/<peerid> off the multiaddr;
			// the remaining transport portion is what libp2p will dial.
			info, err := peer.AddrInfoFromP2pAddr(addr)
			if err != nil {
				return fmt.Errorf("extract peer info from multiaddr: %w", err)
			}
			if info.ID == "" {
				return errors.New("multiaddr is missing /p2p/<peerid> component")
			}

			state, err := loadPeerState()
			if err != nil {
				return err
			}

			transportAddr := addrStr
			if len(info.Addrs) > 0 {
				transportAddr = info.Addrs[0].String()
			}
			state.SetPeer(info.ID.String(), crdt.PeerInfo{
				Status:   "manual",
				Addr:     transportAddr,
				LinkType: "p2p",
				LastSeen: time.Now().UTC().Format(time.RFC3339),
			})

			if err := savePeerState(state); err != nil {
				return err
			}
			// Confirmation is best-effort; the real outcome is the persisted
			// state which the next `peers list` will reflect.
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "added peer %s\n", info.ID)
			return nil
		},
	}
}

// newPeersListCmd returns `agenthive peers list`, which prints the peers
// known to the local CRDT state as a tab-aligned table. The table is sorted
// by PeerID so output is stable across runs (and across machines, which
// makes diffing CI logs sane).
func newPeersListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List known peers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := loadPeerState()
			if err != nil {
				return err
			}
			peers := state.ListPeers()

			ids := make([]string, 0, len(peers))
			for id := range peers {
				ids = append(ids, id)
			}
			sort.Strings(ids)

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			// tabwriter buffers internally; per-row write errors are surfaced
			// at Flush below, which is the only error we propagate.
			_, _ = fmt.Fprintln(w, "PEER\tSTATUS\tLINK\tADDR\tLAST_SEEN")
			for _, id := range ids {
				info := peers[id]
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					id, info.Status, info.LinkType, info.Addr, info.LastSeen)
			}
			return w.Flush()
		},
	}
}
