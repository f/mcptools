/*
Package main implements mcp functionality.
*/
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/f/mcptools/cmd/mcptools/commands"
	"github.com/spf13/cobra"
)

// Build parameters.
var (
	Version       string
	TemplatesPath string
)

func init() {
	commands.Version = Version
	commands.TemplatesPath = TemplatesPath
}

func main() {
	cobra.EnableCommandSorting = false

	configPath := filepath.Join(os.Getenv("HOME"), "mcptools.config")
	fmt.Fprintf(os.Stderr, "Loading server config from %s\n", configPath)

	rootCmd := commands.RootCmd()
	rootCmd.AddCommand(
		commands.VersionCmd(),
		commands.ToolsCmd(),
		commands.ResourcesCmd(),
		commands.PromptsCmd(),
		commands.CallCmd(),
		commands.GetPromptCmd(),
		commands.ReadResourceCmd(),
		commands.ShellCmd(),
		commands.WebCmd(),
		commands.MockCmd(),
		commands.ProxyCmd(),
		commands.AliasCmd(),
		commands.ConfigsCmd(),
		commands.NewCmd(),
		commands.GuardCmd(),
		commands.ServersCmd(configPath),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
