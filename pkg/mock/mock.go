// Package mock provides a simple implementation of an MCP server for testing.
package mock

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Tool represents a mock tool in the MCP protocol.
type Tool struct {
	Name        string
	Description string
}

// Prompt represents a mock prompt in the MCP protocol.
type Prompt struct {
	Name        string
	Description string
	Template    string
}

// Resource represents a mock resource in the MCP protocol.
type Resource struct {
	URI         string
	Description string
	Content     string
}

// Server is a mock MCP server that responds to JSON-RPC requests.
type Server struct {
	// Fields ordered for optimal memory alignment (8-byte aligned fields first)
	tools     map[string]Tool     // pointer (8 bytes)
	prompts   map[string]Prompt   // pointer (8 bytes)
	resources map[string]Resource // pointer (8 bytes)
	logFile   *os.File            // pointer (8 bytes)
	id        int                 // int (8 bytes)
}

// NewServer creates a new mock MCP server.
func NewServer() (*Server, error) {
	// Create log directory - using a fixed, safe path
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

	// Open log file - using a fixed, safe path
	logPath := filepath.Join(logDir, "mock.log")
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
		id:        0,
		tools:     make(map[string]Tool),
		prompts:   make(map[string]Prompt),
		resources: make(map[string]Resource),
		logFile:   logFile,
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

// AddTool adds a new tool to the mock server.
func (s *Server) AddTool(name, description string) {
	s.tools[name] = Tool{
		Name:        name,
		Description: description,
	}
}

// AddPrompt adds a new prompt to the mock server.
func (s *Server) AddPrompt(name, description, template string) {
	s.prompts[name] = Prompt{
		Name:        name,
		Description: description,
		Template:    template,
	}
}

// AddResource adds a new resource to the mock server.
func (s *Server) AddResource(uri, description, content string) {
	s.resources[uri] = Resource{
		URI:         uri,
		Description: description,
		Content:     content,
	}
}

// Start begins listening for JSON-RPC requests on stdin and responding on stdout.
func (s *Server) Start() error {
	decoder := json.NewDecoder(os.Stdin)

	s.log("Mock server started, waiting for requests...")
	fmt.Fprintf(os.Stderr, "Mock server started, waiting for requests...\n")

	// Check error from Close() when deferring
	defer func() {
		if err := s.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing log file: %v\n", err)
		}
	}()

	for {
		// Request struct with fields ordered for optimal memory alignment
		var request struct {
			Method  string         `json:"method"`           // string (16 bytes: pointer + len)
			Params  map[string]any `json:"params,omitempty"` // map (8 bytes)
			JSONRPC string         `json:"jsonrpc"`          // string (16 bytes: pointer + len)
			ID      int            `json:"id"`               // int (8 bytes)
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
		case "resources/list":
			response = s.handleResourcesList()
		case "resources/read":
			response, err = s.handleResourceRead(request.Params)
		case "prompts/list":
			response = s.handlePromptsList()
		case "prompts/get":
			response, err = s.handlePromptGet(request.Params)
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

// StartHTTP begins listening for HTTP requests on the specified port.
func (s *Server) StartHTTP(port string) error {
	s.log("HTTP mock server started, setting up routes...")
	fmt.Fprintf(os.Stderr, "HTTP mock server started on http://localhost:%s\n", port)
	fmt.Fprintf(os.Stderr, "JSON-RPC endpoint: http://localhost:%s/mcp\n", port)

	// Check error from Close() when deferring
	defer func() {
		if err := s.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing log file: %v\n", err)
		}
	}()

	mux := http.NewServeMux()
	
	// Main MCP JSON-RPC endpoint
	mux.HandleFunc("/mcp", s.handleMCP)
	
	// SSE endpoint for client connections (for compatibility)
	mux.HandleFunc("/sse", s.handleSSE)
	
	// Messages endpoint for JSON-RPC requests (for SSE compatibility)
	mux.HandleFunc("/messages", s.handleMessages)

	// Add a health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Start the HTTP server
	fmt.Fprintf(os.Stderr, "Starting HTTP server on port %s...\n", port)
	//nolint:gosec // This is a development/testing tool
	return http.ListenAndServe(":"+port, mux)
}

// handleInitialize handles the initialize request from the client.
func (s *Server) handleInitialize(params map[string]any) map[string]any {
	// Log the initialization parameters
	if clientInfo, ok := params["clientInfo"].(map[string]any); ok {
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
	capabilities := map[string]any{
		"tools": map[string]any{},
	}

	if len(s.prompts) > 0 {
		capabilities["prompts"] = map[string]any{}
	}

	if len(s.resources) > 0 {
		capabilities["resources"] = map[string]any{}
	}

	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    capabilities,
		"serverInfo": map[string]any{
			"name":    "mcp-mock-server",
			"version": "1.0.0",
		},
	}
}

// handleToolsList returns the list of available tools.
func (s *Server) handleToolsList() map[string]any {
	tools := make([]map[string]any, 0, len(s.tools))

	for _, tool := range s.tools {
		tools = append(tools, map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		})
	}

	return map[string]any{
		"tools": tools,
	}
}

// handleToolCall handles a tool call request.
func (s *Server) handleToolCall(params map[string]any) (map[string]any, error) {
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

	// Return a mock response in the correct format for the MCP protocol
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": fmt.Sprintf("hello i am %s mock tool and i confirm it's working", tool.Name),
			},
		},
	}, nil
}

// handleResourcesList returns the list of available resources.
func (s *Server) handleResourcesList() map[string]any {
	if len(s.resources) == 0 {
		return map[string]any{
			"resources": []map[string]any{},
		}
	}

	resources := make([]map[string]any, 0, len(s.resources))

	for _, resource := range s.resources {
		resources = append(resources, map[string]any{
			"uri":         resource.URI,
			"name":        resource.URI, // Using URI as name if not specified
			"description": resource.Description,
			"mimeType":    "text/plain",
		})
	}

	return map[string]any{
		"resources": resources,
	}
}

// handleResourceRead handles a resource read request.
func (s *Server) handleResourceRead(params map[string]any) (map[string]any, error) {
	uriValue, ok := params["uri"]
	if !ok {
		return nil, fmt.Errorf("missing 'uri' parameter")
	}

	uri, ok := uriValue.(string)
	if !ok {
		return nil, fmt.Errorf("'uri' parameter must be a string")
	}

	resource, exists := s.resources[uri]
	if !exists {
		return nil, fmt.Errorf("resource not found: %s", uri)
	}

	// Return the resource content in the required format
	return map[string]any{
		"contents": []map[string]any{
			{
				"uri":      resource.URI,
				"mimeType": "text/plain",
				"text":     resource.Content,
			},
		},
	}, nil
}

// handlePromptsList returns the list of available prompts.
func (s *Server) handlePromptsList() map[string]any {
	if len(s.prompts) == 0 {
		return map[string]any{
			"prompts": []map[string]any{},
		}
	}

	prompts := make([]map[string]any, 0, len(s.prompts))

	for _, prompt := range s.prompts {
		// Extract arguments from the template
		arguments := extractArgumentsFromTemplate(prompt.Template)

		promptInfo := map[string]any{
			"name":        prompt.Name,
			"description": prompt.Description,
		}

		// Only include arguments if there are any
		if len(arguments) > 0 {
			promptInfo["arguments"] = arguments
		}

		prompts = append(prompts, promptInfo)
	}

	return map[string]any{
		"prompts": prompts,
	}
}

// extractArgumentsFromTemplate parses a template string to find placeholders in the format {{argument_name}}
// and returns a list of argument objects.
func extractArgumentsFromTemplate(template string) []map[string]any {
	// Simple implementation - in a real scenario, you might want to use regex
	var arguments []map[string]any

	// Find all occurrences of {{...}} in the template
	startIndex := 0
	for {
		start := strings.Index(template[startIndex:], "{{")
		if start == -1 {
			break
		}
		start += startIndex

		end := strings.Index(template[start:], "}}")
		if end == -1 {
			break
		}
		end += start

		// Extract the argument name from between {{ and }}
		argName := strings.TrimSpace(template[start+2 : end])
		if argName != "" {
			// Check if this argument is already in our list
			found := false
			for _, arg := range arguments {
				if name, ok := arg["name"].(string); ok && name == argName {
					found = true
					break
				}
			}

			if !found {
				// Add the argument to our list
				arguments = append(arguments, map[string]any{
					"name":        argName,
					"description": argName,
					"required":    true,
				})
			}
		}

		startIndex = end + 2
	}

	return arguments
}

// handlePromptGet handles a prompt get request.
func (s *Server) handlePromptGet(params map[string]any) (map[string]any, error) {
	nameValue, ok := params["name"]
	if !ok {
		return nil, fmt.Errorf("missing 'name' parameter")
	}

	name, ok := nameValue.(string)
	if !ok {
		return nil, fmt.Errorf("'name' parameter must be a string")
	}

	prompt, exists := s.prompts[name]
	if !exists {
		return nil, fmt.Errorf("prompt not found: %s", name)
	}

	// Format the template with arguments if available
	content := prompt.Template

	// Get arguments if provided and substitute them in the template
	if argsValue, hasArgs := params["arguments"]; hasArgs {
		if args, isMap := argsValue.(map[string]any); isMap {
			fmt.Fprintf(os.Stderr, "Prompt arguments received: %v\n", args)

			// Simple placeholder substitution
			for argName, argValue := range args {
				placeholder := "{{" + argName + "}}"

				// Convert argValue to string for substitution
				var stringValue string
				switch v := argValue.(type) {
				case string:
					stringValue = v
				default:
					// Use fmt.Sprintf for any other type
					stringValue = fmt.Sprintf("%v", v)
				}

				content = strings.ReplaceAll(content, placeholder, stringValue)
			}
		}
	}

	// Create a user message with the content
	message := map[string]any{
		"role": "user",
		"content": map[string]any{
			"type": "text",
			"text": content,
		},
	}

	// Return the prompt in the correct format
	return map[string]any{
		"description": prompt.Description,
		"messages": []map[string]any{
			message,
		},
	}, nil
}

// writeResponse writes a successful JSON-RPC response to stdout.
func (s *Server) writeResponse(result any) {
	response := map[string]any{
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

	response := map[string]any{
		"jsonrpc": "2.0",
		"id":      s.id,
		"error": map[string]any{
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

// RunMockServer creates and runs a mock MCP server with the specified entities.
func RunMockServer(tools map[string]string, prompts map[string]map[string]string, resources map[string]map[string]string) error {
	server, err := NewServer()
	if err != nil {
		return fmt.Errorf("error creating server: %w", err)
	}

	// Add tools
	for name, desc := range tools {
		server.AddTool(name, desc)
	}

	// Add prompts
	for name, promptInfo := range prompts {
		desc := promptInfo["description"]
		template := promptInfo["template"]
		server.AddPrompt(name, desc, template)
	}

	// Add resources
	for uri, resourceInfo := range resources {
		desc := resourceInfo["description"]
		content := resourceInfo["content"]
		server.AddResource(uri, desc, content)
	}

	server.log(fmt.Sprintf("Starting mock server with %d tools, %d prompts, and %d resources",
		len(tools), len(prompts), len(resources)))

	return server.Start()
}

// RunMockServerHTTP creates and runs a mock MCP server with HTTP SSE transport.
func RunMockServerHTTP(tools map[string]string, prompts map[string]map[string]string, resources map[string]map[string]string, port string) error {
	server, err := NewServer()
	if err != nil {
		return fmt.Errorf("error creating server: %w", err)
	}

	// Add tools
	for name, desc := range tools {
		server.AddTool(name, desc)
	}

	// Add prompts
	for name, promptInfo := range prompts {
		desc := promptInfo["description"]
		template := promptInfo["template"]
		server.AddPrompt(name, desc, template)
	}

	// Add resources
	for uri, resourceInfo := range resources {
		desc := resourceInfo["description"]
		content := resourceInfo["content"]
		server.AddResource(uri, desc, content)
	}

	server.log(fmt.Sprintf("Starting HTTP mock server with %d tools, %d prompts, and %d resources on port %s",
		len(tools), len(prompts), len(resources), port))

	return server.StartHTTP(port)
}

// handleSSE handles the SSE endpoint for client connections.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	s.log("Client connected to SSE endpoint")
	fmt.Fprintf(os.Stderr, "Client connected to SSE endpoint\n")

	// Keep the connection alive
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Send keep-alive ping
			fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-r.Context().Done():
			s.log("SSE client disconnected")
			fmt.Fprintf(os.Stderr, "SSE client disconnected\n")
			return
		}
	}
}

// handleMessages handles JSON-RPC message requests.
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse JSON-RPC request
	var request struct {
		Method  string         `json:"method"`
		Params  map[string]any `json:"params,omitempty"`
		JSONRPC string         `json:"jsonrpc"`
		ID      int            `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.log(fmt.Sprintf("Error decoding HTTP request: %v", err))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Log the incoming request
	s.logJSON("Received HTTP request", request)
	fmt.Fprintf(os.Stderr, "Received HTTP request: %s (ID: %d)\n", request.Method, request.ID)
	s.id = request.ID

	// Handle notifications
	if request.Method == "notifications/initialized" {
		fmt.Fprintf(os.Stderr, "Received initialization notification\n")
		s.log("Received initialization notification")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	var response any
	var err error

	// Handle requests using existing logic
	switch request.Method {
	case "initialize":
		response = s.handleInitialize(request.Params)
	case "tools/list":
		response = s.handleToolsList()
	case "tools/call":
		response, err = s.handleToolCall(request.Params)
	case "resources/list":
		response = s.handleResourcesList()
	case "resources/read":
		response, err = s.handleResourceRead(request.Params)
	case "prompts/list":
		response = s.handlePromptsList()
	case "prompts/get":
		response, err = s.handlePromptGet(request.Params)
	default:
		err = fmt.Errorf("method not found")
	}

	// Prepare JSON-RPC response
	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error handling HTTP request: %v\n", err)
		s.log(fmt.Sprintf("Error handling HTTP request: %v", err))
		
		errorResponse := map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"error": map[string]any{
				"code":    -32601,
				"message": err.Error(),
			},
		}
		
		w.WriteHeader(http.StatusOK) // JSON-RPC errors are still HTTP 200
		json.NewEncoder(w).Encode(errorResponse)
		return
	}

	// Send successful response
	successResponse := map[string]any{
		"jsonrpc": "2.0",
		"id":      request.ID,
		"result":  response,
	}

	fmt.Fprintf(os.Stderr, "Sending HTTP response\n")
	s.logJSON("Sending HTTP response", successResponse)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(successResponse)
}

// handleMCP handles direct JSON-RPC requests to the /mcp endpoint.
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse JSON-RPC request
	var request struct {
		Method  string         `json:"method"`
		Params  map[string]any `json:"params,omitempty"`
		JSONRPC string         `json:"jsonrpc"`
		ID      int            `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.log(fmt.Sprintf("Error decoding MCP request: %v", err))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Log the incoming request
	s.logJSON("Received MCP request", request)
	fmt.Fprintf(os.Stderr, "Received MCP request: %s (ID: %d)\n", request.Method, request.ID)
	s.id = request.ID

	// Handle notifications
	if request.Method == "notifications/initialized" {
		fmt.Fprintf(os.Stderr, "Received initialization notification\n")
		s.log("Received initialization notification")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	var response any
	var err error

	// Handle requests using existing logic
	switch request.Method {
	case "initialize":
		response = s.handleInitialize(request.Params)
	case "tools/list":
		response = s.handleToolsList()
	case "tools/call":
		response, err = s.handleToolCall(request.Params)
	case "resources/list":
		response = s.handleResourcesList()
	case "resources/read":
		response, err = s.handleResourceRead(request.Params)
	case "prompts/list":
		response = s.handlePromptsList()
	case "prompts/get":
		response, err = s.handlePromptGet(request.Params)
	default:
		err = fmt.Errorf("method not found")
	}

	// Prepare JSON-RPC response
	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error handling MCP request: %v\n", err)
		s.log(fmt.Sprintf("Error handling MCP request: %v", err))
		
		errorResponse := map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"error": map[string]any{
				"code":    -32601,
				"message": err.Error(),
			},
		}
		
		w.WriteHeader(http.StatusOK) // JSON-RPC errors are still HTTP 200
		json.NewEncoder(w).Encode(errorResponse)
		return
	}

	// Send successful response
	successResponse := map[string]any{
		"jsonrpc": "2.0",
		"id":      request.ID,
		"result":  response,
	}

	fmt.Fprintf(os.Stderr, "Sending MCP response\n")
	s.logJSON("Sending MCP response", successResponse)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(successResponse)
}
