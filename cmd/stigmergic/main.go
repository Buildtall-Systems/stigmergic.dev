package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/config"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
)

// version is set via ldflags at build time by goreleaser.
// Falls back to "dev" for local builds.
var version = "dev"

var (
	cfgFile          string
	port             int
	host             string
	logLevel         string
	respectGitignore bool
	defaultFile      string
	loadedConfig     *config.Config
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

		loadedConfig = cfg

		if cmd.Flags().Changed("port") {
			loadedConfig.Port = port
		}
		if cmd.Flags().Changed("host") {
			loadedConfig.Host = host
		}
		if cmd.Flags().Changed("log-level") {
			loadedConfig.LogLevel = logLevel
		}
		if cmd.Flags().Changed("respect-gitignore") {
			loadedConfig.RespectGitignore = respectGitignore
		}
		if cmd.Flags().Changed("default-file") {
			loadedConfig.DefaultFile = defaultFile
		}

		logger.Init(loadedConfig.LogLevel)

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return serveCmd.RunE(cmd, []string{"."})
		}
		return cmd.Help()
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is .stigmergic.toml)")
	rootCmd.PersistentFlags().IntVarP(&port, "port", "p", 8080, "server port")
	rootCmd.PersistentFlags().StringVar(&host, "host", "localhost", "server host")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "ERROR", "log level (DEBUG, INFO, WARN, ERROR)")
	rootCmd.PersistentFlags().BoolVar(&respectGitignore, "respect-gitignore", true, "respect .gitignore patterns")
	rootCmd.PersistentFlags().StringVar(&defaultFile, "default-file", "", "file to display on homepage (relative to watch path)")

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
