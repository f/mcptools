package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/f/mcptools/pkg/alias"
	"github.com/f/mcptools/pkg/client"
	"github.com/fatih/color"
	"github.com/peterh/liner"
	"github.com/spf13/cobra"
)

const (
	// LLMProviderOpenAI is the identifier for OpenAI.
	LLMProviderOpenAI = "openai"
	// LLMProviderAnthropic is the identifier for Anthropic.
	LLMProviderAnthropic = "anthropic"
	// MaxServers is the maximum number of servers allowed.
	MaxServers = 3
)

// Terminal colors for UI elements.
var (
	// Color for MCP messages.
	mcpColor = color.New(color.FgCyan).Add(color.Bold)
	// Color for agent responses.
	agentColor = color.New(color.FgMagenta)
	// Color for tool calls.
	toolCallColor = color.New(color.FgYellow).Add(color.Bold)
	// Color for tool results.
	toolResultColor = color.New(color.FgBlue)
	// Color for errors.
	errorColor = color.New(color.FgRed).Add(color.Bold)
	// Color for server information.
	serverColor = color.New(color.FgHiCyan)
)

// Tool represents a tool available to the LLM.
type Tool struct {
	Parameters  map[string]interface{} `json:"parameters"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
}

// LLMCmd creates the llm command.
func LLMCmd() *cobra.Command {
	var providerFlag string
	var modelFlag string
	var apiKeyFlag string
	var multiServerFlags []string
	var noColorFlag bool

	cmd := &cobra.Command{
		Use:   "llm [-- command args...]",
		Short: "Start an interactive shell with LLM integration",
		Long: `Start an interactive shell with LLM integration.
This command connects to an LLM provider and provides a chat interface.
The LLM can execute MCP tools on your behalf.

Example usage:
  mcp llm -- npx -y @modelcontextprotocol/server-filesystem ~
  mcp llm -M https://ne.tools -M "npx -y @modelcontextprotocol/server-filesystem ~"
  mcp llm --provider anthropic --model claude-3-7-sonnet-20250219

Note: When specifying a server command directly, use -- to separate MCP flags from the server command.`,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(thisCmd *cobra.Command, args []string) error {
			// Process the args manually to separate flags from positional args
			processedArgs, err := processArgs(args, &providerFlag, &modelFlag, &apiKeyFlag, &multiServerFlags, &noColorFlag)
			if err != nil {
				return err
			}

			// If no-color flag is set, disable colors
			if noColorFlag {
				color.NoColor = true
			}

			// Collect server commands to connect to
			var serverCommands [][]string

			// If -M/--multi flags are provided, use those
			if len(multiServerFlags) > 0 {
				// Check max servers limit
				if len(multiServerFlags) > MaxServers {
					return fmt.Errorf("maximum of %d servers allowed, got %d", MaxServers, len(multiServerFlags))
				}

				// Process each server flag
				for _, serverFlag := range multiServerFlags {
					// Check if it's an alias
					if aliasCmd, found := alias.GetServerCommand(serverFlag); found {
						// It's an alias, use the command from the alias
						serverCommands = append(serverCommands, client.ParseCommandString(aliasCmd))
					} else {
						// Not an alias, parse it as a command
						serverCommands = append(serverCommands, client.ParseCommandString(serverFlag))
					}
				}
			} else if len(processedArgs) > 0 {
				// Legacy mode - use positional args for the server command
				// Check if it's an alias
				if aliasCmd, found := alias.GetServerCommand(processedArgs[0]); found && len(processedArgs) == 1 {
					// It's an alias, use the command from the alias
					serverCommands = append(serverCommands, client.ParseCommandString(aliasCmd))
				} else {
					// Not an alias, use the args directly
					serverCommands = append(serverCommands, processedArgs)
				}
			}

			// If no server commands, error out
			if len(serverCommands) == 0 {
				return fmt.Errorf("at least one server command is required")
			}

			// Set default provider and model
			provider := providerFlag
			if provider == "" {
				provider = LLMProviderOpenAI
			}

			model := modelFlag
			if model == "" {
				// Default models based on provider
				switch provider {
				case LLMProviderOpenAI:
					model = "gpt-4o"
				case LLMProviderAnthropic:
					model = "claude-3-7-sonnet-20250219"
				}
			}

			// Get API key
			apiKey := apiKeyFlag
			if apiKey == "" {
				switch provider {
				case LLMProviderOpenAI:
					apiKey = os.Getenv("OPENAI_API_KEY")
				case LLMProviderAnthropic:
					apiKey = os.Getenv("ANTHROPIC_API_KEY")
				}
			}

			if apiKey == "" {
				return fmt.Errorf("no API key provided for %s. Please set the appropriate environment variable or use --api-key", provider)
			}

			// Create MCP clients for each server
			var mcpClients []*client.Client
			var allTools []Tool

			// Print colorful header
			mcpColor.Fprintf(thisCmd.OutOrStdout(), "mcp > MCP LLM Shell (%s)\n", Version)

			// Connect to each server and collect tools
			for i, serverCmd := range serverCommands {
				serverColor.Fprintf(thisCmd.OutOrStdout(), "mcp > Connecting to server %d: %s\n", i+1, strings.Join(serverCmd, " "))

				mcpClient, clientErr := CreateClientFunc(serverCmd, client.CloseTransportAfterExecute(false))
				if clientErr != nil {
					errorColor.Fprintf(os.Stderr, "Error creating MCP client for server %d: %v\n", i+1, clientErr)
					return fmt.Errorf("error creating MCP client for server %d: %w", i+1, clientErr)
				}

				// Verify the connection to the MCP server
				toolsResp, listErr := mcpClient.ListTools()
				if listErr != nil {
					errorColor.Fprintf(os.Stderr, "Error connecting to MCP server %d: %v\n", i+1, listErr)
					return fmt.Errorf("error connecting to MCP server %d: %w", i+1, listErr)
				}

				// Extract and format tools for LLM function/tool calling
				serverTools, err := formatToolsForLLM(toolsResp)
				if err != nil {
					errorColor.Fprintf(os.Stderr, "Error formatting tools from server %d: %v\n", i+1, err)
					return fmt.Errorf("error formatting tools from server %d: %w", i+1, err)
				}

				// Tag tools with server prefix to avoid name collisions
				prefix := fmt.Sprintf("s%d_", i+1)
				for j := range serverTools {
					// Update tool name with server prefix
					serverTools[j].Name = prefix + serverTools[j].Name
					// Add server ID to description
					serverTools[j].Description = fmt.Sprintf("[Server %d] %s", i+1, serverTools[j].Description)
				}

				serverColor.Fprintf(thisCmd.OutOrStdout(), "mcp > Server %d: Registered %d tools\n", i+1, len(serverTools))

				// Store client and tools
				mcpClients = append(mcpClients, mcpClient)
				allTools = append(allTools, serverTools...)
			}

			// Get the username for the prompt
			username := "user"
			if u := os.Getenv("USER"); u != "" {
				username = u
			} else if u := os.Getenv("USERNAME"); u != "" { // For Windows
				username = u
			}

			mcpColor.Fprintf(thisCmd.OutOrStdout(), "mcp > Using provider: %s, model: %s\n", provider, model)
			mcpColor.Fprintf(thisCmd.OutOrStdout(), "mcp > Total registered tools: %d\n", len(allTools))
			mcpColor.Fprintf(thisCmd.OutOrStdout(), "mcp > Type 'exit' to quit\n\n")

			// Initialize chat session with a special message type for proper handling
			messages := []map[string]interface{}{
				{
					"role": "system",
					"content": fmt.Sprintf(`You are a helpful AI assistant with access to tools from %d different servers.
Tools from each server are prefixed with "s<server_number>_". For example, tools from server 1 are prefixed with "s1_".
Use these tools when appropriate to perform actions for the user.
Be concise, accurate, and helpful.`, len(serverCommands)),
				},
			}

			// Setup liner for command history and line editing
			line := liner.NewLiner()
			line.SetCtrlCAborts(true)
			defer func() { _ = line.Close() }()

			// Setup history file
			defer setupHistory(line)()

			// Add simple completion for common commands
			line.SetCompleter(func(line string) (c []string) {
				commands := []string{
					"exit",
					"quit",
					"help",
				}
				for _, cmd := range commands {
					if strings.HasPrefix(cmd, line) {
						c = append(c, cmd)
					}
				}
				return
			})

			// Custom writer that colorizes agent output
			agentWriter := &colorWriter{
				out:       thisCmd.OutOrStdout(),
				baseColor: agentColor,
			}

			// Start interactive chat loop
			for {
				// Create prompt with username
				prompt := fmt.Sprintf("%s > ", username)

				// Use liner to get input with history and editing
				// Apply color to prompt (need to temporarily disable for liner)
				color.NoColor = true // Disable color for prompt to avoid liner issues
				input, err := line.Prompt(prompt)
				color.NoColor = noColorFlag // Restore color setting

				if err != nil {
					if err == liner.ErrPromptAborted {
						mcpColor.Fprintf(thisCmd.OutOrStdout(), "\nExiting LLM shell\n")
						break
					}
					errorColor.Fprintf(os.Stderr, "Error reading input: %v\n", err)
					break
				}

				// Skip empty input
				if input == "" {
					continue
				}

				// Add to history
				line.AppendHistory(input)

				// Check for exit commands
				if input == "exit" || input == "quit" {
					mcpColor.Fprintf(thisCmd.OutOrStdout(), "Exiting LLM shell\n")
					break
				}

				// Add user message to the conversation
				messages = append(messages, map[string]interface{}{
					"role":    "user",
					"content": input,
				})

				// Call the LLM API and stream the response
				agentColor.Fprintf(thisCmd.OutOrStdout(), "agent > ")
				responseMessages, err := callLLMWithMultipleTools(provider, model, apiKey, messages, allTools, mcpClients, agentWriter)
				if err != nil {
					errorColor.Fprintf(os.Stderr, "\nError calling LLM: %v\n", err)
					continue
				}

				// Add all messages returned by the function to the conversation
				messages = append(messages, responseMessages...)

				fmt.Println()
			}

			return nil
		},
	}

	return cmd
}

// colorWriter is a custom io.Writer that colorizes output.
type colorWriter struct {
	out       io.Writer
	baseColor *color.Color
}

// Write implements io.Writer.
func (cw *colorWriter) Write(p []byte) (n int, err error) {
	// Use the color to write the output
	return cw.baseColor.Fprint(cw.out, string(p))
}

// setupHistory sets up command history for the liner.
func setupHistory(line *liner.State) func() {
	// Create history directory if it doesn't exist
	historyDir := filepath.Join(os.Getenv("HOME"), ".config", "mcp")
	_ = os.MkdirAll(historyDir, 0o755)

	// History file path
	historyFile := filepath.Join(historyDir, ".mcp_llm_history")

	// Load history from file if it exists
	if f, err := os.Open(filepath.Clean(historyFile)); err == nil {
		_, _ = line.ReadHistory(f)
		_ = f.Close()
	}

	// Return a function that will save history on exit
	return func() {
		if f, err := os.Create(historyFile); err == nil {
			_, _ = line.WriteHistory(f)
			_ = f.Close()
		}
	}
}

// formatToolsForLLM converts MCP tools to a format suitable for LLM function calling.
func formatToolsForLLM(toolsResp map[string]interface{}) ([]Tool, error) {
	toolsData, ok := toolsResp["tools"]
	if !ok {
		return nil, fmt.Errorf("no tools found in response")
	}

	toolsList, ok := toolsData.([]interface{})
	if !ok {
		return nil, fmt.Errorf("tools data is not in expected format")
	}

	var tools []Tool
	for _, item := range toolsList {
		toolMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := toolMap["name"].(string)
		description, _ := toolMap["description"].(string)

		// Extract parameters from schema if available
		var parameters map[string]interface{}

		// Try inputSchema first, then fall back to schema for backward compatibility
		var schema map[string]interface{}
		if inputSchema, ok := toolMap["inputSchema"].(map[string]interface{}); ok {
			schema = inputSchema
		} else if oldSchema, ok := toolMap["schema"].(map[string]interface{}); ok {
			schema = oldSchema
		}

		if schema != nil {
			if props, ok := schema["properties"].(map[string]interface{}); ok && len(props) > 0 {
				// Deep copy the properties and ensure descriptions are included
				propsCopy := make(map[string]interface{})
				for propName, propValue := range props {
					propMap, ok := propValue.(map[string]interface{})
					if !ok {
						// If propValue is not a map, create a simple type object
						propsCopy[propName] = map[string]interface{}{
							"type":        "string",
							"description": fmt.Sprintf("%s parameter", propName),
						}
						continue
					}

					// Ensure all property attributes are copied correctly
					propMapCopy := make(map[string]interface{})
					for k, v := range propMap {
						propMapCopy[k] = v
					}

					// Ensure type is present and valid
					if _, hasType := propMapCopy["type"]; !hasType {
						propMapCopy["type"] = "string"
					}

					// Ensure arrays have items defined
					if propType, ok := propMapCopy["type"].(string); ok && propType == "array" {
						if _, hasItems := propMapCopy["items"]; !hasItems {
							propMapCopy["items"] = map[string]interface{}{
								"type": "string",
							}
						}
					}

					// Ensure description exists
					if _, hasDesc := propMapCopy["description"]; !hasDesc {
						propMapCopy["description"] = fmt.Sprintf("%s parameter", propName)
					}

					propsCopy[propName] = propMapCopy
				}

				// Only add parameters if there are properties
				if len(propsCopy) > 0 {
					parameters = map[string]interface{}{
						"type":       "object",
						"properties": propsCopy,
					}

					// Add required fields if available
					if required, ok := schema["required"].([]interface{}); ok {
						var requiredArr []string
						for _, r := range required {
							if s, ok := r.(string); ok {
								requiredArr = append(requiredArr, s)
							}
						}
						parameters["required"] = requiredArr
					}
				}
			}
		}

		// Only create a tool if we have a name
		if name != "" {
			tool := Tool{
				Name:        name,
				Description: description,
			}

			// Only add parameters if they exist
			if parameters != nil {
				tool.Parameters = parameters
			}

			tools = append(tools, tool)
		}
	}

	return tools, nil
}

// callLLMWithMultipleTools calls the LLM with tools from multiple servers.
func callLLMWithMultipleTools(provider, model, apiKey string, messages []map[string]interface{}, tools []Tool, clients []*client.Client, out io.Writer) ([]map[string]interface{}, error) {
	// Modified function to handle tool calls from multiple servers
	switch provider {
	case LLMProviderOpenAI:
		return callOpenAIWithMultipleTools(model, apiKey, messages, tools, clients, out)
	case LLMProviderAnthropic:
		return callAnthropicWithMultipleTools(model, apiKey, messages, tools, clients, out)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// callOpenAIWithMultipleTools calls OpenAI API with tools from multiple servers.
func callOpenAIWithMultipleTools(model, apiKey string, messages []map[string]interface{}, tools []Tool, clients []*client.Client, out io.Writer) ([]map[string]interface{}, error) {
	var responseMessages []map[string]interface{}
	var fullContent strings.Builder

	// Format tools for OpenAI
	openAITools := []map[string]interface{}{}
	for _, tool := range tools {
		openAITools = append(openAITools, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.Parameters,
			},
		})
	}

	// Convert messages to OpenAI format if needed
	requestMessages := convertMessages(messages)

	// Prepare request
	requestData := map[string]interface{}{
		"model":       model,
		"messages":    requestMessages,
		"tools":       openAITools,
		"tool_choice": "auto",
		"stream":      true,
	}

	requestBody, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", strings.NewReader(string(requestBody)))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	scanner := bufio.NewScanner(resp.Body)

	var toolCalls []map[string]interface{}
	var assistantMessage map[string]interface{}
	var toolCallsComplete bool

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			line = strings.TrimPrefix(line, "data: ")

			if line == "[DONE]" {
				break
			}

			var data map[string]interface{}
			if err := json.Unmarshal([]byte(line), &data); err != nil {
				continue
			}

			if choices, ok := data["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if delta, ok := choice["delta"].(map[string]interface{}); ok {
						// Initialize assistant message if needed
						if assistantMessage == nil {
							assistantMessage = map[string]interface{}{
								"role":       "assistant",
								"content":    "",
								"tool_calls": []map[string]interface{}{},
							}
						}

						// Handle content
						if content, ok := delta["content"].(string); ok {
							fmt.Fprint(out, content)
							fullContent.WriteString(content)
							currentContent, _ := assistantMessage["content"].(string)
							assistantMessage["content"] = currentContent + content
						}

						// Handle tool calls
						if toolCallDelta, ok := delta["tool_calls"].([]interface{}); ok && len(toolCallDelta) > 0 {
							for _, tc := range toolCallDelta {
								if tcDelta, ok := tc.(map[string]interface{}); ok {
									// Get the index for this tool call
									index, _ := tcDelta["index"].(float64)
									indexInt := int(index)

									// Ensure we have enough elements in toolCalls
									for len(toolCalls) <= indexInt {
										toolCalls = append(toolCalls, map[string]interface{}{
											"id":   "",
											"type": "function",
											"function": map[string]interface{}{
												"name":      "",
												"arguments": "",
											},
										})
									}

									// Ensure we have enough elements in the assistant message tool_calls
									toolCallsArr, _ := assistantMessage["tool_calls"].([]map[string]interface{})
									for len(toolCallsArr) <= indexInt {
										toolCallsArr = append(toolCallsArr, map[string]interface{}{
											"id":   "",
											"type": "function",
											"function": map[string]interface{}{
												"name":      "",
												"arguments": "",
											},
										})
									}
									assistantMessage["tool_calls"] = toolCallsArr

									// Update the tool call with this delta
									if id, ok := tcDelta["id"].(string); ok && id != "" {
										toolCalls[indexInt]["id"] = id
										toolCallsArr[indexInt]["id"] = id
									}

									if function, ok := tcDelta["function"].(map[string]interface{}); ok {
										toolCall := toolCalls[indexInt]["function"].(map[string]interface{})
										toolCallInAssistant := toolCallsArr[indexInt]["function"].(map[string]interface{})

										if name, ok := function["name"].(string); ok && name != "" {
											toolCall["name"] = name
											toolCallInAssistant["name"] = name

											// If this is the first time we're seeing the name, print it
											currentName, _ := toolCall["name"].(string)
											if currentName == name {
												// Switch to tool call color
												fmt.Fprint(out, "\n")
												toolCallColor.Fprintf(color.Output, "[Calling %s", name)
											}
										}

										if arguments, ok := function["arguments"].(string); ok && arguments != "" {
											current, _ := toolCall["arguments"].(string)
											toolCall["arguments"] = current + arguments

											currentInAssistant, _ := toolCallInAssistant["arguments"].(string)
											toolCallInAssistant["arguments"] = currentInAssistant + arguments
										}
									}
								}
							}
						}

						// Check if done with tool calls
						if finish, ok := choice["finish_reason"].(string); ok && finish == "tool_calls" {
							toolCallsComplete = true
							break
						}
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	// Process and execute tool calls if we have any
	if toolCallsComplete && len(toolCalls) > 0 {
		// Close the tool call display
		toolCallColor.Fprint(color.Output, "]")

		// First add the assistant message to our responses with proper tool_calls
		responseMessages = append(responseMessages, assistantMessage)

		// Now handle each tool call and execute it
		var toolResponses []map[string]interface{}

		for _, toolCall := range toolCalls {
			functionData := toolCall["function"].(map[string]interface{})
			toolName, _ := functionData["name"].(string)
			toolArgsStr, _ := functionData["arguments"].(string)

			var toolArgs map[string]interface{}
			if err := json.Unmarshal([]byte(toolArgsStr), &toolArgs); err != nil {
				errorColor.Fprintf(color.Output, "\nmcp > Error parsing tool arguments: %v\n", err)
				toolResponses = append(toolResponses, map[string]interface{}{
					"role":         "tool",
					"tool_call_id": toolCall["id"].(string),
					"content":      fmt.Sprintf("Error parsing arguments: %v", err),
				})
				continue
			}

			// Extract server ID from tool name (e.g., "s1_read_file" -> serverIdx = 0)
			var serverIdx int
			var baseToolName string

			if len(toolName) >= 3 && toolName[0] == 's' && toolName[2] == '_' {
				serverChar := toolName[1]
				if serverChar >= '1' && serverChar <= '9' {
					serverIdx = int(serverChar - '1')
					baseToolName = toolName[3:]
				} else {
					baseToolName = toolName
				}
			} else {
				baseToolName = toolName
			}

			// Check if the server index is valid
			if serverIdx < 0 || serverIdx >= len(clients) {
				errorColor.Fprintf(color.Output, "\nmcp > Error: Invalid server index in tool name: %s\n", toolName)
				toolResponses = append(toolResponses, map[string]interface{}{
					"role":         "tool",
					"tool_call_id": toolCall["id"].(string),
					"content":      fmt.Sprintf("Error: Invalid server index in tool name: %s", toolName),
				})
				continue
			}

			// Execute tool on the appropriate server
			mcpColor.Fprintf(color.Output, "\nmcp > ")
			toolCallColor.Fprintf(color.Output, "[Server %d running %s", serverIdx+1, baseToolName)
			serverColor.Fprintf(color.Output, " with params %s]\n", toolArgsStr)

			result, err := clients[serverIdx].CallTool(baseToolName, toolArgs)
			if err != nil {
				errorColor.Fprintf(color.Output, "mcp > Error executing tool: %v\n", err)
				toolResponses = append(toolResponses, map[string]interface{}{
					"role":         "tool",
					"tool_call_id": toolCall["id"].(string),
					"content":      fmt.Sprintf("Error: %v", err),
				})
			} else {
				// Format result as JSON
				resultBytes, _ := json.MarshalIndent(result, "", "  ")
				resultStr := string(resultBytes)

				toolResultColor.Fprintf(color.Output, "%s\n", resultStr)

				toolResponses = append(toolResponses, map[string]interface{}{
					"role":         "tool",
					"tool_call_id": toolCall["id"].(string),
					"content":      resultStr,
				})
			}
		}

		// Add all tool responses
		responseMessages = append(responseMessages, toolResponses...)

		// Make another call to get the AI's final response after tools
		finalMessages, err := callOpenAIFinalResponse(model, apiKey, append(messages, responseMessages...), out)
		if err != nil {
			return nil, err
		}

		return append(responseMessages, finalMessages...), nil
	}

	// Add the assistant message if we didn't execute any tools
	if len(responseMessages) == 0 && fullContent.Len() > 0 {
		responseMessages = append(responseMessages, map[string]interface{}{
			"role":    "assistant",
			"content": fullContent.String(),
		})
	}

	return responseMessages, nil
}

// callOpenAIFinalResponse calls OpenAI to get final response after tool execution.
func callOpenAIFinalResponse(model, apiKey string, messages []map[string]interface{}, out io.Writer) ([]map[string]interface{}, error) {
	// Prepare request
	requestData := map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   true,
	}

	requestBody, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", strings.NewReader(string(requestBody)))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var fullContent strings.Builder
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			line = strings.TrimPrefix(line, "data: ")

			if line == "[DONE]" {
				break
			}

			var data map[string]interface{}
			if err := json.Unmarshal([]byte(line), &data); err != nil {
				continue
			}

			if choices, ok := data["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if delta, ok := choice["delta"].(map[string]interface{}); ok {
						if content, ok := delta["content"].(string); ok {
							fmt.Fprint(out, content)
							fullContent.WriteString(content)
						}
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if fullContent.Len() > 0 {
		return []map[string]interface{}{
			{
				"role":    "assistant",
				"content": fullContent.String(),
			},
		}, nil
	}

	return nil, nil
}

// callAnthropicWithMultipleTools calls Anthropic API with tools from multiple servers.
func callAnthropicWithMultipleTools(model, apiKey string, messages []map[string]interface{}, tools []Tool, clients []*client.Client, out io.Writer) ([]map[string]interface{}, error) {
	// For brevity, we'll reuse the existing Anthropic implementation but add a warning
	serverColor.Fprintf(color.Output, "Note: Multi-server support is limited for Anthropic. Only using tools from server 1.\n")
	return callAnthropicWithTools(model, apiKey, messages, tools, clients[0], out)
}

// callAnthropicWithTools calls Anthropic API with tools support.
func callAnthropicWithTools(model, apiKey string, messages []map[string]interface{}, tools []Tool, mcpClient *client.Client, out io.Writer) ([]map[string]interface{}, error) {
	var responseMessages []map[string]interface{}

	// Extract system message
	systemPrompt := ""
	var anthropicMessages []map[string]interface{}

	for _, msg := range messages {
		role, _ := msg["role"].(string)
		if role == "system" {
			content, _ := msg["content"].(string)
			systemPrompt = content
		} else {
			// Convert tool messages to appropriate format for Anthropic
			if role == "tool" {
				name, _ := msg["name"].(string)
				content, _ := msg["content"].(string)
				anthropicMessages = append(anthropicMessages, map[string]interface{}{
					"role":    "assistant",
					"content": fmt.Sprintf("Tool result from %s: %s", name, content),
				})
			} else {
				anthropicMessages = append(anthropicMessages, msg)
			}
		}
	}

	// Format tools for Anthropic
	anthropicTools := []map[string]interface{}{}
	for _, tool := range tools {
		// Skip tools without parameters for Anthropic
		if tool.Parameters == nil {
			serverColor.Fprintf(color.Output, "Skipping tool %s for Anthropic (no parameters)\n", tool.Name)
			continue
		}

		// Verify we have properties to work with
		props, hasProps := tool.Parameters["properties"].(map[string]interface{})
		if !hasProps || len(props) == 0 {
			serverColor.Fprintf(color.Output, "Skipping tool %s for Anthropic (no properties)\n", tool.Name)
			continue
		}

		// Format the parameters specifically for Anthropic's expected schema structure
		inputSchema := map[string]interface{}{
			"type":       "object",
			"properties": props,
		}

		// Add required field if present
		if required, ok := tool.Parameters["required"].([]string); ok && len(required) > 0 {
			inputSchema["required"] = required
		}

		anthropicTools = append(anthropicTools, map[string]interface{}{
			"name":         tool.Name,
			"description":  tool.Description,
			"input_schema": inputSchema,
		})
	}

	// Note how many tools we included
	serverColor.Fprintf(color.Output, "Registered %d tools for Anthropic (out of %d total)\n", len(anthropicTools), len(tools))

	// Debug: Log the first tool format if any exist
	if len(anthropicTools) > 0 {
		debugBytes, _ := json.MarshalIndent(anthropicTools[0], "", "  ")
		serverColor.Fprintf(color.Output, "Sample Anthropic tool format: %s\n", string(debugBytes))
	}

	// Prepare request
	requestData := map[string]interface{}{
		"model":    model,
		"messages": anthropicMessages,
		"system":   systemPrompt,
		"tools":    anthropicTools,
		"stream":   true,
	}

	requestBody, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", strings.NewReader(string(requestBody)))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	scanner := bufio.NewScanner(resp.Body)
	var toolUses []map[string]interface{}
	var currentToolUse map[string]interface{}
	var assistantContent string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			line = strings.TrimPrefix(line, "data: ")

			var data map[string]interface{}
			if err := json.Unmarshal([]byte(line), &data); err != nil {
				continue
			}

			// Handle content blocks
			if contentBlocks, ok := data["delta"].(map[string]interface{})["content_blocks"].([]interface{}); ok && len(contentBlocks) > 0 {
				for _, block := range contentBlocks {
					if contentBlock, ok := block.(map[string]interface{}); ok {
						if text, ok := contentBlock["text"].(string); ok {
							fmt.Fprint(out, text)
							assistantContent += text
						}
					}
				}
			}

			// Handle tool use
			if delta, ok := data["delta"].(map[string]interface{}); ok {
				if usage, ok := delta["tool_use"].(map[string]interface{}); ok {
					// Initialize tool use if needed
					if currentToolUse == nil {
						currentToolUse = map[string]interface{}{
							"id":    usage["id"],
							"type":  "tool_use",
							"name":  usage["name"],
							"input": "",
						}

						// Print tool name when we first encounter it
						fmt.Fprintf(out, "\n[Calling %s", currentToolUse["name"])
					}

					// Append to input if present
					if input, ok := usage["input"].(string); ok {
						current := currentToolUse["input"].(string)
						currentToolUse["input"] = current + input
					}

					// Check if this tool is complete
					if id, ok := usage["id"].(string); ok && id != "" {
						if _, exists := findToolUseById(toolUses, id); !exists {
							// New completed tool, add to our list
							if name, ok := usage["name"].(string); ok && name != "" {
								toolUse := map[string]interface{}{
									"id":    id,
									"name":  name,
									"input": currentToolUse["input"].(string),
								}
								toolUses = append(toolUses, toolUse)

								// Reset current tool use for the next one
								currentToolUse = nil
							}
						}
					}
				}
			}

			// Handle message stop
			if typ, ok := data["type"].(string); ok && typ == "message_stop" {
				// We're done parsing the message
				break
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	// Add assistant message with content
	if assistantContent != "" {
		responseMessages = append(responseMessages, map[string]interface{}{
			"role":    "assistant",
			"content": assistantContent,
		})
	}

	// Process all tool uses if we have any
	if len(toolUses) > 0 {
		// Close the tool call display
		fmt.Fprint(out, "]")

		// Process and execute each tool
		var toolResponses []map[string]interface{}

		for _, toolUse := range toolUses {
			toolName := toolUse["name"].(string)
			toolInputStr := toolUse["input"].(string)

			var toolArgs map[string]interface{}
			if err := json.Unmarshal([]byte(toolInputStr), &toolArgs); err != nil {
				fmt.Fprintf(out, "\nmcp > Error parsing tool arguments: %v\n", err)
				continue
			}

			// Execute tool
			fmt.Fprintf(out, "\nmcp > [running %s with params %s]\n", toolName, toolInputStr)

			result, err := mcpClient.CallTool(toolName, toolArgs)
			if err != nil {
				fmt.Fprintf(out, "mcp > Error executing tool: %v\n", err)
			} else {
				// Format result as JSON
				resultBytes, _ := json.MarshalIndent(result, "", "  ")
				resultStr := string(resultBytes)

				fmt.Fprint(out, resultStr+"\n")

				// Add tool result as a message
				toolResponses = append(toolResponses, map[string]interface{}{
					"role":    "user",
					"content": fmt.Sprintf("Tool result from %s: %s", toolName, resultStr),
				})
			}
		}

		// Add all tool responses
		responseMessages = append(responseMessages, toolResponses...)

		// Get the final response after tool execution
		finalMessages, err := callAnthropicFinalResponse(model, apiKey, append(messages, responseMessages...), out)
		if err != nil {
			return nil, err
		}

		return append(responseMessages, finalMessages...), nil
	}

	// If we got here without executing tools, return the full content
	if len(responseMessages) == 0 && assistantContent != "" {
		responseMessages = append(responseMessages, map[string]interface{}{
			"role":    "assistant",
			"content": assistantContent,
		})
	}

	return responseMessages, nil
}

// findToolUseById finds a tool use by its ID.
func findToolUseById(toolUses []map[string]interface{}, id string) (map[string]interface{}, bool) {
	for _, toolUse := range toolUses {
		if toolId, ok := toolUse["id"].(string); ok && toolId == id {
			return toolUse, true
		}
	}
	return nil, false
}

// callAnthropicFinalResponse calls Anthropic to get final response after tool execution.
func callAnthropicFinalResponse(model, apiKey string, messages []map[string]interface{}, out io.Writer) ([]map[string]interface{}, error) {
	// Extract system message and format messages
	systemPrompt := ""
	var anthropicMessages []map[string]interface{}

	for _, msg := range messages {
		role, _ := msg["role"].(string)
		if role == "system" {
			content, _ := msg["content"].(string)
			systemPrompt = content
		} else {
			anthropicMessages = append(anthropicMessages, msg)
		}
	}

	// Prepare request
	requestData := map[string]interface{}{
		"model":    model,
		"messages": anthropicMessages,
		"system":   systemPrompt,
		"stream":   true,
	}

	requestBody, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", strings.NewReader(string(requestBody)))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var fullContent strings.Builder
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			line = strings.TrimPrefix(line, "data: ")

			var data map[string]interface{}
			if err := json.Unmarshal([]byte(line), &data); err != nil {
				continue
			}

			// Handle content blocks
			if contentBlocks, ok := data["delta"].(map[string]interface{})["content_blocks"].([]interface{}); ok && len(contentBlocks) > 0 {
				for _, block := range contentBlocks {
					if contentBlock, ok := block.(map[string]interface{}); ok {
						if text, ok := contentBlock["text"].(string); ok {
							fmt.Fprint(out, text)
							fullContent.WriteString(text)
						}
					}
				}
			}

			// Handle message stop
			if typ, ok := data["type"].(string); ok && typ == "message_stop" {
				break
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if fullContent.Len() > 0 {
		return []map[string]interface{}{
			{
				"role":    "assistant",
				"content": fullContent.String(),
			},
		}, nil
	}

	return nil, nil
}

// convertMessages converts between different message formats if needed.
func convertMessages(messages []map[string]interface{}) []map[string]interface{} {
	convertedMessages := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		convertedMsg := make(map[string]interface{})
		for k, v := range msg {
			convertedMsg[k] = v
		}
		convertedMessages = append(convertedMessages, convertedMsg)
	}
	return convertedMessages
}

// processArgs manually processes the arguments to separate flags from positional args
func processArgs(args []string, providerFlag, modelFlag, apiKeyFlag *string, multiServerFlags *[]string, noColorFlag *bool) ([]string, error) {
	var processedArgs []string

	// Check for -- separator
	dashDashIndex := -1
	for i, arg := range args {
		if arg == "--" {
			dashDashIndex = i
			break
		}
	}

	// If -- separator is found, treat everything after it as positional args
	if dashDashIndex != -1 {
		// Process flags before the separator
		flagArgs := args[:dashDashIndex]
		if err := parseFlags(flagArgs, providerFlag, modelFlag, apiKeyFlag, multiServerFlags, noColorFlag); err != nil {
			return nil, err
		}

		// Everything after -- is treated as server command
		if dashDashIndex+1 < len(args) {
			processedArgs = args[dashDashIndex+1:]
		}
		return processedArgs, nil
	}

	// No -- separator, try to auto-detect flags vs positional args
	var flagArgs []string
	i := 0
	for i < len(args) {
		arg := args[i]

		// If argument starts with - or --, it's a flag
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)

			// Check if the flag needs a value
			if arg == "-M" || arg == "--multi" ||
				arg == "--provider" || arg == "--model" ||
				arg == "--api-key" {
				if i+1 < len(args) {
					flagArgs = append(flagArgs, args[i+1])
					i += 2
					continue
				}
			}
			i++
			continue
		}

		// If we reach here, it's not a flag, so it must be the start of the server command
		break
	}

	// Parse the flags we've collected
	if err := parseFlags(flagArgs, providerFlag, modelFlag, apiKeyFlag, multiServerFlags, noColorFlag); err != nil {
		return nil, err
	}

	// Everything else is treated as the server command
	if i < len(args) {
		processedArgs = args[i:]
	}

	return processedArgs, nil
}

// parseFlags parses the flag arguments and sets the corresponding values
func parseFlags(flagArgs []string, providerFlag, modelFlag, apiKeyFlag *string, multiServerFlags *[]string, noColorFlag *bool) error {
	for i := 0; i < len(flagArgs); i++ {
		switch flagArgs[i] {
		case "--provider":
			if i+1 < len(flagArgs) {
				*providerFlag = flagArgs[i+1]
				i++
			} else {
				return fmt.Errorf("--provider requires a value")
			}
		case "--model":
			if i+1 < len(flagArgs) {
				*modelFlag = flagArgs[i+1]
				i++
			} else {
				return fmt.Errorf("--model requires a value")
			}
		case "--api-key":
			if i+1 < len(flagArgs) {
				*apiKeyFlag = flagArgs[i+1]
				i++
			} else {
				return fmt.Errorf("--api-key requires a value")
			}
		case "-M", "--multi":
			if i+1 < len(flagArgs) {
				*multiServerFlags = append(*multiServerFlags, flagArgs[i+1])
				i++
			} else {
				return fmt.Errorf("--multi requires a value")
			}
		case "--no-color":
			*noColorFlag = true
		}
	}

	return nil
}
