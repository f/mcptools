/*
Package commands implements individual commands for the MCP CLI.
*/
package commands

import (
	"github.com/spf13/cobra"
)

// flags.
const (
	FlagFormat       = "--format"
	FlagFormatShort  = "-f"
	FlagParams       = "--params"
	FlagParamsShort  = "-p"
	FlagHelp         = "--help"
	FlagHelpShort    = "-h"
	FlagServerLogs   = "--server-logs"
	FlagVerbose      = "--verbose"
	FlagVerboseShort = "-v"
)

// entity types.
const (
	EntityTypeTool   = "tool"
	EntityTypePrompt = "prompt"
	EntityTypeRes    = "resource"
)

var (
	// FormatOption is the format option for the command, valid values are "table", "json", and
	// "pretty".
	// Default is "table".
	FormatOption = "table"
	// ParamsString is the params for the command.
	ParamsString string
	// ShowServerLogs is a flag to show server logs.
	ShowServerLogs bool
	// Show http verbose info.
	Verbose bool
)

// RootCmd creates the root command.
func RootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP is a command line interface for interacting with MCP servers",
		Long: `MCP is a command line interface for interacting with Model Context Protocol (MCP) servers.
It allows you to discover and call tools, list resources, and interact with MCP-compatible services.`,
	}

	cmd.PersistentFlags().StringVarP(&FormatOption, "format", "f", "table", "Output format (table, json, pretty)")
	cmd.PersistentFlags().
		StringVarP(&ParamsString, "params", "p", "{}", "JSON string of parameters to pass to the tool (for call command)")
	cmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", false, "Show http verbose info")

	return cmd
}
