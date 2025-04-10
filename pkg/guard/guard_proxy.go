// Package guard provides functionality for filtering MCP tool requests.
package guard

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FilterServer handles proxying requests and filtering tools, prompts, and resources.
type FilterServer struct {
	allowPatterns map[string][]string
	denyPatterns  map[string][]string
	logFile       *os.File
	id            int
}

// NewFilterServer creates a new filter server.
func NewFilterServer(allowPatterns, denyPatterns map[string][]string) (*FilterServer, error) {
	// Create log directory
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		return nil, fmt.Errorf("HOME environment variable not set")
	}

	logDir := filepath.Join(homeDir, ".mcpt", "logs")
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		return nil, fmt.Errorf("error creating log directory: %w", err)
	}

	// Open log file
	logPath := filepath.Join(logDir, "guard.log")
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

	return &FilterServer{
		allowPatterns: allowPatterns,
		denyPatterns:  denyPatterns,
		id:            0,
		logFile:       logFile,
	}, nil
}

// log writes a message to the log file with a timestamp.
func (s *FilterServer) log(message string) {
	timestamp := time.Now().Format(time.RFC3339)
	fmt.Fprintf(s.logFile, "[%s] %s\n", timestamp, message)
}

// logJSON writes a JSON-formatted message to the log file with a timestamp.
func (s *FilterServer) logJSON(label string, v any) {
	jsonBytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		s.log(fmt.Sprintf("Error marshaling %s: %v", label, err))
		return
	}
	s.log(fmt.Sprintf("%s: %s", label, string(jsonBytes)))
}

// Close closes the log file.
func (s *FilterServer) Close() error {
	if s.logFile != nil {
		return s.logFile.Close()
	}
	return nil
}

// IsAllowed determines if a name is allowed based on the configured patterns.
func (s *FilterServer) IsAllowed(entityType, name string) bool {
	// Default: allow if no allow patterns
	allowed := len(s.allowPatterns[entityType]) == 0

	// If allow patterns exist, check if name matches any
	for _, pattern := range s.allowPatterns[entityType] {
		match, _ := filepath.Match(pattern, name)
		if match {
			allowed = true
			break
		}
	}

	// Even if allowed, check if name is denied
	for _, pattern := range s.denyPatterns[entityType] {
		match, _ := filepath.Match(pattern, name)
		if match {
			allowed = false
			break
		}
	}

	return allowed
}

// filterResponse filters the response based on the allow and deny patterns.
func (s *FilterServer) filterResponse(entityType string, resp map[string]interface{}) map[string]interface{} {
	switch entityType {
	case "tool":
		return s.filterToolsResponse(resp)
	case "prompt":
		return s.filterPromptsResponse(resp)
	case "resource":
		return s.filterResourcesResponse(resp)
	default:
		return resp
	}
}

// filterToolsResponse filters the tools in a tools/list response.
func (s *FilterServer) filterToolsResponse(resp map[string]interface{}) map[string]interface{} {
	// Extract the tools from the response
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return resp
	}

	tools, ok := result["tools"].([]interface{})
	if !ok {
		return resp
	}

	// Filter the tools
	filteredTools := make([]interface{}, 0, len(tools))
	for _, tool := range tools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := toolMap["name"].(string)
		if !ok {
			continue
		}

		if s.IsAllowed("tool", name) {
			filteredTools = append(filteredTools, tool)
		} else {
			s.log(fmt.Sprintf("Filtered tool: %s", name))
		}
	}

	// Update the response
	result["tools"] = filteredTools
	resp["result"] = result
	return resp
}

// filterPromptsResponse filters the prompts in a prompts/list response.
func (s *FilterServer) filterPromptsResponse(resp map[string]interface{}) map[string]interface{} {
	// Extract the prompts from the response
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return resp
	}

	prompts, ok := result["prompts"].([]interface{})
	if !ok {
		return resp
	}

	// Filter the prompts
	filteredPrompts := make([]interface{}, 0, len(prompts))
	for _, prompt := range prompts {
		promptMap, ok := prompt.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := promptMap["name"].(string)
		if !ok {
			continue
		}

		if s.IsAllowed("prompt", name) {
			filteredPrompts = append(filteredPrompts, prompt)
		} else {
			s.log(fmt.Sprintf("Filtered prompt: %s", name))
		}
	}

	// Update the response
	result["prompts"] = filteredPrompts
	resp["result"] = result
	return resp
}

// filterResourcesResponse filters the resources in a resources/list response.
func (s *FilterServer) filterResourcesResponse(resp map[string]interface{}) map[string]interface{} {
	// Extract the resources from the response
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return resp
	}

	resources, ok := result["resources"].([]interface{})
	if !ok {
		return resp
	}

	// Filter the resources
	filteredResources := make([]interface{}, 0, len(resources))
	for _, resource := range resources {
		resourceMap, ok := resource.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := resourceMap["name"].(string)
		if !ok {
			continue
		}

		if s.IsAllowed("resource", name) {
			filteredResources = append(filteredResources, resource)
		} else {
			s.log(fmt.Sprintf("Filtered resource: %s", name))
		}
	}

	// Update the response
	result["resources"] = filteredResources
	resp["result"] = result
	return resp
}

// Start begins listening for JSON-RPC requests on stdin, proxying them to the child process,
// and responding on stdout with filtered responses.
func (s *FilterServer) Start(cmdArgs []string) error {
	// Launch the child process
	childCmd := NewChildProcess(cmdArgs)
	if err := childCmd.Start(); err != nil {
		return fmt.Errorf("error starting child process: %w", err)
	}
	defer func() {
		if err := childCmd.Close(); err != nil {
			s.log(fmt.Sprintf("Error closing child process: %v", err))
		}
	}()

	// Decoder for client requests (stdin)
	clientDecoder := json.NewDecoder(os.Stdin)

	// Encoder for client responses (stdout)
	clientEncoder := json.NewEncoder(os.Stdout)

	// Decoder for child process responses (childCmd.Stdout)
	childDecoder := json.NewDecoder(childCmd.Stdout)

	s.log("Guard proxy started, waiting for requests...")
	fmt.Fprintf(os.Stderr, "Guard proxy started, waiting for requests...\n")

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
		if err := clientDecoder.Decode(&request); err != nil {
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

		// Filter tool calls if necessary
		if request.Method == "tools/call" {
			if name, ok := request.Params["name"].(string); ok {
				if !s.IsAllowed("tool", name) {
					s.log(fmt.Sprintf("Blocked call to filtered tool: %s", name))
					s.writeError(fmt.Errorf("tool not found: %s", name))
					continue
				}
			}
		}

		// Forward the request to the child process
		if err := json.NewEncoder(childCmd.Stdin).Encode(request); err != nil {
			s.log(fmt.Sprintf("Error forwarding request to child: %v", err))
			s.writeError(fmt.Errorf("error forwarding request: %w", err))
			continue
		}

		// Read the response from the child process
		var response map[string]interface{}
		if err := childDecoder.Decode(&response); err != nil {
			if err == io.EOF {
				s.log("Child process disconnected (EOF)")
				return fmt.Errorf("child process disconnected unexpectedly")
			}
			s.log(fmt.Sprintf("Error reading response from child: %v", err))
			s.writeError(fmt.Errorf("error reading response: %w", err))
			continue
		}

		// Apply filtering based on the request method
		switch request.Method {
		case "tools/list":
			response = s.filterResponse("tool", response)
		case "prompts/list":
			response = s.filterResponse("prompt", response)
		case "resources/list":
			response = s.filterResponse("resource", response)
		}

		// Forward the filtered response to the client
		s.logJSON("Sending response", response)
		if err := clientEncoder.Encode(response); err != nil {
			s.log(fmt.Sprintf("Error sending response to client: %v", err))
			fmt.Fprintf(os.Stderr, "Error sending response to client: %v\n", err)
		}
	}
}

// writeError writes a JSON-RPC error response to stdout.
func (s *FilterServer) writeError(err error) {
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

// RunFilterServer creates and runs a filter server with the specified patterns and command.
func RunFilterServer(allowPatterns, denyPatterns map[string][]string, cmdArgs []string) error {
	server, err := NewFilterServer(allowPatterns, denyPatterns)
	if err != nil {
		return fmt.Errorf("error creating server: %w", err)
	}

	// Print filtering patterns
	fmt.Fprintln(os.Stderr, "Guard proxy with filtering:")
	for entityType, patterns := range allowPatterns {
		if len(patterns) > 0 {
			fmt.Fprintf(os.Stderr, "- Allowing %s matching: %s\n", entityType, strings.Join(patterns, ", "))
		}
	}
	for entityType, patterns := range denyPatterns {
		if len(patterns) > 0 {
			fmt.Fprintf(os.Stderr, "- Denying %s matching: %s\n", entityType, strings.Join(patterns, ", "))
		}
	}

	server.log(fmt.Sprintf("Starting guard proxy for command: %s", strings.Join(cmdArgs, " ")))
	return server.Start(cmdArgs)
}
