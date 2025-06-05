package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"
)

// CallCmd creates the call command.
func CallCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "call entity [command args...]",
		Short:              "Call a tool, resource, or prompt on the MCP server",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		Run: func(thisCmd *cobra.Command, args []string) {
			if len(args) == 1 && (args[0] == FlagHelp || args[0] == FlagHelpShort) {
				_ = thisCmd.Help()
				return
			}

			if len(args) == 0 {
				fmt.Fprintln(os.Stderr, "Error: entity name is required")
				fmt.Fprintln(
					os.Stderr,
					"Example: mcp call read_file npx -y @modelcontextprotocol/server-filesystem ~",
				)
				os.Exit(1)
			}

			cmdArgs := args
			parsedArgs := []string{}
			entityName := ""

			i := 0
			entityExtracted := false

			for i < len(cmdArgs) {
				switch {
				case (cmdArgs[i] == FlagFormat || cmdArgs[i] == FlagFormatShort) && i+1 < len(cmdArgs):
					FormatOption = cmdArgs[i+1]
					i += 2
				case (cmdArgs[i] == FlagParams || cmdArgs[i] == FlagParamsShort) && i+1 < len(cmdArgs):
					ParamsString = cmdArgs[i+1]
					i += 2
				case !entityExtracted:
					entityName = cmdArgs[i]
					entityExtracted = true
					i++
				case cmdArgs[i] == FlagServerLogs:
					ShowServerLogs = true
					i++
				default:
					parsedArgs = append(parsedArgs, cmdArgs[i])
					i++
				}
			}

			if entityName == "" {
				fmt.Fprintln(os.Stderr, "Error: entity name is required")
				fmt.Fprintln(
					os.Stderr,
					"Example: mcp call read_file npx -y @modelcontextprotocol/server-filesystem ~",
				)
				os.Exit(1)
			}

			entityType := EntityTypeTool

			parts := strings.SplitN(entityName, ":", 2)
			if len(parts) == 2 {
				entityType = parts[0]
				entityName = parts[1]
			}

			if len(parsedArgs) == 0 {
				fmt.Fprintln(os.Stderr, "Error: command to execute is required when using stdio transport")
				fmt.Fprintln(
					os.Stderr,
					"Example: mcp call read_file npx -y @modelcontextprotocol/server-filesystem ~",
				)
				os.Exit(1)
			}

			var params map[string]any
			if ParamsString != "" {
				if jsonErr := json.Unmarshal([]byte(ParamsString), &params); jsonErr != nil {
					fmt.Fprintf(os.Stderr, "Error: invalid JSON for params: %v\n", jsonErr)
					os.Exit(1)
				}
			}

			mcpClient, clientErr := CreateClientFunc(parsedArgs)
			if clientErr != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", clientErr)
				os.Exit(1)
			}

			var resp map[string]any
			var execErr error

			switch entityType {
			case EntityTypeTool:
				var toolResponse *mcp.CallToolResult
				request := mcp.CallToolRequest{}
				request.Params.Name = entityName
				request.Params.Arguments = params
				toolResponse, execErr = mcpClient.CallTool(context.Background(), request)
				if execErr == nil && toolResponse != nil {
					resp = ConvertJSONToMap(toolResponse)
				} else {
					resp = map[string]any{}
				}
			case EntityTypeRes:
				var resourceResponse *mcp.ReadResourceResult
				request := mcp.ReadResourceRequest{}
				request.Params.URI = entityName
				resourceResponse, execErr = mcpClient.ReadResource(context.Background(), request)
				if execErr == nil && resourceResponse != nil {
					resp = ConvertJSONToMap(resourceResponse)
				} else {
					resp = map[string]any{}
				}
			case EntityTypePrompt:
				var promptResponse *mcp.GetPromptResult
				request := mcp.GetPromptRequest{}
				request.Params.Name = entityName
				promptResponse, execErr = mcpClient.GetPrompt(context.Background(), request)
				if execErr == nil && promptResponse != nil {
					resp = ConvertJSONToMap(promptResponse)
				} else {
					resp = map[string]any{}
				}
			default:
				fmt.Fprintf(os.Stderr, "Error: unsupported entity type: %s\n", entityType)
				os.Exit(1)
			}

			if formatErr := FormatAndPrintResponse(thisCmd, resp, execErr); formatErr != nil {
				fmt.Fprintf(os.Stderr, "%v\n", formatErr)
				os.Exit(1)
			}

			// Exit with non-zero code if there was an execution error
			if execErr != nil {
				os.Exit(1)
			}
		},
	}
}
