package main

import (
	"os"

	"github.com/spf13/cobra"
)

const version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:   "stigmergic",
	Short: "A markdown file watcher and renderer",
	Long: `stigmergic watches a directory for markdown files and renders them
beautifully through a local web server with real-time updates.`,
	Version: version,
}

func Execute() error {
	return rootCmd.Execute()
}

func main() {
	if err := Execute(); err != nil {
		os.Exit(1)
	}
}
