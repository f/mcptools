package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// ServersConfig represents the root configuration structure
type ServersConfig struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// ServersCmd creates the servers command.
func ServersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "servers",
		Short: "List the MCP servers installed",
		Run: func(cmd *cobra.Command, args []string) {
			// Read the config file
			configPath := filepath.Join("/Users/kriti/Projects/mcptools/config/mcp_servers.json")
			data, err := os.ReadFile(configPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading config file: %v\n", err)
				os.Exit(1)
			}

			// Parse the JSON
			var config ServersConfig
			if err := json.Unmarshal(data, &config); err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing config file: %v\n", err)
				os.Exit(1)
			}

			// Print each server configuration
			for name, server := range config.MCPServers {
				// Print server name and command
				fmt.Printf("%s: %s", name, server.Command)

				// Print arguments
				for _, arg := range server.Args {
					fmt.Printf(" %s", arg)
				}
				fmt.Println()
			}
		},
	}
}
