package commands

import (
	"fmt"
	"os"

	"github.com/f/mcptools/pkg/mock"
	"github.com/spf13/cobra"
)

// MockCmd creates the mock command.
func MockCmd() *cobra.Command {
	var port string
	
	cmd := &cobra.Command{
		Use:   "mock [type] [name] [description] [content]...",
		Short: "Create a mock MCP server with tools, prompts, and resources",
		Long: `Create a mock MCP server with tools, prompts, and resources.
This is useful for testing MCP clients without implementing a full server.

The mock server implements the MCP protocol with:
- Full initialization handshake (initialize method)
- Support for notifications/initialized notification
- Tool listing with standardized schema format
- Tool calling with simple responses
- Resource listing and reading with proper format
- Prompt listing and retrieving with proper format
- Standard error codes (-32601 for method not found)
- Detailed request/response logging to ~/.mcpt/logs/mock.log

Available types:
- tool <name> <description>
- prompt <name> <description> <template>
- resource <uri> <description> <content>

Transport options:
- Default: stdin/stdout (stdio transport)
- --port <port>: HTTP SSE transport on specified port

Examples:
  # Stdio transport (default)
  mcp mock tool hello_world "when user says hello world, run this tool"

  # HTTP transport
  mcp mock --port 3000 tool hello_world "A greeting tool" \
         prompt welcome "A welcome prompt" "Hello {{name}}, welcome to {{location}}!" \
         resource docs:readme "Documentation" "# Mock MCP Server\nThis is a mock server"

  # Then connect to http://localhost:3000/sse`,
		Args: cobra.MinimumNArgs(2),
		Run: func(_ *cobra.Command, args []string) {
			tools := make(map[string]string)
			prompts := make(map[string]map[string]string)
			resources := make(map[string]map[string]string)

			i := 0
			for i < len(args) {
				entityType := args[i]
				i++

				switch entityType {
				case EntityTypeTool:
					if i+1 >= len(args) {
						fmt.Fprintln(os.Stderr, "Error: each tool must have both a name and description")
						fmt.Fprintln(os.Stderr, "Example: mcp mock tool hello_world \"when user says hello world, run this tool\"")
						os.Exit(1)
					}

					toolName := args[i]
					toolDescription := args[i+1]
					tools[toolName] = toolDescription
					fmt.Fprintf(os.Stderr, "Added tool: %s - %s\n", toolName, toolDescription)
					i += 2

				case EntityTypePrompt:
					if i+2 >= len(args) {
						fmt.Fprintln(os.Stderr, "Error: each prompt must have a name, description, and template")
						fmt.Fprintln(os.Stderr, "Example: mcp mock prompt welcome \"Welcome message\" \"Hello {{name}}!\"")
						os.Exit(1)
					}

					promptName := args[i]
					promptDescription := args[i+1]
					promptTemplate := args[i+2]

					prompts[promptName] = map[string]string{
						"description": promptDescription,
						"template":    promptTemplate,
					}

					fmt.Fprintf(os.Stderr, "Added prompt: %s - %s\n", promptName, promptDescription)
					i += 3

				case EntityTypeRes:
					if i+2 >= len(args) {
						fmt.Fprintln(os.Stderr, "Error: each resource must have a URI, description, and content")
						fmt.Fprintln(os.Stderr, "Example: mcp mock resource docs:readme \"Documentation\" \"# README\"")
						os.Exit(1)
					}

					resourceURI := args[i]
					resourceDescription := args[i+1]
					resourceContent := args[i+2]

					resources[resourceURI] = map[string]string{
						"description": resourceDescription,
						"content":     resourceContent,
					}

					fmt.Fprintf(os.Stderr, "Added resource: %s - %s\n", resourceURI, resourceDescription)
					i += 3

				default:
					fmt.Fprintf(os.Stderr, "Error: unknown entity type: %s\n", entityType)
					fmt.Fprintln(os.Stderr, "Available types: tool, prompt, resource")
					os.Exit(1)
				}
			}

			if len(tools) == 0 && len(prompts) == 0 && len(resources) == 0 {
				fmt.Fprintln(os.Stderr, "Error: at least one tool, prompt, or resource must be specified")
				os.Exit(1)
			}

			if port != "" {
				fmt.Fprintf(os.Stderr, "Starting HTTP mock MCP server with %d tool(s), %d prompt(s), and %d resource(s) on port %s\n",
					len(tools), len(prompts), len(resources), port)
				fmt.Fprintf(os.Stderr, "JSON-RPC endpoint: http://localhost:%s/mcp\n", port)
				fmt.Fprintf(os.Stderr, "SSE endpoint: http://localhost:%s/sse (for SSE-compatible clients)\n", port)
				fmt.Fprintf(os.Stderr, "Health check: http://localhost:%s/health\n", port)
				fmt.Fprintf(os.Stderr, "Use Ctrl+C to exit\n")

				if err := mock.RunMockServerHTTP(tools, prompts, resources, port); err != nil {
					fmt.Fprintf(os.Stderr, "Error running HTTP mock server: %v\n", err)
					os.Exit(1)
				}
			} else {
				fmt.Fprintf(os.Stderr, "Starting mock MCP server with %d tool(s), %d prompt(s), and %d resource(s)\n",
					len(tools), len(prompts), len(resources))
				fmt.Fprintf(os.Stderr, "Use Ctrl+C to exit\n")

				if err := mock.RunMockServer(tools, prompts, resources); err != nil {
					fmt.Fprintf(os.Stderr, "Error running mock server: %v\n", err)
					os.Exit(1)
				}
			}
		},
	}

	cmd.Flags().StringVar(&port, "port", "", "Start HTTP server on specified port (e.g., 3000). If not specified, uses stdio transport.")

	return cmd
}
