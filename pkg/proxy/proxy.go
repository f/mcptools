// Package proxy provides functionality for proxying MCP tool requests to shell scripts.
package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/f/mcptools/pkg/jsonutils"
)

// Parameter represents a tool parameter with a name and type.
type Parameter struct {
	Name     string
	Type     string
	Required bool
}

// Tool represents a proxy tool that executes a shell script or command.
type Tool struct {
	// Fields ordered for optimal memory alignment (8-byte aligned fields first)
	Name        string
	Description string
	ScriptPath  string
	Command     string // Inline command to execute
	Parameters  []Parameter
}

// Server handles proxying requests to shell scripts.
type Server struct {
	// Fields ordered for optimal memory alignment (8-byte aligned fields first)
	tools   map[string]Tool
	logFile *os.File
	id      int
}

// NewProxyServer creates a new proxy server.
func NewProxyServer() (*Server, error) {
	// Create log directory
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		// On Windows, try USERPROFILE if HOME is not set
		homeDir = os.Getenv("USERPROFILE")
		if homeDir == "" {
			return nil, fmt.Errorf("HOME environment variable not set and USERPROFILE not found")
		}
	}

	logDir := filepath.Join(homeDir, ".mcpt", "logs")
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		return nil, fmt.Errorf("error creating log directory: %w", err)
	}

	// Open log file
	logPath := filepath.Join(logDir, "proxy.log")
	// Clean the path to avoid any path traversal
	logPath = filepath.Clean(logPath)

	// Verify the path is still under the expected log directory
	if !strings.HasPrefix(logPath, logDir) {
		return nil, fmt.Errorf("invalid log path: outside of log directory")
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("error opening log file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Logging to %s\n", logPath)

	return &Server{
		tools:   make(map[string]Tool),
		id:      0,
		logFile: logFile,
	}, nil
}

// log writes a message to the log file with a timestamp.
func (s *Server) log(message string) {
	timestamp := time.Now().Format(time.RFC3339)
	fmt.Fprintf(s.logFile, "[%s] %s\n", timestamp, message)
}

// logJSON writes a JSON-formatted message to the log file with a timestamp.
func (s *Server) logJSON(label string, v any) {
	jsonBytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		s.log(fmt.Sprintf("Error marshaling %s: %v", label, err))
		return
	}
	s.log(fmt.Sprintf("%s: %s", label, string(jsonBytes)))
}

// Close closes the log file.
func (s *Server) Close() error {
	if s.logFile != nil {
		return s.logFile.Close()
	}
	return nil
}

// AddTool adds a new tool to the proxy server.
func (s *Server) AddTool(name, description, paramStr, scriptPath string, command string) error {
	// Parse parameters
	params, err := parseParameters(paramStr)
	if err != nil {
		return fmt.Errorf("invalid parameters: %w", err)
	}

	// If a command is provided, use it directly
	if command != "" {
		s.tools[name] = Tool{
			Name:        name,
			Description: description,
			Parameters:  params,
			Command:     command,
		}
		return nil
	}

	// Otherwise, validate and use the script path
	absPath, err := filepath.Abs(scriptPath)
	if err != nil {
		return fmt.Errorf("invalid script path: %w", err)
	}

	// Clean the path to avoid any path traversal
	absPath = filepath.Clean(absPath)

	// Check if script exists and is executable
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("script not found: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("not a script: %s is a directory", absPath)
	}

	// Additional security check: verify the file is executable
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("script is not executable: %s", absPath)
	}

	s.tools[name] = Tool{
		Name:        name,
		Description: description,
		Parameters:  params,
		ScriptPath:  absPath,
	}

	return nil
}

// parseParameters parses a comma-separated parameter string in the format "name:type,name:type".
// If a parameter is wrapped in square brackets like [name:type], it's considered optional.
func parseParameters(paramStr string) ([]Parameter, error) {
	if paramStr == "" {
		return []Parameter{}, nil
	}

	params := strings.Split(paramStr, ",")
	parameters := make([]Parameter, 0, len(params))

	for _, param := range params {
		param = strings.TrimSpace(param)
		required := true

		// Check if parameter is optional (wrapped in square brackets)
		if strings.HasPrefix(param, "[") && strings.HasSuffix(param, "]") {
			// Remove the brackets
			param = strings.TrimPrefix(param, "[")
			param = strings.TrimSuffix(param, "]")
			required = false
		}

		parts := strings.Split(strings.TrimSpace(param), ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid parameter format: %s, expected name:type", param)
		}

		name := strings.TrimSpace(parts[0])
		paramType := strings.TrimSpace(parts[1])

		// Validate parameter name
		if name == "" {
			return nil, fmt.Errorf("parameter name cannot be empty")
		}

		// Normalize parameter type
		normalizedType := jsonutils.NormalizeParameterType(paramType)

		// Validate parameter type
		validTypes := map[string]bool{"string": true, "int": true, "float": true, "bool": true}
		if !validTypes[normalizedType] {
			return nil, fmt.Errorf("invalid parameter type: %s, supported types: string, int, float, bool", paramType)
		}

		parameters = append(parameters, Parameter{
			Name:     name,
			Type:     normalizedType,
			Required: required,
		})
	}

	return parameters, nil
}

// ExecuteScript executes a shell script or command with the given parameters.
func (s *Server) ExecuteScript(toolName string, args map[string]interface{}) (string, error) {
	tool, exists := s.tools[toolName]
	if !exists {
		return "", fmt.Errorf("tool not found: %s", toolName)
	}

	// Set up environment variables for the script/command
	env := os.Environ()
	for name, value := range args {
		// Convert value to string
		strValue := fmt.Sprintf("%v", value)
		env = append(env, fmt.Sprintf("%s=%s", name, strValue))
	}

	// Determine which shell to use for executing the script/command
	shell := "/bin/sh"
	bashExists, statErr := os.Stat("/bin/bash")
	if statErr == nil && !bashExists.IsDir() {
		shell = "/bin/bash"
	}

	var cmd *exec.Cmd
	if tool.Command != "" {
		// Use the inline command
		// #nosec G204 - Command is validated and comes from a trusted source (config)
		cmd = exec.Command(shell, "-c", tool.Command)
	} else {
		// Use the script file
		scriptPath := filepath.Clean(tool.ScriptPath)
		info, err := os.Stat(scriptPath)
		if err != nil {
			return "", fmt.Errorf("script not found or not accessible: %w", err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("not a script: %s is a directory", scriptPath)
		}
		if info.Mode()&0o111 == 0 {
			return "", fmt.Errorf("script is not executable: %s", scriptPath)
		}
		// #nosec G204 - scriptPath is validated and comes from a trusted source (config)
		cmd = exec.Command(shell, "-c", scriptPath)
	}

	cmd.Env = env
	cmd.Stderr = os.Stderr

	// Execute and capture output
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error executing command: %w", err)
	}

	return string(output), nil
}

// GetToolSchema generates a JSON schema for the tool's parameters.
func (s *Server) GetToolSchema(toolName string) (map[string]interface{}, error) {
	tool, exists := s.tools[toolName]
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}

	properties := make(map[string]interface{})
	required := make([]string, 0, len(tool.Parameters))

	for _, param := range tool.Parameters {
		var paramSchema map[string]interface{}

		switch param.Type {
		case "string":
			paramSchema = map[string]interface{}{
				"type": "string",
			}
		case "int":
			paramSchema = map[string]interface{}{
				"type": "integer",
			}
		case "float":
			paramSchema = map[string]interface{}{
				"type": "number",
			}
		case "bool":
			paramSchema = map[string]interface{}{
				"type": "boolean",
			}
		}

		properties[param.Name] = paramSchema

		// Only add the parameter to required list if it's marked as required
		if param.Required {
			required = append(required, param.Name)
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema, nil
}

// Start begins listening for JSON-RPC requests on stdin and responding on stdout.
func (s *Server) Start() error {
	decoder := json.NewDecoder(os.Stdin)

	s.log("Proxy server started, waiting for requests...")
	fmt.Fprintf(os.Stderr, "Proxy server started, waiting for requests...\n")

	// Check error from Close() when deferring
	defer func() {
		if err := s.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing log file: %v\n", err)
		}
	}()

	for {
		// Request struct with fields ordered for optimal memory alignment
		var request struct {
			Method  string                 `json:"method"`           // string (16 bytes: pointer + len)
			Params  map[string]interface{} `json:"params,omitempty"` // map (8 bytes)
			JSONRPC string                 `json:"jsonrpc"`          // string (16 bytes: pointer + len)
			ID      int                    `json:"id"`               // int (8 bytes)
		}

		fmt.Fprintf(os.Stderr, "Waiting for request...\n")
		if err := decoder.Decode(&request); err != nil {
			if err == io.EOF {
				s.log("Client disconnected (EOF)")
				return nil
			}
			s.log(fmt.Sprintf("Error decoding request: %v", err))
			fmt.Fprintf(os.Stderr, "Error decoding request: %v\n", err)
			return fmt.Errorf("error decoding request: %w", err)
		}

		// Log the incoming request
		s.logJSON("Received request", request)
		fmt.Fprintf(os.Stderr, "Received request: %s (ID: %d)\n", request.Method, request.ID)
		s.id = request.ID

		// Handle notifications (methods without an ID)
		if request.Method == "notifications/initialized" {
			fmt.Fprintf(os.Stderr, "Received initialization notification\n")
			s.log("Received initialization notification")
			continue
		}

		var response any
		var err error

		switch request.Method {
		case "initialize":
			response = s.handleInitialize(request.Params)
		case "tools/list":
			response = s.handleToolsList()
		case "tools/call":
			response, err = s.handleToolCall(request.Params)
		default:
			err = fmt.Errorf("method not found")
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error handling request: %v\n", err)
			s.log(fmt.Sprintf("Error handling request: %v", err))
			s.writeError(err)
			continue
		}

		fmt.Fprintf(os.Stderr, "Sending response\n")
		s.writeResponse(response)
	}
}

// handleInitialize handles the initialize request from the client.
func (s *Server) handleInitialize(params map[string]interface{}) map[string]interface{} {
	// Log the initialization parameters
	if clientInfo, ok := params["clientInfo"].(map[string]interface{}); ok {
		clientName, _ := clientInfo["name"].(string)
		clientVersion, _ := clientInfo["version"].(string)
		fmt.Fprintf(os.Stderr, "Client initialized: %s v%s\n", clientName, clientVersion)
	}

	// Extract protocol version from params, defaulting to latest if not provided
	protocolVersion := "2024-11-05"
	if version, ok := params["protocolVersion"].(string); ok {
		protocolVersion = version
	}

	// Return server information and capabilities in the format expected by clients
	capabilities := map[string]interface{}{
		"tools": map[string]interface{}{},
	}

	return map[string]interface{}{
		"protocolVersion": protocolVersion,
		"capabilities":    capabilities,
		"serverInfo": map[string]interface{}{
			"name":    "mcp-proxy-server",
			"version": "1.0.0",
		},
	}
}

// handleToolsList returns the list of available tools.
func (s *Server) handleToolsList() map[string]interface{} {
	tools := make([]map[string]interface{}, 0, len(s.tools))

	for _, tool := range s.tools {
		// Generate schema directly from the tool parameters
		properties := make(map[string]interface{})
		required := make([]string, 0, len(tool.Parameters))

		for _, param := range tool.Parameters {
			var paramSchema map[string]interface{}

			switch param.Type {
			case "string":
				paramSchema = map[string]interface{}{
					"type": "string",
				}
			case "int":
				paramSchema = map[string]interface{}{
					"type": "integer",
				}
			case "float":
				paramSchema = map[string]interface{}{
					"type": "number",
				}
			case "bool":
				paramSchema = map[string]interface{}{
					"type": "boolean",
				}
			}

			properties[param.Name] = paramSchema

			// Only add to required list if the parameter is required
			if param.Required {
				required = append(required, param.Name)
			}
		}

		schema := map[string]interface{}{
			"type":       "object",
			"properties": properties,
		}

		if len(required) > 0 {
			schema["required"] = required
		}

		tools = append(tools, map[string]interface{}{
			"name":        tool.Name,
			"description": tool.Description,
			"inputSchema": schema,
		})
	}

	return map[string]interface{}{
		"tools": tools,
	}
}

// handleToolCall handles a tool call request.
func (s *Server) handleToolCall(params map[string]interface{}) (map[string]interface{}, error) {
	nameValue, ok := params["name"]
	if !ok {
		return nil, fmt.Errorf("missing 'name' parameter")
	}

	name, ok := nameValue.(string)
	if !ok {
		return nil, fmt.Errorf("'name' parameter must be a string")
	}

	tool, exists := s.tools[name]
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	// Extract input arguments
	argumentsValue, ok := params["arguments"]
	if !ok {
		return nil, fmt.Errorf("missing 'arguments' parameter")
	}

	arguments, ok := argumentsValue.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("'arguments' parameter must be an object")
	}

	// Check for required parameters
	for _, param := range tool.Parameters {
		if param.Required {
			if _, exists := arguments[param.Name]; !exists {
				return nil, fmt.Errorf("missing required parameter: %s", param.Name)
			}
		}
	}

	// Log the input parameters
	s.logJSON("Tool input", arguments)

	// Execute the shell script
	output, err := s.ExecuteScript(name, arguments)
	if err != nil {
		s.log(fmt.Sprintf("Error executing script: %v", err))
		return nil, fmt.Errorf("error executing script: %w", err)
	}

	// Log the output
	s.log(fmt.Sprintf("Script output: %s", output))

	// Return the output in the correct format for the MCP protocol
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": output,
			},
		},
	}, nil
}

// writeResponse writes a successful JSON-RPC response to stdout.
func (s *Server) writeResponse(result any) {
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      s.id,
		"result":  result,
	}

	// Log the outgoing response
	s.logJSON("Sending response", response)

	err := json.NewEncoder(os.Stdout).Encode(response)
	if err != nil {
		s.log(fmt.Sprintf("Error encoding response: %v", err))
		fmt.Fprintf(os.Stderr, "Error encoding response: %v\n", err)
	}
}

// writeError writes a JSON-RPC error response to stdout.
func (s *Server) writeError(err error) {
	// Use method not found error code for unsupported methods
	code := -32000 // Default server error
	if err.Error() == "method not found" {
		code = -32601 // Method not found error code
	}

	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      s.id,
		"error": map[string]interface{}{
			"code":    code,
			"message": err.Error(),
		},
	}

	// Log the outgoing error response
	s.logJSON("Sending error response", response)

	encodeErr := json.NewEncoder(os.Stdout).Encode(response)
	if encodeErr != nil {
		s.log(fmt.Sprintf("Error encoding error response: %v", encodeErr))
		fmt.Fprintf(os.Stderr, "Error encoding error response: %v\n", encodeErr)
	}
}

// RunProxyServer creates and runs a proxy server with the specified tool configs.
func RunProxyServer(toolConfigs map[string]map[string]string) error {
	server, err := NewProxyServer()
	if err != nil {
		return fmt.Errorf("error creating server: %w", err)
	}

	// Add tools from configs
	for name, config := range toolConfigs {
		description := config["description"]
		parameters := config["parameters"]
		scriptPath := config["script"]
		command := config["command"]

		addErr := server.AddTool(name, description, parameters, scriptPath, command)
		if addErr != nil {
			return fmt.Errorf("error adding tool %s: %w", name, addErr)
		}
	}

	// Print registered tools
	fmt.Fprintln(os.Stderr, "Registered proxy tools:")
	for name, tool := range server.tools {
		fmt.Fprintf(os.Stderr, "- %s: %s (%s: %s)\n", name, tool.Description,
			map[bool]string{true: "script", false: "command"}[tool.ScriptPath != ""],
			map[bool]string{true: tool.ScriptPath, false: tool.Command}[tool.ScriptPath != ""])
		paramStr := ""
		for i, param := range tool.Parameters {
			if i > 0 {
				paramStr += ", "
			}
			if param.Required {
				paramStr += param.Name + ":" + param.Type
			} else {
				paramStr += "[" + param.Name + ":" + param.Type + "]"
			}
		}
		if paramStr != "" {
			fmt.Fprintf(os.Stderr, "  Parameters: %s\n", paramStr)
		}
	}

	server.log(fmt.Sprintf("Starting proxy server with %d tools", len(toolConfigs)))
	return server.Start()
}
