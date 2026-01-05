package commands

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/f/mcptools/pkg/alias"
	"github.com/f/mcptools/pkg/jsonutils"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/spf13/cobra"
)

// sentinel errors.
var (
	ErrCommandRequired = fmt.Errorf("command to execute is required when using stdio transport")
)

// IsHTTP returns true if the string is a valid HTTP URL.
func IsHTTP(str string) bool {
	return strings.HasPrefix(str, "http://") || strings.HasPrefix(str, "https://") || strings.HasPrefix(str, "localhost:")
}

// validConfigPrefixes defines the valid config source prefixes.
var validConfigPrefixes = map[string]bool{
	"vscode":          true,
	"vscode-insiders": true,
	"windsurf":        true,
	"cursor":          true,
	"claude-desktop":  true,
	"claude-code":     true,
}

// ParseConfigPrefix parses a config source prefix from a server specification.
// Examples:
//
//	"claude-code:firecrawl" -> ("claude-code", "firecrawl", true)
//	"cursor:myserver"       -> ("cursor", "myserver", true)
//	"myalias"               -> ("", "myalias", false)
func ParseConfigPrefix(serverSpec string) (configSource, serverName string, hasPrefix bool) {
	if idx := strings.Index(serverSpec, ":"); idx > 0 {
		prefix := serverSpec[:idx]
		if validConfigPrefixes[strings.ToLower(prefix)] {
			return strings.ToLower(prefix), serverSpec[idx+1:], true
		}
	}
	return "", serverSpec, false
}

// GetServerFromConfigSource loads a server configuration from a config source.
// Returns the server config or an error if not found.
func GetServerFromConfigSource(configSource, serverName string) (*ServerConfig, error) {
	// Load the configs file to get the alias path
	configs, err := loadConfigsFile()
	if err != nil {
		return nil, fmt.Errorf("failed to load configs: %w", err)
	}

	// Get the config alias
	aliasConfig, ok := configs.Aliases[configSource]
	if !ok {
		return nil, fmt.Errorf("config source '%s' not found", configSource)
	}

	// Expand and read the config file
	configPath := expandPath(aliasConfig.Path)

	// Get servers using existing function based on JSONPath
	var servers []ServerConfig
	var scanErr error

	if strings.Contains(aliasConfig.JSONPath, "mcp.servers") {
		servers, scanErr = scanVSCodeConfig(configPath, aliasConfig.Source)
	} else {
		servers, scanErr = scanMCPServersConfig(configPath, aliasConfig.Source)
	}

	if scanErr != nil {
		return nil, fmt.Errorf("failed to scan config: %w", scanErr)
	}

	// Find the specific server
	var available []string
	for _, server := range servers {
		if server.Name == serverName {
			return &server, nil
		}
		available = append(available, server.Name)
	}

	return nil, fmt.Errorf("server '%s' not found in %s\nAvailable servers: %s",
		serverName, configSource, strings.Join(available, ", "))
}

// buildAuthHeader builds an Authorization header from the available auth options.
// It returns the header value and a cleaned URL (with embedded credentials removed).
func buildAuthHeader(originalURL string) (string, string, error) {
	cleanURL := originalURL

	// First, check if we have explicit auth-user flag with username:password format
	if AuthUser != "" {
		// Parse username:password format
		if !strings.Contains(AuthUser, ":") {
			return "", originalURL, fmt.Errorf("auth-user must be in username:password format (missing colon)")
		}

		parts := strings.SplitN(AuthUser, ":", 2)
		username := parts[0]
		password := parts[1]

		// Allow empty username or password, but not both
		if username != "" || password != "" {
			// Create basic auth header
			auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
			header := "Basic " + auth
			return header, cleanURL, nil
		}
		// Both empty, treat as no auth - fall through
	}

	// Check for custom auth header
	if AuthHeader != "" {
		return AuthHeader, cleanURL, nil
	}

	// Extract credentials from URL if embedded
	parsedURL, err := url.Parse(originalURL)
	if err != nil {
		return "", originalURL, err
	}

	if parsedURL.User != nil {
		username := parsedURL.User.Username()
		password, _ := parsedURL.User.Password()

		if username != "" {
			// Create basic auth header
			auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))

			// Clean the URL by removing user info
			parsedURL.User = nil
			cleanURL = parsedURL.String()

			return "Basic " + auth, cleanURL, nil
		}
	}

	return "", cleanURL, nil
}

// CreateClientFunc is the function used to create MCP clients.
// This can be replaced in tests to use a mock transport.
var CreateClientFunc = func(args []string, _ ...client.ClientOption) (*client.Client, error) {
	if len(args) == 0 {
		return nil, ErrCommandRequired
	}

	// Check for config source prefix (e.g., "claude-code:firecrawl")
	if len(args) == 1 {
		configSource, serverName, hasPrefix := ParseConfigPrefix(args[0])
		if hasPrefix {
			serverConfig, err := GetServerFromConfigSource(configSource, serverName)
			if err != nil {
				return nil, err
			}

			// Build args from server config
			if serverConfig.URL != "" {
				// URL-based server (SSE/HTTP)
				args = []string{serverConfig.URL}
				// TODO: Handle headers for SSE servers
			} else {
				// Stdio-based server
				args = append([]string{serverConfig.Command}, serverConfig.Args...)
			}

			// Apply environment variables
			for key, value := range serverConfig.Env {
				os.Setenv(key, value)
			}
		}
	}

	// Check if the first argument is an alias (fallback if no prefix match)
	if len(args) == 1 {
		server, found := alias.GetServerCommand(args[0])
		if found {
			args = ParseCommandString(server)
		}
	}

	var c *client.Client
	var err error

	if len(args) == 1 && IsHTTP(args[0]) {
		// Validate transport option for HTTP URLs
		if TransportOption != TransportHTTP && TransportOption != TransportSSE {
			return nil, fmt.Errorf("invalid transport option: %s (supported: http, sse)", TransportOption)
		}

		// Build authentication header
		authHeader, cleanURL, authErr := buildAuthHeader(args[0])
		if authErr != nil {
			return nil, fmt.Errorf("failed to parse authentication: %w", authErr)
		}

		// Create headers map with required Accept header for MCP protocol
		headers := make(map[string]string)

		// Add authentication header if provided
		if authHeader != "" {
			headers["Authorization"] = authHeader
		}

		// Add Accept header required by MCP streamable HTTP and SSE transports
		// Many MCP servers require clients to accept both JSON responses and event streams
		headers["Accept"] = "application/json, text/event-stream"

		if TransportOption == TransportSSE {
			// For SSE transport, use transport.ClientOption
			c, err = client.NewSSEMCPClient(cleanURL, transport.WithHeaders(headers))
		} else {
			// For StreamableHTTP transport, use transport.StreamableHTTPCOption
			c, err = client.NewStreamableHttpClient(cleanURL, transport.WithHTTPHeaders(headers))
		}

		if err != nil {
			return nil, err
		}
		err = c.Start(context.Background())
	} else {
		c, err = client.NewStdioMCPClient(args[0], nil, args[1:]...)
	}

	if err != nil {
		return nil, err
	}

	stdErr, ok := client.GetStderr(c)
	if ok && ShowServerLogs {
		go func() {
			scanner := bufio.NewScanner(stdErr)
			for scanner.Scan() {
				fmt.Printf("[>] %s\n", scanner.Text())
			}
		}()
	}

	done := make(chan error, 1)

	go func() {
		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = "2024-11-05"
		initRequest.Params.Capabilities = mcp.ClientCapabilities{}
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    "mcptools",
			Version: "1.0.0",
		}
		_, err := c.Initialize(context.Background(), initRequest)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("init error: %w", err)
		}
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("initialization timed out")
	}

	return c, nil
}

// ProcessFlags processes command line flags, sets the format option, and returns the remaining
// arguments. Supported format options: json, pretty, and table.
// Supported transport options: http and sse.
//
// For example, if the input arguments are ["tools", "--format", "pretty", "npx", "-y",
// "@modelcontextprotocol/server-filesystem", "~"], it would return ["npx", "-y",
// "@modelcontextprotocol/server-filesystem", "~"] and set the format option to "pretty".
func ProcessFlags(args []string) []string {
	parsedArgs := []string{}

	i := 0
	for i < len(args) {
		switch {
		case (args[i] == FlagFormat || args[i] == FlagFormatShort) && i+1 < len(args):
			FormatOption = args[i+1]
			i += 2
		case args[i] == FlagTransport && i+1 < len(args):
			TransportOption = args[i+1]
			i += 2
		case args[i] == FlagServerLogs:
			ShowServerLogs = true
			i++
		case args[i] == FlagAuthUser && i+1 < len(args):
			AuthUser = args[i+1]
			i += 2
		case args[i] == FlagAuthHeader && i+1 < len(args):
			AuthHeader = args[i+1]
			i += 2
		default:
			parsedArgs = append(parsedArgs, args[i])
			i++
		}
	}

	return parsedArgs
}

// FormatAndPrintResponse formats and prints an MCP response in the format specified by
// FormatOption.
func FormatAndPrintResponse(cmd *cobra.Command, resp any, err error) error {
	if err != nil {
		return fmt.Errorf("error: %w", err)
	}

	output, err := jsonutils.Format(resp, FormatOption)
	if err != nil {
		return fmt.Errorf("error formatting output: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), output)
	return nil
}

// IsValidFormat returns true if the format is valid.
func IsValidFormat(format string) bool {
	return format == "json" || format == "j" ||
		format == "pretty" || format == "p" ||
		format == "table" || format == "t"
}

// ParseCommandString splits a command string into separate arguments,
// respecting spaces as argument separators.
// Note: This is a simple implementation that doesn't handle quotes or escapes.
func ParseCommandString(cmdStr string) []string {
	if cmdStr == "" {
		return nil
	}

	return strings.Fields(cmdStr)
}

// ConvertJSONToSlice converts a JSON serialized object to a slice of any type.
func ConvertJSONToSlice(jsonData any) []any {
	if jsonData == nil {
		return nil
	}
	var toolsSlice []any
	data, _ := json.Marshal(jsonData)
	_ = json.Unmarshal(data, &toolsSlice)
	return toolsSlice
}

// ConvertJSONToMap converts a JSON serialized object to a map of strings to any type.
func ConvertJSONToMap(jsonData any) map[string]any {
	if jsonData == nil {
		return nil
	}
	var promptMap map[string]any
	data, _ := json.Marshal(jsonData)
	_ = json.Unmarshal(data, &promptMap)
	return promptMap
}
