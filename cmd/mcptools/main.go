/*
Package main implements mcp functionality.
*/
package main

import (
	"os"

	"github.com/f/mcptools/cmd/mcptools/commands"
	"github.com/spf13/cobra"
)

// version information placeholders.
var (
	Version = "dev"
)

func init() {
	commands.Version = Version
}

func main() {
	cobra.EnableCommandSorting = false

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
		commands.MockCmd(),
		commands.ProxyCmd(),
		commands.AliasCmd(),
		commands.NewCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
