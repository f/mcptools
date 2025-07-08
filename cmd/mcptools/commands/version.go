package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version information placeholder.
var Version = "dev"

// getHomeDirectory returns the user's home directory
// Tries HOME first, then falls back to USERPROFILE for Windows.
func getHomeDirectory() string {
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = os.Getenv("USERPROFILE")
	}
	return homeDir
}

// TemplatesPath information placeholder.
var TemplatesPath = getHomeDirectory() + "/.mcpt/templates"

// VersionCmd creates the version command.
func VersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "MCP Tools version %s\n", Version)
		},
	}
}
