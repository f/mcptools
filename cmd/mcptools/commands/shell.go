package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/peterh/liner"
	"github.com/spf13/cobra"
)

// ShellCmd creates the shell command.
func ShellCmd() *cobra.Command { //nolint:gocyclo
	return &cobra.Command{
		Use:                "shell [command args...]",
		Short:              "Start an interactive shell for MCP commands",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		Run: func(thisCmd *cobra.Command, args []string) {
			if len(args) == 1 && (args[0] == FlagHelp || args[0] == FlagHelpShort) {
				_ = thisCmd.Help()
				return
			}

			cmdArgs := args
			parsedArgs := []string{}

			i := 0
			for i < len(cmdArgs) {
				switch {
				case (cmdArgs[i] == FlagFormat || cmdArgs[i] == FlagFormatShort) && i+1 < len(cmdArgs):
					FormatOption = cmdArgs[i+1]
					i += 2
				case cmdArgs[i] == FlagServerLogs:
					ShowServerLogs = true
					i++
				case cmdArgs[i] == FlagAuthUser && i+1 < len(cmdArgs):
					AuthUser = cmdArgs[i+1]
					i += 2
				case cmdArgs[i] == FlagAuthHeader && i+1 < len(cmdArgs):
					AuthHeader = cmdArgs[i+1]
					i += 2
				default:
					parsedArgs = append(parsedArgs, cmdArgs[i])
					i++
				}
			}

			if len(parsedArgs) == 0 {
				fmt.Fprintln(os.Stderr, "Error: command to execute is required when using the shell")
				fmt.Fprintln(os.Stderr, "Example: mcp shell npx -y @modelcontextprotocol/server-filesystem ~")
				os.Exit(1)
			}

			mcpClient, clientErr := CreateClientFunc(parsedArgs)
			if clientErr != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", clientErr)
				os.Exit(1)
			}

			fmt.Fprintf(thisCmd.OutOrStdout(), "mcp > MCP Tools Shell (%s)\n", Version)
			fmt.Fprintf(thisCmd.OutOrStdout(), "mcp > Connected to Server: %s\n", strings.Join(parsedArgs, " "))
			fmt.Fprintf(thisCmd.OutOrStdout(), "\nmcp > Type '/h' for help or '/q' to quit\n")

			line := liner.NewLiner()
			line.SetCtrlCAborts(true)
			defer func() { _ = line.Close() }()

			defer setUpHistory(line)()
			setUpCompleter(line)

			for {
				input, err := line.Prompt("mcp > ")
				if err != nil {
					if errors.Is(err, liner.ErrPromptAborted) {
						fmt.Fprintln(thisCmd.OutOrStdout(), "Exiting MCP shell")
						break
					}
					fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
					break
				}

				if input == "" {
					continue
				}

				line.AppendHistory(input)

				if input == "/q" || input == "/quit" || input == "exit" {
					fmt.Fprintln(thisCmd.OutOrStdout(), "Exiting MCP shell")
					break
				}

				if input == "/h" || input == "/help" || input == "help" {
					printShellHelp(thisCmd)
					continue
				}

				parts := strings.Fields(input)
				if len(parts) == 0 {
					continue
				}

				command := parts[0]
				commandArgs := parts[1:]

				var resp map[string]any
				var listErr error

				switch command {
				case "tools":
					var listToolsResult *mcp.ListToolsResult
					listToolsResult, listErr = mcpClient.ListTools(context.Background(), mcp.ListToolsRequest{})

					var tools []any
					if listErr == nil && listToolsResult != nil {
						tools = ConvertJSONToSlice(listToolsResult.Tools)
					}

					resp = map[string]any{"tools": tools}
					if formatErr := FormatAndPrintResponse(thisCmd, resp, listErr); formatErr != nil {
						fmt.Fprintf(os.Stderr, "%v\n", formatErr)
						continue
					}
				case "resources":
					var listResourcesResult *mcp.ListResourcesResult
					listResourcesResult, listErr = mcpClient.ListResources(context.Background(), mcp.ListResourcesRequest{})

					var resources []any
					if listErr == nil && listResourcesResult != nil {
						resources = ConvertJSONToSlice(listResourcesResult.Resources)
					}

					resp = map[string]any{"resources": resources}
					if formatErr := FormatAndPrintResponse(thisCmd, resp, listErr); formatErr != nil {
						fmt.Fprintf(os.Stderr, "%v\n", formatErr)
						continue
					}
				case "prompts":
					var listPromptsResult *mcp.ListPromptsResult
					listPromptsResult, listErr = mcpClient.ListPrompts(context.Background(), mcp.ListPromptsRequest{})

					var prompts []any
					if listErr == nil && listPromptsResult != nil {
						prompts = ConvertJSONToSlice(listPromptsResult.Prompts)
					}

					resp = map[string]any{"prompts": prompts}
					if formatErr := FormatAndPrintResponse(thisCmd, resp, listErr); formatErr != nil {
						fmt.Fprintf(os.Stderr, "%v\n", formatErr)
						continue
					}
				case "format":
					if len(commandArgs) < 1 {
						fmt.Fprintf(thisCmd.OutOrStdout(), "Current format: %s\n", FormatOption)
						continue
					}

					oldFormat := FormatOption
					defer func() { FormatOption = oldFormat }()
					newFormat := commandArgs[0]
					if IsValidFormat(newFormat) {
						FormatOption = newFormat
						fmt.Fprintf(thisCmd.OutOrStdout(), "Format set to: %s\n", FormatOption)
					} else {
						fmt.Fprintln(thisCmd.OutOrStdout(), "Invalid format. Use: table, json, or pretty")
					}
				case "call":
					if len(commandArgs) < 1 {
						fmt.Fprintln(thisCmd.OutOrStdout(), "Usage: call <entity> [--params '{...}']")
						continue
					}
					err := callCommand(thisCmd, mcpClient, commandArgs)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
						continue
					}
				default:
					if err := callCommand(thisCmd, mcpClient, append([]string{command}, commandArgs...)); err != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n", err)
						continue
					}
				}
			}
		},
	}
}

func callCommand(thisCmd *cobra.Command, mcpClient *client.Client, commandArgs []string) error {
	entityName := commandArgs[0]
	entityType := EntityTypeTool
	parts := strings.SplitN(entityName, ":", 2)
	if len(parts) == 2 {
		entityType = parts[0]
		entityName = parts[1]
	}

	params := map[string]any{}
	remainingArgs := []string{}
	for i := 1; i < len(commandArgs); i++ {
		switch commandArgs[i] {
		case FlagParams, FlagParamsShort:
			continue
		case FlagFormat, FlagFormatShort:
			if i+1 >= len(commandArgs) {
				return fmt.Errorf("no format provided after %s", commandArgs[i])
			}
			oldFormat := FormatOption
			defer func() { FormatOption = oldFormat }()
			newFormat := commandArgs[i+1]
			if IsValidFormat(newFormat) {
				FormatOption = newFormat
			} else {
				fmt.Fprintln(thisCmd.OutOrStdout(), "Invalid format. Use: table, json, or pretty")
			}
			i++
		default:
			remainingArgs = append(remainingArgs, commandArgs[i])
		}
	}

	if len(remainingArgs) > 0 {
		if err := parseJSONBestEffort(strings.Join(remainingArgs, " "), &params); err != nil {
			return fmt.Errorf("invalid JSON for params: %w", err)
		}
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
	}

	if execErr != nil {
		return execErr
	}

	formatErr := FormatAndPrintResponse(thisCmd, resp, nil)
	if formatErr != nil {
		return fmt.Errorf("error formatting output: %w", formatErr)
	}

	return nil
}

func parseJSONBestEffort(jsonString string, params *map[string]any) error {
	jsonString = strings.Trim(jsonString, "'\"")
	if jsonString == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(jsonString), &params); err != nil {
		return err
	}
	return nil
}

func setUpHistory(line *liner.State) func() {
	historyFile := filepath.Join(getHomeDirectory(), ".mcp_history")
	if f, err := os.Open(filepath.Clean(historyFile)); err == nil {
		_, _ = line.ReadHistory(f)
		_ = f.Close()
	}

	return func() {
		if f, err := os.Create(historyFile); err == nil {
			_, _ = line.WriteHistory(f)
			_ = f.Close()
		}
	}
}

func setUpCompleter(line *liner.State) {
	line.SetCompleter(func(line string) (c []string) {
		commands := []string{
			"tools",
			"resources",
			"prompts",
			"call",
			"format",
			"help",
			"exit",
			"/h",
			"/q",
			"/help",
			"/quit",
		}
		for _, cmd := range commands {
			if strings.HasPrefix(cmd, line) {
				c = append(c, cmd)
			}
		}
		return
	})
}

func printShellHelp(thisCmd *cobra.Command) {
	fmt.Fprintln(thisCmd.OutOrStdout(), "MCP Shell Commands:")
	fmt.Fprintln(thisCmd.OutOrStdout(), "  tools                      List available tools")
	fmt.Fprintln(thisCmd.OutOrStdout(), "  resources                  List available resources")
	fmt.Fprintln(thisCmd.OutOrStdout(), "  prompts                    List available prompts")
	fmt.Fprintln(thisCmd.OutOrStdout(), "  call <entity> [--params '{...}']  Call a tool, resource, or prompt")
	fmt.Fprintln(thisCmd.OutOrStdout(), "  format [json|pretty|table] Get or set output format")
	fmt.Fprintln(thisCmd.OutOrStdout(), "Direct Tool Calling:")
	fmt.Fprintln(thisCmd.OutOrStdout(), "  <tool_name> {\"param\": \"value\"}  Call a tool directly with JSON parameters")
	fmt.Fprintln(thisCmd.OutOrStdout(), "  resource:<uri>             Read a resource directly")
	fmt.Fprintln(thisCmd.OutOrStdout(), "  prompt:<name>              Get a prompt directly")
	fmt.Fprintln(thisCmd.OutOrStdout(), "Special Commands:")
	fmt.Fprintln(thisCmd.OutOrStdout(), "  /h, /help                  Show this help")
	fmt.Fprintln(thisCmd.OutOrStdout(), "  /q, /quit, exit            Exit the shell")
}
