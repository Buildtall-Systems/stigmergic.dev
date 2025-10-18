package main

import (
	"fmt"
	"os"

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

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is .stigmergic.toml)")
	rootCmd.PersistentFlags().IntVarP(&port, "port", "p", 8080, "server port")
	rootCmd.PersistentFlags().StringVar(&host, "host", "localhost", "server host")
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
