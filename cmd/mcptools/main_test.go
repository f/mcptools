package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/f/mcptools/pkg/proxy"
	"github.com/f/mcptools/pkg/transport"
)

const entityTypeValue = "tool"

type MockTransport struct {
	Responses map[string]map[string]interface{}
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		Responses: map[string]map[string]interface{}{},
	}
}

func (t *MockTransport) Execute(method string, params interface{}) (map[string]interface{}, error) {
	if resp, ok := t.Responses[method]; ok {
		return resp, nil
	}

	if method == "tools/list" {
		return map[string]interface{}{
			"tools": []map[string]interface{}{
				{
					"name":        "test_tool",
					"description": "A test tool",
				},
				{
					"name":        "another_tool",
					"description": "Another test tool",
				},
			},
		}, nil
	}

	if method == "tools/call" {
		paramsMap := params.(map[string]interface{})
		toolName := paramsMap["name"].(string)
		return map[string]interface{}{
			"result": fmt.Sprintf("Called tool: %s", toolName),
		}, nil
	}

	if method == "resources/list" {
		return map[string]interface{}{
			"resources": []map[string]interface{}{
				{
					"uri":         "test_resource",
					"description": "A test resource",
				},
			},
		}, nil
	}

	if method == "resources/read" {
		paramsMap := params.(map[string]interface{})
		uri := paramsMap["uri"].(string)
		return map[string]interface{}{
			"content": fmt.Sprintf("Content of resource: %s", uri),
		}, nil
	}

	if method == "prompts/list" {
		return map[string]interface{}{
			"prompts": []map[string]interface{}{
				{
					"name":        "test_prompt",
					"description": "A test prompt",
				},
			},
		}, nil
	}

	if method == "prompts/get" {
		paramsMap := params.(map[string]interface{})
		promptName := paramsMap["name"].(string)
		return map[string]interface{}{
			"content": fmt.Sprintf("Content of prompt: %s", promptName),
		}, nil
	}

	return map[string]interface{}{}, fmt.Errorf("unknown method: %s", method)
}

type Shell struct {
	Transport transport.Transport
	Reader    io.Reader
	Writer    io.Writer
	Format    string
}

func (s *Shell) Run() {
	scanner := bufio.NewScanner(s.Reader)

	for scanner.Scan() {
		input := scanner.Text()

		if input == "/q" || input == "/quit" || input == "exit" {
			fmt.Fprintln(s.Writer, "Exiting MCP shell")
			break
		}

		parts := strings.Fields(input)
		if len(parts) == 0 {
			continue
		}

		command := parts[0]
		args := parts[1:]

		switch command {
		case "tools":
			resp, _ := s.Transport.Execute("tools/list", nil)
			fmt.Fprintln(s.Writer, "Tools:", resp)

		case "resources":
			resp, _ := s.Transport.Execute("resources/list", nil)
			fmt.Fprintln(s.Writer, "Resources:", resp)

		case "prompts":
			resp, _ := s.Transport.Execute("prompts/list", nil)
			fmt.Fprintln(s.Writer, "Prompts:", resp)

		case "call":
			if len(args) < 1 {
				fmt.Fprintln(s.Writer, "Usage: call <entity> [--params '{...}']")
				continue
			}

			entityName := args[0]
			entityType := entityTypeValue

			parts = strings.SplitN(entityName, ":", 2)
			if len(parts) == 2 {
				entityType = parts[0]
				entityName = parts[1]
			}

			params := map[string]interface{}{}

			for i := 1; i < len(args); i++ {
				if args[i] == "--params" || args[i] == "-p" {
					if i+1 < len(args) {
						_ = json.Unmarshal([]byte(args[i+1]), &params)
						break
					}
				}
			}

			var resp map[string]interface{}

			switch entityType {
			case "tool":
				resp, _ = s.Transport.Execute("tools/call", map[string]interface{}{
					"name":      entityName,
					"arguments": params,
				})
			case "resource":
				resp, _ = s.Transport.Execute("resources/read", map[string]interface{}{
					"uri": entityName,
				})
			case "prompt":
				resp, _ = s.Transport.Execute("prompts/get", map[string]interface{}{
					"name": entityName,
				})
			}

			fmt.Fprintln(s.Writer, "Call result:", resp)

		default:
			entityName := command
			entityType := entityTypeValue

			parts = strings.SplitN(entityName, ":", 2)
			if len(parts) == 2 {
				entityType = parts[0]
				entityName = parts[1]
			}

			params := map[string]interface{}{}

			if len(args) > 0 {
				firstArg := args[0]
				if strings.HasPrefix(firstArg, "{") && strings.HasSuffix(firstArg, "}") {
					_ = json.Unmarshal([]byte(firstArg), &params)
				} else {
					for i := 0; i < len(args); i++ {
						if args[i] == "--params" || args[i] == "-p" {
							if i+1 < len(args) {
								_ = json.Unmarshal([]byte(args[i+1]), &params)
								break
							}
						}
					}
				}
			}

			var resp map[string]interface{}

			switch entityType {
			case "tool":
				resp, _ = s.Transport.Execute("tools/call", map[string]interface{}{
					"name":      entityName,
					"arguments": params,
				})
				fmt.Fprintln(s.Writer, "Direct tool call result:", resp)
			case "resource":
				resp, _ = s.Transport.Execute("resources/read", map[string]interface{}{
					"uri": entityName,
				})
				fmt.Fprintln(s.Writer, "Direct resource read result:", resp)
			case "prompt":
				resp, _ = s.Transport.Execute("prompts/get", map[string]interface{}{
					"name": entityName,
				})
				fmt.Fprintln(s.Writer, "Direct prompt get result:", resp)
			default:
				fmt.Fprintln(s.Writer, "Unknown command:", command)
			}
		}
	}
}

func TestDirectToolCalling(t *testing.T) {
	testCases := []struct {
		input          string
		expectedOutput string
	}{
		{
			input:          "test_tool {\"param\": \"value\"}",
			expectedOutput: "Called tool: test_tool",
		},
		{
			input:          "resource:test_resource",
			expectedOutput: "Content of resource: test_resource",
		},
		{
			input:          "prompt:test_prompt",
			expectedOutput: "Content of prompt: test_prompt",
		},
	}

	mockTransport := NewMockTransport()

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			outBuf := &bytes.Buffer{}

			shell := &Shell{
				Transport: mockTransport,
				Format:    "table",
				Reader:    strings.NewReader(tc.input + "\n/q\n"),
				Writer:    outBuf,
			}

			shell.Run()

			if !strings.Contains(outBuf.String(), tc.expectedOutput) {
				t.Errorf("Expected output to contain %q, got: %s", tc.expectedOutput, outBuf.String())
			}
		})
	}
}

func TestExecuteShell(t *testing.T) {
	mockTransport := NewMockTransport()

	inputs := []string{
		"tools",
		"resources",
		"prompts",
		"call test_tool --params '{\"foo\":\"bar\"}'",
		"test_tool {\"foo\":\"bar\"}",
		"resource:test_resource",
		"prompt:test_prompt",
		"/q",
	}

	expectedOutputs := []string{
		"A test tool",                        // tools command
		"A test resource",                    // resources command
		"A test prompt",                      // prompts command
		"Called tool: test_tool",             // call command
		"Called tool: test_tool",             // direct tool call
		"Content of resource: test_resource", // direct resource read
		"Content of prompt: test_prompt",     // direct prompt get
		"Exiting MCP shell",                  // quit command
	}

	outBuf := &bytes.Buffer{}

	shell := &Shell{
		Transport: mockTransport,
		Format:    "table",
		Reader:    strings.NewReader(strings.Join(inputs, "\n") + "\n"),
		Writer:    outBuf,
	}

	shell.Run()

	output := outBuf.String()
	for _, expected := range expectedOutputs {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain %q, but it doesn't.\nFull output: %s", expected, output)
		}
	}
}

func TestProxyToolRegistration(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()
	if err := os.Setenv("HOME", tmpDir); err != nil {
		t.Fatalf("Failed to set HOME environment variable: %v", err)
	}

	// Create config directory
	configDir := filepath.Join(tmpDir, ".mcpt")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	// Create an empty config file
	configFile := filepath.Join(configDir, "proxy_config.json")
	if err := os.WriteFile(configFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Test cases
	testCases := []struct {
		name        string
		toolName    string
		description string
		parameters  string
		scriptPath  string
		command     string
		expectError bool
	}{
		{
			name:        "register with script",
			toolName:    "add_numbers",
			description: "Adds two numbers",
			parameters:  "a:int,b:int",
			scriptPath:  filepath.Join(tmpDir, "add.sh"),
			command:     "",
			expectError: false,
		},
		{
			name:        "register with inline command",
			toolName:    "add_op",
			description: "Adds given numbers",
			parameters:  "a:int,b:int",
			scriptPath:  "",
			command:     "echo \"$a + $b = $(($a+$b))\"",
			expectError: false,
		},
		{
			name:        "register without script or command",
			toolName:    "invalid",
			description: "Invalid tool",
			parameters:  "x:int",
			scriptPath:  "",
			command:     "",
			expectError: true,
		},
	}

	// Create a temporary script file for the first test case
	if err := os.WriteFile(testCases[0].scriptPath, []byte("#!/bin/sh\necho $a + $b = $(($a+$b))"), 0755); err != nil {
		t.Fatalf("Failed to create script file: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a proxy server
			server, err := proxy.NewProxyServer()
			if err != nil {
				t.Fatalf("Failed to create proxy server: %v", err)
			}
			defer server.Close()

			// Register the tool
			err = server.AddTool(tc.toolName, tc.description, tc.parameters, tc.scriptPath, tc.command)

			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Now let's verify the tool can be executed if we have a valid script or command
			if tc.scriptPath != "" || tc.command != "" {
				result, err := server.ExecuteScript(tc.toolName, map[string]interface{}{
					"a": 5,
					"b": 3,
				})
				if err != nil {
					t.Errorf("Failed to execute script: %v", err)
					return
				}

				if !strings.Contains(result, "5 + 3 = 8") {
					t.Errorf("Unexpected script result: %s", result)
				}
			}
		})
	}
}

func TestProxyToolUnregistration(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()
	if err := os.Setenv("HOME", tmpDir); err != nil {
		t.Fatalf("Failed to set HOME environment variable: %v", err)
	}

	// Create a proxy server
	server, err := proxy.NewProxyServer()
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}
	defer server.Close()

	// Register a tool with an inline command
	toolName := "test_tool"
	err = server.AddTool(
		toolName,
		"Test tool",
		"x:int",
		"",        // no script path
		"echo $x", // inline command
	)
	if err != nil {
		t.Fatalf("Error registering tool: %v", err)
	}

	// Verify the tool exists by executing it
	result, err := server.ExecuteScript(toolName, map[string]interface{}{
		"x": 42,
	})
	if err != nil {
		t.Fatalf("Error executing tool: %v", err)
	}
	if !strings.Contains(result, "42") {
		t.Errorf("Unexpected result: %s", result)
	}

	// The proxy package doesn't have direct unregister functionality, so we'll test
	// that the tool is properly registered in the server's internal map by
	// creating a new server instance that shouldn't have the tool

	newServer, err := proxy.NewProxyServer()
	if err != nil {
		t.Fatalf("Failed to create new proxy server: %v", err)
	}
	defer newServer.Close()

	// Try to execute the tool on the new server (should fail as tools aren't persisted)
	_, err = newServer.ExecuteScript(toolName, map[string]interface{}{
		"x": 42,
	})
	if err == nil {
		t.Error("Expected error executing tool on new server, but got none")
	}
}

func TestShellCommands(t *testing.T) {
	// Create a mock server for testing
	mockServer := NewMockTransport()
	mockServer.Responses = map[string]map[string]interface{}{
		"tools/list": {
			"tools": []map[string]interface{}{
				{
					"name":        "test_tool",
					"description": "A test tool",
				},
			},
		},
		"tools/call": {
			"result": "Called test_tool",
		},
		"resources/list": {
			"resources": []map[string]interface{}{
				{
					"uri":         "test_resource",
					"description": "A test resource",
				},
			},
		},
		"resources/read": {
			"content": "Resource content",
		},
		"prompts/list": {
			"prompts": []map[string]interface{}{
				{
					"name":        "test_prompt",
					"description": "A test prompt",
				},
			},
		},
		"prompts/get": {
			"content": "Prompt content",
		},
	}

	// Test cases
	testCases := []struct {
		name           string
		input          string
		expectedOutput string
	}{
		{
			name:           "list tools",
			input:          "tools\n/q\n",
			expectedOutput: "test_tool",
		},
		{
			name:           "list resources",
			input:          "resources\n/q\n",
			expectedOutput: "test_resource",
		},
		{
			name:           "list prompts",
			input:          "prompts\n/q\n",
			expectedOutput: "test_prompt",
		},
		{
			name:           "call tool with params",
			input:          "call test_tool --params {\"foo\":\"bar\"}\n/q\n",
			expectedOutput: "Called test_tool",
		},
		{
			name:           "direct tool call",
			input:          "test_tool {\"foo\":\"bar\"}\n/q\n",
			expectedOutput: "Called test_tool",
		},
		{
			name:           "read resource",
			input:          "resource:test_resource\n/q\n",
			expectedOutput: "Resource content",
		},
		{
			name:           "get prompt",
			input:          "prompt:test_prompt\n/q\n",
			expectedOutput: "Prompt content",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			outBuf := &bytes.Buffer{}

			shell := &Shell{
				Transport: mockServer,
				Format:    "table",
				Reader:    strings.NewReader(tc.input),
				Writer:    outBuf,
			}

			shell.Run()

			output := outBuf.String()
			if !strings.Contains(output, tc.expectedOutput) {
				t.Errorf("Expected output to contain %q, got: %s", tc.expectedOutput, output)
			}
		})
	}
}

func TestWeatherTool(t *testing.T) {
	// Create a mock transport for testing the weather tool
	mockTransport := NewMockTransport()
	mockTransport.Responses["tools/call"] = map[string]interface{}{
		"result": map[string]interface{}{
			"forecast": map[string]interface{}{
				"temperature": 25.5,
				"conditions":  "Sunny",
				"humidity":    60,
				"wind_speed":  10.5,
			},
		},
	}

	outBuf := &bytes.Buffer{}

	shell := &Shell{
		Transport: mockTransport,
		Format:    "table",
		Reader:    strings.NewReader("call weather_get_forecast --params {\"latitude\":37.7749,\"longitude\":-122.4194}\n/q\n"),
		Writer:    outBuf,
	}

	shell.Run()

	output := outBuf.String()
	expectedOutputs := []string{"temperature", "25.5", "Sunny"}
	for _, expected := range expectedOutputs {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain %q, but it doesn't.\nFull output: %s", expected, output)
		}
	}

	// Test direct tool call syntax
	outBuf = &bytes.Buffer{}
	shell = &Shell{
		Transport: mockTransport,
		Format:    "table",
		Reader:    strings.NewReader("weather_get_forecast {\"latitude\":37.7749,\"longitude\":-122.4194}\n/q\n"),
		Writer:    outBuf,
	}

	shell.Run()

	output = outBuf.String()
	for _, expected := range expectedOutputs {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain %q, but it doesn't.\nFull output: %s", expected, output)
		}
	}
}
