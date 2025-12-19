package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/f/mcptools/pkg/jsonutils"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// ServerConfig represents a configuration for a server.
type ServerConfig struct {
	Headers     map[string]string      `json:"headers,omitempty"`
	Env         map[string]string      `json:"env,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Source      string                 `json:"source"`
	Type        string                 `json:"type,omitempty"`
	Command     string                 `json:"command,omitempty"`
	URL         string                 `json:"url,omitempty"`
	Path        string                 `json:"path,omitempty"`
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Args        []string               `json:"args,omitempty"`
}

// Constants for common strings.
const (
	defaultJSONPath = "$.mcpServers"
	formatJSON      = "json"
	formatPretty    = "pretty"
	formatTable     = "table"

	// File permissions.
	dirPermissions  = 0o750
	filePermissions = 0o600
)

// ConfigFileOption stores the path to the configuration file.
var ConfigFileOption string

// HeadersOption stores the headers for URL-based servers.
var HeadersOption string

// EnvOption stores the environment variables.
var EnvOption string

// URLOption stores the URL for URL-based servers.
var URLOption string

// ConfigAlias represents a configuration alias.
type ConfigAlias struct {
	Path     string `json:"path"`
	JSONPath string `json:"jsonPath"`
	Source   string `json:"source,omitempty"`
}

// ConfigsFile represents the structure of the configs file.
type ConfigsFile struct {
	Aliases map[string]ConfigAlias `json:"aliases"`
}

// getConfigsFilePath returns the path to the configs file.
func getConfigsFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".mcpt")
	if err := os.MkdirAll(configDir, dirPermissions); err != nil { //nolint:gosec // We want the directory to be readable
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	return filepath.Join(configDir, "configs.json"), nil
}

// loadConfigsFile loads the configs file, creating it if it doesn't exist.
func loadConfigsFile() (*ConfigsFile, error) {
	configsPath, err := getConfigsFilePath()
	if err != nil {
		return nil, err
	}

	// Create default config if file doesn't exist
	fileInfo, statErr := os.Stat(configsPath) //nolint:govet // Intentional shadow
	if os.IsNotExist(statErr) {
		defaultConfig := &ConfigsFile{
			Aliases: map[string]ConfigAlias{
				"vscode": {
					Path:     "~/Library/Application Support/Code/User/settings.json",
					JSONPath: "$.mcp.servers",
					Source:   "VS Code",
				},
				"vscode-insiders": {
					Path:     "~/Library/Application Support/Code - Insiders/User/settings.json",
					JSONPath: "$.mcp.servers",
					Source:   "VS Code Insiders",
				},
				"windsurf": {
					Path:     "~/.codeium/windsurf/mcp_config.json",
					JSONPath: defaultJSONPath,
					Source:   "Windsurf",
				},
				"cursor": {
					Path:     "~/.cursor/mcp.json",
					JSONPath: defaultJSONPath,
					Source:   "Cursor",
				},
				"claude-desktop": {
					Path:     "~/Library/Application Support/Claude/claude_desktop_config.json",
					JSONPath: defaultJSONPath,
					Source:   "Claude Desktop",
				},
				"claude-code": {
					Path:     "~/.claude.json",
					JSONPath: defaultJSONPath,
					Source:   "Claude Code",
				},
			},
		}

		configData, marshalErr := json.MarshalIndent(defaultConfig, "", "  ")
		if marshalErr != nil {
			return nil, fmt.Errorf("failed to marshal default config: %w", marshalErr)
		}

		if writeErr := os.WriteFile(configsPath, configData, filePermissions); writeErr != nil { //nolint:gosec // User config file
			return nil, fmt.Errorf("failed to write default config: %w", writeErr)
		}

		return defaultConfig, nil
	}

	// Handle other errors from Stat
	if statErr != nil {
		return nil, fmt.Errorf("failed to check config file: %w", statErr)
	}

	// Ensure it's a regular file
	if !fileInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("config path exists but is not a regular file")
	}

	// Read existing config
	data, err := os.ReadFile(configsPath) //nolint:gosec // File path from user home directory
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config ConfigsFile
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Initialize map if nil
	if config.Aliases == nil {
		config.Aliases = make(map[string]ConfigAlias)
	}

	return &config, nil
}

// saveConfigsFile saves the configs file.
func saveConfigsFile(config *ConfigsFile) error {
	configsPath, err := getConfigsFilePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configsPath, data, filePermissions) //nolint:gosec // User config file
}

// expandPath expands the ~ in the path.
func expandPath(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	return filepath.Join(homeDir, path[1:])
}

// parseKeyValueOption parses a comma-separated list of key=value pairs.
func parseKeyValueOption(option string) (map[string]string, error) {
	result := make(map[string]string)
	if option == "" {
		return result, nil
	}

	pairs := strings.Split(option, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid key-value pair: %s", pair)
		}
		result[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}

	return result, nil
}

// getConfigFileAndPath gets the config file path and json path from an alias or direct file path.
func getConfigFileAndPath(configs *ConfigsFile, aliasName, configFile string) (string, string, error) {
	var jsonPath string
	if configFile == "" {
		// Check if the name is an alias
		aliasConfig, ok := configs.Aliases[strings.ToLower(aliasName)]
		if !ok {
			return "", "", fmt.Errorf("alias '%s' not found and no config file specified", aliasName)
		}
		configFile = aliasConfig.Path
		jsonPath = aliasConfig.JSONPath
	} else {
		// Default JSON path if using direct file path
		jsonPath = defaultJSONPath
	}

	// Expand the path if needed
	configFile = expandPath(configFile)
	return configFile, jsonPath, nil
}

// readConfigFile reads and parses a config file.
func readConfigFile(configFile string) (map[string]interface{}, error) {
	var configData map[string]interface{}
	if _, err := os.Stat(configFile); err == nil {
		// File exists, read and parse it
		data, err := os.ReadFile(configFile) //nolint:gosec // File path is validated earlier
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := json.Unmarshal(data, &configData); err != nil {
			// Create new empty config if file exists but isn't valid JSON
			configData = make(map[string]interface{})
		}
	} else {
		// Create new empty config if file doesn't exist
		configData = make(map[string]interface{})
	}
	return configData, nil
}

// addServerToConfig adds a server configuration to the config data.
func addServerToConfig(configData map[string]interface{}, jsonPath, serverName string, serverConfig map[string]interface{}) {
	if strings.Contains(jsonPath, "mcp.servers") {
		// VS Code format
		if _, ok := configData["mcp"]; !ok {
			configData["mcp"] = map[string]interface{}{}
		}

		mcpMap, ok := configData["mcp"].(map[string]interface{})
		if !ok {
			mcpMap = map[string]interface{}{}
			configData["mcp"] = mcpMap
		}

		if _, exists := mcpMap["servers"]; !exists { //nolint:govet // Intentional shadow
			mcpMap["servers"] = map[string]interface{}{}
		}

		serversMap, exists := mcpMap["servers"].(map[string]interface{}) //nolint:govet // Intentional shadow
		if !exists {
			serversMap = map[string]interface{}{}
			mcpMap["servers"] = serversMap
		}

		serversMap[serverName] = serverConfig
		return
	}

	// Other formats with mcpServers
	if _, ok := configData["mcpServers"]; !ok {
		configData["mcpServers"] = map[string]interface{}{}
	}

	serversMap, ok := configData["mcpServers"].(map[string]interface{})
	if !ok {
		serversMap = map[string]interface{}{}
		configData["mcpServers"] = serversMap
	}

	serversMap[serverName] = serverConfig
}

// getServerFromConfig gets a server configuration from the config data.
func getServerFromConfig(configData map[string]interface{}, jsonPath, serverName string) (map[string]interface{}, bool) {
	if strings.Contains(jsonPath, "mcp.servers") {
		// VS Code format
		mcpMap, ok := configData["mcp"].(map[string]interface{})
		if !ok {
			return nil, false
		}

		serversMap, ok := mcpMap["servers"].(map[string]interface{})
		if !ok {
			return nil, false
		}

		server, ok := serversMap[serverName].(map[string]interface{})
		return server, ok
	}

	// Other formats with mcpServers
	serversMap, ok := configData["mcpServers"].(map[string]interface{})
	if !ok {
		return nil, false
	}

	server, ok := serversMap[serverName].(map[string]interface{})
	return server, ok
}

// removeServerFromConfig removes a server configuration from the config data.
func removeServerFromConfig(configData map[string]interface{}, jsonPath, serverName string) bool {
	if strings.Contains(jsonPath, "mcp.servers") {
		// VS Code format
		mcpMap, ok := configData["mcp"].(map[string]interface{})
		if !ok {
			return false
		}

		serversMap, ok := mcpMap["servers"].(map[string]interface{})
		if !ok {
			return false
		}

		if _, exists := serversMap[serverName]; !exists {
			return false
		}

		delete(serversMap, serverName)
		return true
	}

	// Other formats with mcpServers
	serversMap, ok := configData["mcpServers"].(map[string]interface{})
	if !ok {
		return false
	}

	if _, exists := serversMap[serverName]; !exists {
		return false
	}

	delete(serversMap, serverName)
	return true
}

// getServersFromConfig extracts all servers from a config file.
func getServersFromConfig(configFile string, jsonPath string, _ string) (map[string]map[string]interface{}, error) {
	// Read the config file
	data, err := os.ReadFile(configFile) //nolint:gosec // File path is validated earlier
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var configData map[string]interface{}
	if err := json.Unmarshal(data, &configData); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Extract servers based on JSONPath
	var serversMap map[string]interface{}
	if strings.Contains(jsonPath, "mcp.servers") {
		// VS Code format
		mcpMap, ok := configData["mcp"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("no mcp key found in config")
		}
		serversMap, ok = mcpMap["servers"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("no mcp.servers key found in config")
		}
	} else {
		// Other formats with mcpServers
		var ok bool
		serversMap, ok = configData["mcpServers"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("no mcpServers key found in config")
		}
	}

	// Convert to map of maps for easier handling
	result := make(map[string]map[string]interface{})
	for name, server := range serversMap {
		if serverConfig, ok := server.(map[string]interface{}); ok {
			result[name] = serverConfig
		}
	}

	return result, nil
}

// formatJSONForComparison formats a server config as indented JSON for display.
func formatJSONForComparison(config map[string]interface{}) string {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error formatting JSON: %v", err)
	}
	return string(data)
}

// areConfigsIdentical checks if two server configurations are identical.
func areConfigsIdentical(config1, config2 map[string]interface{}) bool {
	// Marshal both configs to JSON for deep comparison
	json1, err1 := json.Marshal(config1)
	json2, err2 := json.Marshal(config2)

	// If we can't marshal either one, consider them different
	if err1 != nil || err2 != nil {
		return false
	}

	// Compare the JSON representations
	return bytes.Equal(json1, json2)
}

// formatSourceGroupedJSON formats servers grouped by source with raw JSON.
func formatSourceGroupedJSON(servers []ServerConfig) string {
	if len(servers) == 0 {
		return "No MCP servers found"
	}

	// Group servers by source
	serversBySource := make(map[string][]ServerConfig)
	var sourceOrder []string

	for _, server := range servers {
		if _, exists := serversBySource[server.Source]; !exists {
			sourceOrder = append(sourceOrder, server.Source)
		}
		serversBySource[server.Source] = append(serversBySource[server.Source], server)
	}

	// Sort sources
	sort.Strings(sourceOrder)

	var buf bytes.Buffer
	for _, source := range sourceOrder {
		// Print source header
		fmt.Fprintf(&buf, "%s:\n\n", source)

		// Reconstruct the original JSON structure
		sourceServers := serversBySource[source]

		if strings.Contains(source, "VS Code") {
			// VS Code format with mcp.servers
			vsCodeJSON := map[string]interface{}{
				"mcp": map[string]interface{}{
					"servers": make(map[string]interface{}),
				},
			}

			serversMap := vsCodeJSON["mcp"].(map[string]interface{})["servers"].(map[string]interface{})
			for _, server := range sourceServers {
				serversMap[server.Name] = server.Config
			}

			jsonData, _ := json.MarshalIndent(vsCodeJSON, "", "  ")
			fmt.Fprintf(&buf, "%s\n\n", string(jsonData))
		} else {
			// Other formats with mcpServers
			otherJSON := map[string]interface{}{
				"mcpServers": make(map[string]interface{}),
			}

			serversMap := otherJSON["mcpServers"].(map[string]interface{})
			for _, server := range sourceServers {
				serversMap[server.Name] = server.Config
			}

			jsonData, _ := json.MarshalIndent(otherJSON, "", "  ")
			fmt.Fprintf(&buf, "%s\n\n", string(jsonData))
		}
	}

	return buf.String()
}

// formatColoredGroupedServers formats servers in a colored, grouped display by source.
func formatColoredGroupedServers(servers []ServerConfig) string {
	if len(servers) == 0 {
		return "No MCP servers found"
	}

	// Group servers by source
	serversBySource := make(map[string][]ServerConfig)
	var sourceOrder []string

	for _, server := range servers {
		if _, exists := serversBySource[server.Source]; !exists {
			sourceOrder = append(sourceOrder, server.Source)
		}
		serversBySource[server.Source] = append(serversBySource[server.Source], server)
	}

	// Sort sources
	sort.Strings(sourceOrder)

	var buf bytes.Buffer
	// Check if we're outputting to a terminal (for colors)
	useColors := jsonutils.IsTerminal()

	for _, source := range sourceOrder {
		// Print source header with bold blue
		if useColors {
			fmt.Fprintf(&buf, "\x1b[1m\x1b[34m%s\x1b[0m\n", source)
		} else {
			fmt.Fprintf(&buf, "%s\n", source)
		}

		servers := serversBySource[source]
		// Sort servers by name
		sort.Slice(servers, func(i, j int) bool {
			return servers[i].Name < servers[j].Name
		})

		for _, server := range servers {
			// Determine server type
			serverType := server.Type
			if serverType == "" {
				if server.URL != "" {
					serverType = "sse" // nolint:goconst
				} else {
					serverType = "stdio" // nolint:goconst
				}
			}

			// Print server name and type
			if useColors {
				fmt.Fprintf(&buf, "  \x1b[1m\x1b[35m%s\x1b[0m", server.Name)
				fmt.Fprintf(&buf, " \x1b[1m\x1b[36m(%s)\x1b[0m:", serverType)
			} else {
				fmt.Fprintf(&buf, "  %s (%s):", server.Name, serverType)
			}

			// Print command or URL
			if serverType == "sse" {
				if useColors {
					fmt.Fprintf(&buf, "\n    \x1b[32m%s\x1b[0m\n", server.URL)
				} else {
					fmt.Fprintf(&buf, "\n    %s\n", server.URL)
				}

				// Print headers for SSE servers
				if len(server.Headers) > 0 {
					// Get sorted header keys
					var headerKeys []string
					for k := range server.Headers {
						headerKeys = append(headerKeys, k)
					}
					sort.Strings(headerKeys)

					// Print each header
					for _, k := range headerKeys {
						if useColors {
							fmt.Fprintf(&buf, "      \x1b[33m%s\x1b[0m: %s\n", k, server.Headers[k])
						} else {
							fmt.Fprintf(&buf, "      %s: %s\n", k, server.Headers[k])
						}
					}
				}
			} else {
				// Print command and args
				commandStr := server.Command

				// Add args with quotes for ones containing spaces
				if len(server.Args) > 0 {
					commandStr += " "
					quotedArgs := make([]string, len(server.Args))
					for i, arg := range server.Args {
						if strings.Contains(arg, " ") && !strings.HasPrefix(arg, "\"") && !strings.HasSuffix(arg, "\"") {
							quotedArgs[i] = "\"" + arg + "\""
						} else {
							quotedArgs[i] = arg
						}
					}
					commandStr += strings.Join(quotedArgs, " ")
				}

				if useColors {
					fmt.Fprintf(&buf, "\n    \x1b[32m%s\x1b[0m\n", commandStr)
				} else {
					fmt.Fprintf(&buf, "\n    %s\n", commandStr)
				}

				// Print env vars
				if len(server.Env) > 0 {
					// Get sorted env keys
					var envKeys []string
					for k := range server.Env {
						envKeys = append(envKeys, k)
					}
					sort.Strings(envKeys)

					// Print each env var
					for _, k := range envKeys {
						if useColors {
							fmt.Fprintf(&buf, "      \x1b[33m%s\x1b[0m: %s\n", k, server.Env[k])
						} else {
							fmt.Fprintf(&buf, "      %s: %s\n", k, server.Env[k])
						}
					}
				}
			}

			// Add a newline after each server for readability
			fmt.Fprintln(&buf)
		}
	}

	return buf.String()
}

// scanForServers scans various configuration files for MCP servers.
func scanForServers() ([]ServerConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	var servers []ServerConfig

	// Scan VS Code Insiders
	vscodeInsidersPath := filepath.Join(homeDir, "Library", "Application Support", "Code - Insiders", "User", "settings.json")
	vscodeServers, err := scanVSCodeConfig(vscodeInsidersPath, "VS Code Insiders")
	if err == nil {
		servers = append(servers, vscodeServers...)
	}

	// Scan VS Code
	vscodePath := filepath.Join(homeDir, "Library", "Application Support", "Code", "User", "settings.json")
	vscodeServers, err = scanVSCodeConfig(vscodePath, "VS Code")
	if err == nil {
		servers = append(servers, vscodeServers...)
	}

	// Scan Windsurf
	windsurfPath := filepath.Join(homeDir, ".codeium", "windsurf", "mcp_config.json")
	windsurfServers, err := scanMCPServersConfig(windsurfPath, "Windsurf")
	if err == nil {
		servers = append(servers, windsurfServers...)
	}

	// Scan Cursor
	cursorPath := filepath.Join(homeDir, ".cursor", "mcp.json")
	cursorServers, err := scanMCPServersConfig(cursorPath, "Cursor")
	if err == nil {
		servers = append(servers, cursorServers...)
	}

	// Scan Claude Desktop
	claudeDesktopPath := filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	claudeServers, err := scanMCPServersConfig(claudeDesktopPath, "Claude Desktop")
	if err == nil {
		servers = append(servers, claudeServers...)
	}

	// Scan Claude Code
	claudeCodePath := filepath.Join(homeDir, ".claude.json")
	claudeCodeServers, err := scanMCPServersConfig(claudeCodePath, "Claude Code")
	if err == nil {
		servers = append(servers, claudeCodeServers...)
	}

	return servers, nil
}

// scanVSCodeConfig scans a VS Code settings.json file for MCP servers.
func scanVSCodeConfig(path, source string) ([]ServerConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // File path from user home directory
	if err != nil {
		return nil, fmt.Errorf("failed to read %s config: %w", source, err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("failed to parse %s settings.json: %w", source, err)
	}

	mcpObject, ok := settings["mcp"]
	if !ok {
		return nil, fmt.Errorf("no mcp configuration found in %s settings", source)
	}

	mcpMap, ok := mcpObject.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid mcp format in %s settings", source)
	}

	mcpServers, ok := mcpMap["servers"]
	if !ok {
		return nil, fmt.Errorf("no mcp.servers found in %s settings", source)
	}

	servers, ok := mcpServers.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid mcp.servers format in %s settings", source)
	}

	var result []ServerConfig
	for name, config := range servers {
		serverConfig, ok := config.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract common properties
		serverType, _ := serverConfig["type"].(string)
		command, _ := serverConfig["command"].(string)
		description, _ := serverConfig["description"].(string)
		url, _ := serverConfig["url"].(string)

		// Extract args if available
		var args []string
		if argsInterface, ok := serverConfig["args"].([]interface{}); ok {
			for _, arg := range argsInterface {
				if argStr, ok := arg.(string); ok {
					args = append(args, argStr)
				}
			}
		}

		// Extract headers if available
		headers := make(map[string]string)
		if headersInterface, ok := serverConfig["headers"].(map[string]interface{}); ok {
			for k, v := range headersInterface {
				if valStr, ok := v.(string); ok {
					headers[k] = valStr
				}
			}
		}

		// Extract env if available
		env := make(map[string]string)
		if envInterface, ok := serverConfig["env"].(map[string]interface{}); ok {
			for k, v := range envInterface {
				if valStr, ok := v.(string); ok {
					env[k] = valStr
				}
			}
		}

		result = append(result, ServerConfig{
			Source:      source,
			Type:        serverType,
			Command:     command,
			Args:        args,
			URL:         url,
			Headers:     headers,
			Env:         env,
			Name:        name,
			Config:      serverConfig,
			Description: description,
		})
	}

	return result, nil
}

// scanMCPServersConfig scans a config file with mcpServers as the top-level key.
func scanMCPServersConfig(path, source string) ([]ServerConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // File path from user home directory
	if err != nil {
		return nil, fmt.Errorf("failed to read %s config: %w", source, err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse %s config: %w", source, err)
	}

	mcpServers, ok := config["mcpServers"]
	if !ok {
		return nil, fmt.Errorf("no mcpServers key found in %s config", source)
	}

	servers, ok := mcpServers.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid mcpServers format in %s config", source)
	}

	var result []ServerConfig
	for name, serverData := range servers {
		serverConfig, ok := serverData.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract common properties
		command, _ := serverConfig["command"].(string)
		url, _ := serverConfig["url"].(string)

		// Extract args if available
		var args []string
		if argsInterface, ok := serverConfig["args"].([]interface{}); ok {
			for _, arg := range argsInterface {
				if argStr, ok := arg.(string); ok {
					args = append(args, argStr)
				}
			}
		}

		// Extract headers if available
		headers := make(map[string]string)
		if headersInterface, ok := serverConfig["headers"].(map[string]interface{}); ok {
			for k, v := range headersInterface {
				if valStr, ok := v.(string); ok {
					headers[k] = valStr
				}
			}
		}

		// Extract env if available
		env := make(map[string]string)
		if envInterface, ok := serverConfig["env"].(map[string]interface{}); ok {
			for k, v := range envInterface {
				if valStr, ok := v.(string); ok {
					env[k] = valStr
				}
			}
		}

		// Determine type based on whether URL is present
		serverType := ""
		if url != "" {
			serverType = "sse"
		}

		result = append(result, ServerConfig{
			Source:  source,
			Type:    serverType,
			Command: command,
			Args:    args,
			URL:     url,
			Headers: headers,
			Env:     env,
			Name:    name,
			Config:  serverConfig,
		})
	}

	return result, nil
}

// ConfigsCmd creates the configs command.
func ConfigsCmd() *cobra.Command { //nolint:gocyclo // This is a large command with many subcommands
	cmd := &cobra.Command{
		Use:   "configs",
		Short: "Manage MCP server configurations",
		Long:  `Manage MCP server configurations including scanning, adding, and aliasing.`,
	}

	// Add scan subcommand
	scanCmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan for available MCP servers in various configurations",
		Long: `Scan for available MCP servers in various configuration files of these Applications on macOS:
VS Code, VS Code Insiders, Windsurf, Cursor, Claude Desktop, Claude Code`,
		Run: func(cmd *cobra.Command, _ []string) {
			servers, err := scanForServers()
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error scanning for servers: %v\n", err)
				return
			}

			// Table format (default) now uses the colored grouped display
			if strings.ToLower(FormatOption) == "table" || strings.ToLower(FormatOption) == "pretty" {
				output := formatColoredGroupedServers(servers)
				fmt.Fprintln(cmd.OutOrStdout(), output)
				return
			}

			// For JSON format, use the grouped display
			if strings.ToLower(FormatOption) == "json" {
				output := formatSourceGroupedJSON(servers)
				fmt.Fprintln(cmd.OutOrStdout(), output)
				return
			}

			// For other formats, use the full server data
			output, err := json.MarshalIndent(servers, "", "  ")
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error formatting output: %v\n", err)
				return
			}

			fmt.Fprintln(cmd.OutOrStdout(), string(output))
		},
	}

	// Add the view subcommand with AllOption flag
	var AllOption bool
	viewCmd := &cobra.Command{
		Use:   "view [alias or path]",
		Short: "View MCP servers in configurations",
		Long:  `View MCP servers in a specific configuration file or all configured aliases with --all flag.`,
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// Load configs
			configs, err := loadConfigsFile()
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error loading configs: %v\n", err)
				return
			}

			var servers []ServerConfig

			// All mode - scan all aliases (same as previous scan command)
			if AllOption {
				if len(args) > 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: ignoring specified alias/path when using --all flag\n")
				}

				// Scan all configured aliases
				for alias, config := range configs.Aliases {
					// Skip if path is empty
					if config.Path == "" {
						continue
					}

					// Determine the scanner function based on the JSONPath
					expandedPath := expandPath(config.Path)
					var configServers []ServerConfig
					var scanErr error

					source := config.Source
					if source == "" {
						titleCase := cases.Title(language.English)
						source = titleCase.String(alias) // Use capitalized alias name if source not provided
					}

					if strings.Contains(config.JSONPath, "mcp.servers") {
						configServers, scanErr = scanVSCodeConfig(expandedPath, source)
					} else if strings.Contains(config.JSONPath, "mcpServers") {
						configServers, scanErr = scanMCPServersConfig(expandedPath, source)
					}

					if scanErr == nil {
						servers = append(servers, configServers...)
					}
				}
			} else {
				// Single config mode - require an argument
				if len(args) == 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error: must specify an alias or path, use --all flag, or use `configs ls` command\n")
					return
				}

				target := args[0]
				var configPath string
				var source string
				var jsonPath string

				// Check if the argument is an alias
				if aliasConfig, ok := configs.Aliases[strings.ToLower(target)]; ok {
					configPath = aliasConfig.Path
					source = aliasConfig.Source
					if source == "" {
						titleCase := cases.Title(language.English)
						source = titleCase.String(target)
					}
					jsonPath = aliasConfig.JSONPath
				} else {
					// Assume it's a direct path
					configPath = target
					source = filepath.Base(target)
					jsonPath = "$.mcpServers" // Default JSON path
				}

				// Expand the path if needed
				expandedPath := expandPath(configPath)

				// Scan the config file
				var configServers []ServerConfig
				var scanErr error

				if strings.Contains(jsonPath, "mcp.servers") {
					configServers, scanErr = scanVSCodeConfig(expandedPath, source)
				} else {
					configServers, scanErr = scanMCPServersConfig(expandedPath, source)
				}

				if scanErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error scanning configuration: %v\n", scanErr)
					return
				}

				servers = configServers
			}

			// Handle empty results
			if len(servers) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No MCP servers found")
				return
			}

			// Output based on format
			if strings.ToLower(FormatOption) == formatTable || strings.ToLower(FormatOption) == formatPretty {
				output := formatColoredGroupedServers(servers)
				fmt.Fprintln(cmd.OutOrStdout(), output)
				return
			}

			if strings.ToLower(FormatOption) == formatJSON {
				output := formatSourceGroupedJSON(servers)
				fmt.Fprintln(cmd.OutOrStdout(), output)
				return
			}

			// For other formats, use the full server data
			output, err := json.MarshalIndent(servers, "", "  ")
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error formatting output: %v\n", err)
				return
			}

			fmt.Fprintln(cmd.OutOrStdout(), string(output))
		},
	}

	// Add --all flag to view command
	viewCmd.Flags().BoolVar(&AllOption, "all", false, "View all configured aliases")

	// Add ls command as an alias for view --all
	lsCmd := &cobra.Command{
		Use:   "ls [alias or path]",
		Short: "List all MCP servers in configurations (alias for view --all)",
		Long:  `List all MCP servers in configurations. If a path is specified, only show that configuration.`,
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// Set AllOption to true to make this default to --all
			AllOption = true

			// Run the view command logic
			viewCmd.Run(cmd, args)
		},
	}

	// Add --all flag to ls command (though it's true by default)
	lsCmd.Flags().BoolVar(&AllOption, "all", false, "View all configured aliases (default: false)")

	// Create the set subcommand (merges add and update functionality)
	setCmd := &cobra.Command{
		Use:   "set [alias,alias2,...] [server] [command/url] [args...]",
		Short: "Add or update an MCP server configuration",
		Long:  `Add or update an MCP server configuration. Creates a new server if it doesn't exist, or updates an existing one. Multiple aliases can be specified with commas.`,
		Args:  cobra.MinimumNArgs(2),
		// Disable flag parsing after the first arguments
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			// We need to manually extract the flags we care about
			var configFile string
			var headers string
			var env string

			// Create cleaned arguments (without our flags)
			cleanedArgs := make([]string, 0, len(args))

			// Process all args to extract our flags and build clean args
			i := 0
			for i < len(args) {
				arg := args[i]

				// Handle both --flag=value and --flag value formats
				if strings.HasPrefix(arg, "--config=") {
					configFile = strings.TrimPrefix(arg, "--config=")
					i++
					continue
				} else if arg == "--config" && i+1 < len(args) {
					configFile = args[i+1]
					i += 2
					continue
				}

				if strings.HasPrefix(arg, "--headers=") {
					headers = strings.TrimPrefix(arg, "--headers=")
					i++
					continue
				} else if arg == "--headers" && i+1 < len(args) {
					headers = args[i+1]
					i += 2
					continue
				}

				if strings.HasPrefix(arg, "--env=") {
					env = strings.TrimPrefix(arg, "--env=")
					i++
					continue
				} else if arg == "--env" && i+1 < len(args) {
					env = args[i+1]
					i += 2
					continue
				}

				// If none of our flags, add to cleaned args
				cleanedArgs = append(cleanedArgs, arg)
				i++
			}

			// Make sure we have enough arguments
			if len(cleanedArgs) < 2 {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: set command requires at least alias and server name arguments\n")
				return
			}

			// Set the values we normally would have through flags
			ConfigFileOption = configFile
			HeadersOption = headers
			EnvOption = env

			// Load configs
			configs, err := loadConfigsFile()
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error loading configs: %v\n", err)
				return
			}

			// Get the alias/config file and server name - allow for comma-separated aliases
			aliasInput := cleanedArgs[0]
			serverName := cleanedArgs[1]

			// Split aliases by comma
			aliasList := strings.Split(aliasInput, ",")
			successCount := 0

			// Process each alias
			for _, aliasName := range aliasList {
				aliasName = strings.TrimSpace(aliasName)
				if aliasName == "" {
					continue
				}

				// Get config file and JSON path from alias or direct path
				configFile, jsonPath, err := getConfigFileAndPath(configs, aliasName, ConfigFileOption)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error for alias '%s': %v\n", aliasName, err)
					continue
				}

				// Read the target config file
				configData, err := readConfigFile(configFile)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error for alias '%s': %v\n", aliasName, err)
					continue
				}

				// Check if the server already exists
				existingServer, exists := getServerFromConfig(configData, jsonPath, serverName)

				// Set up the server config - either new or existing
				var serverConfig map[string]interface{}
				if exists {
					// Update existing server
					serverConfig = existingServer
					if serverConfig == nil {
						serverConfig = make(map[string]interface{})
					}
				} else {
					// Create new server
					serverConfig = make(map[string]interface{})
				}

				// Determine command type - check if command is a URL
				if len(cleanedArgs) > 2 {
					command := cleanedArgs[2]
					if strings.HasPrefix(command, "http://") || strings.HasPrefix(command, "https://") {
						// URL-based server
						serverConfig["url"] = command
						// Remove command-related fields if they exist
						delete(serverConfig, "command")
						delete(serverConfig, "args")

						// Parse headers
						if HeadersOption != "" {
							headers, parseErr := parseKeyValueOption(HeadersOption) //nolint:govet,shadow // reusing variable name for clarity
							if parseErr != nil {
								fmt.Fprintf(cmd.ErrOrStderr(), "Error parsing headers for alias '%s': %v\n", aliasName, parseErr)
								continue
							}
							if len(headers) > 0 {
								serverConfig["headers"] = headers
							}
						}
					} else {
						// Command-based server
						serverConfig["command"] = command
						// Remove URL-related fields if they exist
						delete(serverConfig, "url")
						delete(serverConfig, "headers")

						// Add command args if provided
						if len(cleanedArgs) > 3 {
							serverConfig["args"] = cleanedArgs[3:]
						} else if exists {
							// Only delete args if explicitly not provided during update
							delete(serverConfig, "args")
						}
					}
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error for alias '%s': command or URL must be provided\n", aliasName)
					continue
				}

				// Parse environment variables
				if EnvOption != "" {
					env, parseErr := parseKeyValueOption(EnvOption) //nolint:govet,shadow // reusing variable name for clarity
					if parseErr != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "Error parsing environment variables for alias '%s': %v\n", aliasName, parseErr)
						continue
					}
					if len(env) > 0 {
						serverConfig["env"] = env
					}
				}

				// Add/update the server in the config
				addServerToConfig(configData, jsonPath, serverName, serverConfig)

				// Write the updated config back to the file
				data, err := json.MarshalIndent(configData, "", "  ")
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error marshaling config for alias '%s': %v\n", aliasName, err)
					continue
				}

				if writeErr := os.WriteFile(configFile, data, filePermissions); writeErr != nil { //nolint:gosec // User config file
					fmt.Fprintf(cmd.ErrOrStderr(), "Error writing config file for alias '%s': %v\n", aliasName, writeErr)
					continue
				}

				successCount++
				if exists {
					fmt.Fprintf(cmd.OutOrStdout(), "Server '%s' updated for alias '%s' in %s\n", serverName, aliasName, configFile)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Server '%s' added for alias '%s' to %s\n", serverName, aliasName, configFile)
				}
			}

			// Report summary if multiple aliases were processed
			if len(aliasList) > 1 {
				fmt.Fprintf(cmd.OutOrStdout(), "\nSummary: Successfully processed %d of %d aliases\n", successCount, len(aliasList))
			}
		},
	}

	// Add flags to the commands - these are just for documentation since we do manual parsing
	setCmd.Flags().StringVar(&ConfigFileOption, "config", "", "Path to the configuration file")
	setCmd.Flags().StringVar(&HeadersOption, "headers", "", "Headers for URL-based servers (comma-separated key=value pairs)")
	setCmd.Flags().StringVar(&EnvOption, "env", "", "Environment variables (comma-separated key=value pairs)")

	// Add the remove subcommand
	removeCmd := &cobra.Command{
		Use:   "remove [alias,alias2,...] [server]",
		Short: "Remove an MCP server configuration",
		Long:  `Remove an MCP server configuration from a config file. Multiple aliases can be specified with commas.`,
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			// Load configs
			configs, err := loadConfigsFile()
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error loading configs: %v\n", err)
				return
			}

			// Get the alias/config file and server name - allow for comma-separated aliases
			aliasInput := args[0]
			serverName := args[1]

			// Split aliases by comma
			aliasList := strings.Split(aliasInput, ",")
			successCount := 0

			// Process each alias
			for _, aliasName := range aliasList {
				aliasName = strings.TrimSpace(aliasName)
				if aliasName == "" {
					continue
				}

				// Get config file and JSON path from alias or direct path
				configFile, jsonPath, err := getConfigFileAndPath(configs, aliasName, ConfigFileOption)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error for alias '%s': %v\n", aliasName, err)
					continue
				}

				// Read the target config file
				configData, err := readConfigFile(configFile)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error for alias '%s': %v\n", aliasName, err)
					continue
				}

				// Remove the server
				removed := removeServerFromConfig(configData, jsonPath, serverName)
				if !removed {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error: server '%s' not found for alias '%s' in %s\n", serverName, aliasName, configFile)
					continue
				}

				// Write the updated config back to the file
				data, err := json.MarshalIndent(configData, "", "  ")
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error marshaling config for alias '%s': %v\n", aliasName, err)
					continue
				}

				if writeErr := os.WriteFile(configFile, data, filePermissions); writeErr != nil { //nolint:gosec // User config file
					fmt.Fprintf(cmd.ErrOrStderr(), "Error writing config file for alias '%s': %v\n", aliasName, writeErr)
					continue
				}

				successCount++
				fmt.Fprintf(cmd.OutOrStdout(), "Server '%s' removed for alias '%s' from %s\n", serverName, aliasName, configFile)
			}

			// Report summary if multiple aliases were processed
			if len(aliasList) > 1 {
				fmt.Fprintf(cmd.OutOrStdout(), "\nSummary: Successfully processed %d of %d aliases\n", successCount, len(aliasList))
			}
		},
	}

	// Add flag to remove command
	removeCmd.Flags().StringVar(&ConfigFileOption, "config", "", "Path to the configuration file")

	// Add the alias subcommand
	aliasCmd := &cobra.Command{
		Use:   "alias [name] [path] [jsonPath]",
		Short: "Create an alias for a config file",
		Long:  `Create an alias for a configuration file with the specified JSONPath (defaults to "$.mcpServers").`,
		Args:  cobra.MinimumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			path := args[1]
			jsonPath := "$.mcpServers" // Default value

			// Use custom jsonPath if provided
			if len(args) > 2 {
				jsonPath = args[2]
			}

			// Load configs
			configs, err := loadConfigsFile()
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error loading configs: %v\n", err)
				return
			}

			// Add or update the alias
			configs.Aliases[strings.ToLower(name)] = ConfigAlias{
				Path:     path,
				JSONPath: jsonPath,
				Source:   name, // Use the provided name as the source
			}

			// Save the updated configs
			if err := saveConfigsFile(configs); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error saving configs: %v\n", err)
				return
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Alias '%s' created for %s with JSONPath '%s'\n", name, path, jsonPath)
		},
	}

	// Add the sync command
	var OutputAliasOption string
	var DefaultChoiceOption string
	syncCmd := &cobra.Command{
		Use:   "sync [alias1] [alias2] [...]",
		Short: "Synchronize and merge MCP server configurations",
		Long:  `Synchronize and merge MCP server configurations from multiple alias sources with interactive conflict resolution.`,
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// Load configs
			configs, err := loadConfigsFile()
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error loading configs: %v\n", err)
				return
			}

			// Determine output alias and ensure it's valid
			outputAlias := OutputAliasOption
			if outputAlias == "" {
				// Default to the first alias if no output alias specified
				outputAlias = args[0]
			}

			// Get the target alias config
			outputAliasLower := strings.ToLower(outputAlias)
			targetAliasConfig, ok := configs.Aliases[outputAliasLower]
			if !ok {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: output alias '%s' not found\n", outputAlias)
				return
			}

			// Collect all valid alias configurations
			aliasConfigs := make(map[string]ConfigAlias)
			aliasNames := make([]string, 0)
			aliasFiles := make([]string, 0)

			// Always include the output alias
			aliasConfigs[outputAliasLower] = targetAliasConfig
			aliasNames = append(aliasNames, outputAlias)
			aliasFiles = append(aliasFiles, expandPath(targetAliasConfig.Path))

			// Process each alias to validate and collect configurations
			for _, aliasName := range args {
				aliasLower := strings.ToLower(aliasName)

				// Skip if this is the output alias (already added)
				if aliasLower == outputAliasLower {
					continue
				}

				aliasConfig, ok := configs.Aliases[aliasLower]
				if !ok {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: alias '%s' not found, skipping\n", aliasName)
					continue
				}

				expandedPath := expandPath(aliasConfig.Path)

				// Skip if file doesn't exist
				if _, err := os.Stat(expandedPath); os.IsNotExist(err) {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: config file for alias '%s' not found at %s, skipping\n", aliasName, expandedPath)
					continue
				}

				// Add to our collection of valid aliases
				aliasConfigs[aliasLower] = aliasConfig
				aliasNames = append(aliasNames, aliasName)
				aliasFiles = append(aliasFiles, expandedPath)
			}

			if len(aliasNames) < 2 {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: at least two valid aliases are required for syncing\n")
				return
			}

			// Determine default choice for conflicts
			defaultChoice := strings.ToLower(DefaultChoiceOption)
			if defaultChoice != "first" && defaultChoice != "second" && defaultChoice != "interactive" {
				defaultChoice = "interactive"
			}

			// Collect all servers
			allServers := make(map[string]map[string]interface{})
			serverSources := make(map[string]string) // track where each server comes from
			conflicts := make(map[string][]map[string]interface{})
			conflictSources := make(map[string][]string)

			// Process each alias to collect servers
			for _, aliasName := range aliasNames {
				aliasLower := strings.ToLower(aliasName)
				aliasConfig := aliasConfigs[aliasLower]
				expandedPath := expandPath(aliasConfig.Path)

				// Get servers from this config
				servers, err := getServersFromConfig(expandedPath, aliasConfig.JSONPath, aliasConfig.Source)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to read servers from alias '%s': %v, skipping\n", aliasName, err)
					continue
				}

				// Check for conflicts
				for name, server := range servers {
					if existing, ok := allServers[name]; ok {
						// Check if the configurations are identical
						if areConfigsIdentical(existing, server) {
							// Configs are identical - no need to mark as conflict
							continue
						}

						// This is a true conflict - store both versions
						if conflicts[name] == nil {
							conflicts[name] = []map[string]interface{}{existing, server}
							conflictSources[name] = []string{serverSources[name], aliasName}
						} else {
							conflicts[name] = append(conflicts[name], server)
							conflictSources[name] = append(conflictSources[name], aliasName)
						}
					} else {
						// No conflict, just add it
						allServers[name] = server
						serverSources[name] = aliasName
					}
				}
			}

			// Resolve conflicts
			if len(conflicts) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Found %d server name conflicts to resolve\n", len(conflicts))

				for name, conflictingConfigs := range conflicts {
					sources := conflictSources[name]

					// Skip interactive resolution if default choice is set
					if defaultChoice == "first" {
						allServers[name] = conflictingConfigs[0]
						fmt.Fprintf(cmd.OutOrStdout(), "Conflict for '%s': automatically selected version from '%s'\n", name, sources[0])
						continue
					} else if defaultChoice == "second" {
						allServers[name] = conflictingConfigs[1]
						fmt.Fprintf(cmd.OutOrStdout(), "Conflict for '%s': automatically selected version from '%s'\n", name, sources[1])
						continue
					}

					// Interactive resolution
					fmt.Fprintf(cmd.OutOrStdout(), "\nConflict found for server '%s'\n", name)

					// Display options
					for i, config := range conflictingConfigs {
						fmt.Fprintf(cmd.OutOrStdout(), "Option %d (from alias '%s'):\n", i+1, sources[i])
						fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n", formatJSONForComparison(config))
					}

					// Ask user which to keep
					var choice int
					for {
						fmt.Fprintf(cmd.OutOrStdout(), "Enter option number to keep (1-%d): ", len(conflictingConfigs))

						var input string
						if _, err := fmt.Scanln(&input); err != nil {
							// Handle scan error (EOF, no input, etc.)
							fmt.Fprintf(cmd.ErrOrStderr(), "Error reading input: %v\n", err)
							input = "1" // Default to first option on error
						}

						if n, err := fmt.Sscanf(input, "%d", &choice); err == nil && n == 1 && choice >= 1 && choice <= len(conflictingConfigs) {
							break
						}

						fmt.Fprintf(cmd.OutOrStdout(), "Invalid choice. Please enter a number between 1 and %d\n", len(conflictingConfigs))
					}

					// Save user's choice
					allServers[name] = conflictingConfigs[choice-1]
					fmt.Fprintf(cmd.OutOrStdout(), "Selected option %d for '%s'\n", choice, name)
				}
			}

			// Now update all configuration files
			fmt.Fprintf(cmd.OutOrStdout(), "\nUpdating %d configuration files with %d merged servers\n", len(aliasNames), len(allServers))

			// Track success/failure
			successful := 0

			// Update each alias configuration
			for i, aliasName := range aliasNames {
				aliasLower := strings.ToLower(aliasName)
				aliasConfig := aliasConfigs[aliasLower]
				configFile := aliasFiles[i]
				jsonPath := aliasConfig.JSONPath

				// Read the existing file to preserve its structure
				var configData map[string]interface{}
				if _, err := os.Stat(configFile); err == nil {
					// File exists, read and parse it
					data, err := os.ReadFile(configFile) //nolint:gosec // File path is validated earlier
					if err == nil {
						if unmarshalErr := json.Unmarshal(data, &configData); unmarshalErr != nil {
							// Handle unmarshaling error
							configData = make(map[string]interface{})
						}
					}
				}

				// If we couldn't read the file or it was empty, create a new structure
				if configData == nil {
					configData = make(map[string]interface{})
				}

				// Structure the output based on the JSONPath
				if strings.Contains(jsonPath, "mcp.servers") {
					// VS Code format
					if _, ok := configData["mcp"]; !ok {
						configData["mcp"] = map[string]interface{}{}
					}

					mcpMap, ok := configData["mcp"].(map[string]interface{})
					if !ok {
						mcpMap = map[string]interface{}{}
						configData["mcp"] = mcpMap
					}

					mcpMap["servers"] = allServers
				} else {
					// Other formats with mcpServers (default)
					configData["mcpServers"] = allServers
				}

				// Write the merged config
				data, err := json.MarshalIndent(configData, "", "  ")
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error marshaling merged config for '%s': %v\n", aliasName, err)
					continue
				}

				if err := os.WriteFile(configFile, data, filePermissions); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error writing merged config to %s: %v\n", configFile, err)
					continue
				}

				successful++
				fmt.Fprintf(cmd.OutOrStdout(), "Updated configuration for alias '%s' at %s\n", aliasName, configFile)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "\nSuccessfully synced %d servers across %d/%d alias configurations\n",
				len(allServers), successful, len(aliasNames))
		},
	}

	// Add flags to the sync command
	syncCmd.Flags().StringVar(&OutputAliasOption, "output", "", "Output alias (defaults to first alias)")
	syncCmd.Flags().StringVar(&DefaultChoiceOption, "default", "interactive", "Default choice for conflicts: 'first', 'second', or 'interactive'")

	// Add subcommands to the configs command
	cmd.AddCommand(lsCmd, viewCmd, setCmd, removeCmd, aliasCmd, syncCmd, scanCmd)

	// Add the as-json subcommand
	asJSONCmd := &cobra.Command{
		Use:   "as-json [command/url] [args...]",
		Short: "Convert a command or URL to MCP server JSON configuration",
		Long:  `Convert a command line or URL to a JSON configuration that can be used for MCP servers.`,
		Args:  cobra.MinimumNArgs(1),
		// Disable flag parsing after the first arguments to handle command args properly
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			// We need to manually extract the flags we care about
			var headers string
			var env string

			// Create cleaned arguments (without our flags)
			cleanedArgs := make([]string, 0, len(args))

			// Process all args to extract our flags and build clean args
			i := 0
			for i < len(args) {
				arg := args[i]

				// Handle both --flag=value and --flag value formats
				if strings.HasPrefix(arg, "--headers=") {
					headers = strings.TrimPrefix(arg, "--headers=")
					i++
					continue
				} else if arg == "--headers" && i+1 < len(args) {
					headers = args[i+1]
					i += 2
					continue
				}

				if strings.HasPrefix(arg, "--env=") {
					env = strings.TrimPrefix(arg, "--env=")
					i++
					continue
				} else if arg == "--env" && i+1 < len(args) {
					env = args[i+1]
					i += 2
					continue
				}

				// If none of our flags, add to cleaned args
				cleanedArgs = append(cleanedArgs, arg)
				i++
			}

			// Make sure we have at least one argument
			if len(cleanedArgs) < 1 {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: as-json command requires at least one argument (command or URL)\n")
				return
			}

			// Determine if first argument is a URL
			firstArg := cleanedArgs[0]
			isURL := strings.HasPrefix(firstArg, "http://") || strings.HasPrefix(firstArg, "https://")

			// Create the server configuration
			serverConfig := make(map[string]interface{})

			if isURL {
				// URL-based server
				serverConfig["url"] = firstArg

				// Parse headers if provided
				if headers != "" {
					headersMap, err := parseKeyValueOption(headers)
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "Error parsing headers: %v\n", err)
						return
					}
					if len(headersMap) > 0 {
						serverConfig["headers"] = headersMap
					}
				}
			} else {
				// Command-based server
				serverConfig["command"] = firstArg

				// Add command args if provided
				if len(cleanedArgs) > 1 {
					serverConfig["args"] = cleanedArgs[1:]
				}
			}

			// Parse environment variables if provided (for both URL and command)
			if env != "" {
				envMap, err := parseKeyValueOption(env)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error parsing environment variables: %v\n", err)
					return
				}
				if len(envMap) > 0 {
					serverConfig["env"] = envMap
				}
			}

			// Output the JSON configuration
			output, err := json.MarshalIndent(serverConfig, "", "  ")
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error generating JSON: %v\n", err)
				return
			}

			fmt.Fprintln(cmd.OutOrStdout(), string(output))
		},
	}

	// Add flags to the as-json command - these are just for documentation since we do manual parsing
	asJSONCmd.Flags().StringVar(&HeadersOption, "headers", "", "Headers for URL-based servers (comma-separated key=value pairs)")
	asJSONCmd.Flags().StringVar(&EnvOption, "env", "", "Environment variables (comma-separated key=value pairs)")

	// Add the as-json command to the main command
	cmd.AddCommand(asJSONCmd)

	return cmd
}
