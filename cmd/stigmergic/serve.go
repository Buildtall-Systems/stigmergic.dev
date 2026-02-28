package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/server"
)

func isPortAvailable(host string, port int) bool {
	addr := fmt.Sprintf("%s:%d", host, port)
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func findAvailablePort(host string, startPort int, maxAttempts int) (int, error) {
	for i := 0; i < maxAttempts; i++ {
		port := startPort + i
		if isPortAvailable(host, port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port found after %d attempts starting from %d", maxAttempts, startPort)
}

var enableAuth bool

func init() {
	serveCmd.Flags().BoolVar(&enableAuth, "auth", false, "enable Nostr authentication (requires allowed_npubs in config)")
}

var serveCmd = &cobra.Command{
	Use:   "serve [path]",
	Short: "Start the markdown server",
	Long: `Start a local web server that watches and renders markdown files.
If no path is provided, the current directory is used.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("failed to resolve path: %w", err)
		}

		info, err := os.Stat(absPath)
		if err != nil {
			return fmt.Errorf("path does not exist: %w", err)
		}

		if !info.IsDir() {
			return fmt.Errorf("path is not a directory: %s", absPath)
		}

		loadedConfig.WatchPath = absPath

		if cmd.Flags().Changed("auth") {
			loadedConfig.Auth.Enabled = enableAuth
		}

		portExplicitlySet := cmd.Flags().Changed("port")
		if !portExplicitlySet {
			availablePort, err := findAvailablePort(loadedConfig.Host, loadedConfig.Port, 100)
			if err != nil {
				return fmt.Errorf("failed to find available port: %w", err)
			}
			if availablePort != loadedConfig.Port {
				fmt.Printf("Port %d is occupied, using port %d instead\n", loadedConfig.Port, availablePort)
				loadedConfig.Port = availablePort
			}
		}

		fmt.Printf("Starting server at %s:%d\n", loadedConfig.Host, loadedConfig.Port)
		fmt.Printf("Watching directory: %s\n", absPath)

		srv := server.NewServer(loadedConfig)
		return srv.Start()
	},
}
