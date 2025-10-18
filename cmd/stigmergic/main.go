package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/spf13/cobra"
)

const version = "0.1.0"

var (
	cfgFile string
	port    int
	host    string
)

var rootCmd = &cobra.Command{
	Use:   "stigmergic",
	Short: "A markdown file watcher and renderer",
	Long: `stigmergic watches a directory for markdown files and renders them
beautifully through a local web server with real-time updates.`,
	Version: version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}

		if !cmd.Flags().Changed("port") {
			port = cfg.Port
		}
		if !cmd.Flags().Changed("host") {
			host = cfg.Host
		}

		return nil
	},
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

		fmt.Printf("Starting server at %s:%d\n", host, port)
		fmt.Printf("Watching directory: %s\n", absPath)

		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is .stigmergic.toml)")
	rootCmd.PersistentFlags().IntVarP(&port, "port", "p", 8080, "server port")
	rootCmd.PersistentFlags().StringVar(&host, "host", "localhost", "server host")

	rootCmd.AddCommand(serveCmd)
}

func Execute() error {
	return rootCmd.Execute()
}

func main() {
	if err := Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
