package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/server"
	"github.com/spf13/cobra"
)

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

		fmt.Printf("Starting server at %s:%d\n", loadedConfig.Host, loadedConfig.Port)
		fmt.Printf("Watching directory: %s\n", absPath)

		srv := server.NewServer(loadedConfig)
		return srv.Start()
	},
}
