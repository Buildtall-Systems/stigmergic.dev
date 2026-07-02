package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/server"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/source"
	"github.com/Buildtall-Systems/stigmergic.dev/site"
)

var siteCmd = &cobra.Command{
	Use:   "site",
	Short: "Serve the embedded public website",
	Long: `Serve the stigmergic.dev website from content compiled into the binary.
The content is fixed at build time: no filesystem access, no live reload.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		contentFS, err := site.Content()
		if err != nil {
			return fmt.Errorf("failed to open embedded content: %w", err)
		}

		portExplicitlySet := cmd.Flags().Changed("port")
		if !portExplicitlySet {
			availablePort, portErr := findAvailablePort(loadedConfig.Host, loadedConfig.Port, 100)
			if portErr != nil {
				return fmt.Errorf("failed to find available port: %w", portErr)
			}
			if availablePort != loadedConfig.Port {
				fmt.Printf("Port %d is occupied, using port %d instead\n", loadedConfig.Port, availablePort)
				loadedConfig.Port = availablePort
			}
		}

		fmt.Printf("Starting server at %s:%d\n", loadedConfig.Host, loadedConfig.Port)
		fmt.Println("Serving embedded site content")

		src := source.NewEmbedded(contentFS, "stigmergic.dev")
		srv := server.NewServer(loadedConfig, src)
		return srv.Start()
	},
}
