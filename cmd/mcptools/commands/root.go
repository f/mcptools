/*
Package commands implements individual commands for the MCP CLI.
*/
package commands

import (
	"github.com/spf13/cobra"
)

// flags.
const (
	FlagFormat      = "--format"
	FlagFormatShort = "-f"
	FlagParams      = "--params"
	FlagParamsShort = "-p"
	FlagHelp        = "--help"
	FlagHelpShort   = "-h"
	FlagServerLogs  = "--server-logs"
	FlagTransport   = "--transport"
	FlagAuthUser    = "--auth-user"
	FlagAuthHeader  = "--auth-header"
	FlagQuiet       = "--quiet"
	FlagQuietShort  = "-q"
)

// entity types.
const (
	EntityTypeTool   = "tool"
	EntityTypePrompt = "prompt"
	EntityTypeRes    = "resource"
)

// transport types.
const (
	TransportSSE   = "sse"
	TransportHTTP  = "http"
	TransportStdio = "stdio"
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
	// TransportOption is the transport option for HTTP connections, valid values are "sse" and "http".
	// Default is "http" (streamable HTTP).
	TransportOption = "http"
	// AuthUser contains username:password for basic authentication.
	AuthUser string
	// AuthHeader is a custom Authorization header.
	AuthHeader string
	// QuietMode suppresses startup messages.
	QuietMode bool
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
	cmd.PersistentFlags().StringVar(&TransportOption, "transport", "http", "HTTP transport type (http, sse)")
	cmd.PersistentFlags().StringVar(&AuthUser, "auth-user", "", "Basic authentication in username:password format")
	cmd.PersistentFlags().StringVar(&AuthHeader, "auth-header", "", "Custom Authorization header (e.g., 'Bearer token' or 'Basic base64credentials')")

	return cmd
}
