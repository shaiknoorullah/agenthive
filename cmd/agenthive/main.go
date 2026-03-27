package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/daemon"
	"github.com/shaiknoorullah/agenthive/internal/identity"
	"github.com/spf13/cobra"
)

func defaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "agenthive")
	}
	return filepath.Join(home, ".config", "agenthive")
}

func main() {
	var configDir string

	rootCmd := &cobra.Command{
		Use:   "agenthive",
		Short: "A self-hosted, encrypted mesh for AI agent notification and control",
		Long: `agenthive turns every terminal into a command center for your AI agents.
Get notifications, approve actions, and control your coding agents from anywhere.`,
	}

	rootCmd.PersistentFlags().StringVar(&configDir, "config-dir", defaultConfigDir(), "configuration directory")

	// agenthive init
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize peer identity and configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("name")
			if name == "" {
				hostname, err := os.Hostname()
				if err != nil {
					name = "unnamed"
				} else {
					name = hostname
				}
			}

			if err := os.MkdirAll(configDir, 0700); err != nil {
				return fmt.Errorf("create config directory: %w", err)
			}

			idPath := filepath.Join(configDir, "identity.json")
			if _, err := os.Stat(idPath); err == nil {
				return fmt.Errorf("identity already exists at %s (use --force to regenerate)", idPath)
			}

			id, err := identity.Generate(name)
			if err != nil {
				return fmt.Errorf("generate identity: %w", err)
			}

			if err := id.SaveToFile(idPath); err != nil {
				return fmt.Errorf("save identity: %w", err)
			}

			fmt.Printf("Initialized agenthive peer:\n")
			fmt.Printf("  Name:     %s\n", id.Name)
			fmt.Printf("  Peer ID:  %s\n", id.PeerID)
			fmt.Printf("  Config:   %s\n", configDir)
			return nil
		},
	}
	initCmd.Flags().String("name", "", "peer name (defaults to hostname)")

	// agenthive start
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the agenthive daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			idPath := filepath.Join(configDir, "identity.json")
			id, err := identity.LoadFromFile(idPath)
			if err != nil {
				return fmt.Errorf("load identity (run 'agenthive init' first): %w", err)
			}

			cfg := daemon.DaemonConfig{
				ConfigDir: configDir,
				PeerName:  id.Name,
			}

			d, err := daemon.NewDaemon(cfg)
			if err != nil {
				return fmt.Errorf("create daemon: %w", err)
			}

			fmt.Printf("Starting agenthive daemon (peer: %s)...\n", id.Name)
			return d.Start()
		},
	}

	// agenthive stop
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the agenthive daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := daemon.DaemonConfig{ConfigDir: configDir}
			status := daemon.DaemonStatus(cfg)
			if !status.Running {
				fmt.Println("Daemon is not running.")
				return nil
			}

			proc, err := os.FindProcess(status.PID)
			if err != nil {
				return fmt.Errorf("find daemon process: %w", err)
			}

			if err := proc.Signal(os.Interrupt); err != nil {
				return fmt.Errorf("send interrupt signal: %w", err)
			}

			fmt.Printf("Sent stop signal to daemon (PID %d).\n", status.PID)
			return nil
		},
	}

	// agenthive status
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := daemon.DaemonConfig{ConfigDir: configDir}
			status := daemon.DaemonStatus(cfg)
			if status.Running {
				fmt.Printf("Daemon is running (PID %d).\n", status.PID)
			} else {
				fmt.Println("Daemon is not running.")
			}
			return nil
		},
	}

	// agenthive peers
	peersCmd := &cobra.Command{
		Use:   "peers",
		Short: "List connected peers",
		RunE: func(cmd *cobra.Command, args []string) error {
			store := crdt.NewStateStore("local")
			statePath := filepath.Join(configDir, "state.json")
			if err := store.LoadFromFile(statePath); err != nil {
				return fmt.Errorf("load state: %w", err)
			}

			peers := store.ListPeers()
			if len(peers) == 0 {
				fmt.Println("No peers configured.")
				return nil
			}

			for id, peer := range peers {
				fmt.Printf("  %s  %-12s  %s\n", statusIcon(peer.Status), id, peer.Name)
			}
			return nil
		},
	}

	// agenthive routes
	routesCmd := &cobra.Command{
		Use:   "routes",
		Short: "List or manage routing rules",
	}

	routesListCmd := &cobra.Command{
		Use:   "list",
		Short: "List routing rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			store := crdt.NewStateStore("local")
			statePath := filepath.Join(configDir, "state.json")
			if err := store.LoadFromFile(statePath); err != nil {
				return fmt.Errorf("load state: %w", err)
			}

			routes := store.ListRoutes()
			if len(routes) == 0 {
				fmt.Println("No routing rules configured.")
				return nil
			}

			for id, rule := range routes {
				fmt.Printf("  %-20s -> %v\n", formatMatch(id, rule.Match), rule.Targets)
			}
			return nil
		},
	}

	routesCmd.AddCommand(routesListCmd)

	// agenthive config
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Show or set configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			store := crdt.NewStateStore("local")
			statePath := filepath.Join(configDir, "state.json")
			if err := store.LoadFromFile(statePath); err != nil {
				return fmt.Errorf("load state: %w", err)
			}

			if len(args) == 0 {
				// Show all config as JSON
				data, _ := json.MarshalIndent(store.ConfigMap(), "", "  ")
				fmt.Println(string(data))
				return nil
			}
			return nil
		},
	}

	// agenthive respond
	respondCmd := &cobra.Command{
		Use:   "respond <allow|deny>:<request-id>",
		Short: "Respond to an agent action request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse "allow:req-42" or "deny:req-42"
			fmt.Printf("Response registered: %s\n", args[0])
			return nil
		},
	}

	rootCmd.AddCommand(initCmd, startCmd, stopCmd, statusCmd, peersCmd, routesCmd, configCmd, respondCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func statusIcon(status string) string {
	switch status {
	case "online":
		return "*"
	case "offline":
		return "o"
	default:
		return "?"
	}
}

func formatMatch(id string, match crdt.RouteMatch) string {
	if match.Project == "" && match.Source == "" && match.Session == "" && match.Priority == "" {
		return id + " (default)"
	}
	parts := []string{}
	if match.Project != "" {
		parts = append(parts, "project:"+match.Project)
	}
	if match.Source != "" {
		parts = append(parts, "source:"+match.Source)
	}
	if match.Session != "" {
		parts = append(parts, "session:"+match.Session)
	}
	if match.Priority != "" {
		parts = append(parts, "priority:"+match.Priority)
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}
