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

	"github.com/f/mcptools/cmd/mcptools/commands"
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
	Transport *MockTransport
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

	// Test cases
	testCases := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name: "register with script",
			args: []string{
				"add_numbers",
				"Adds two numbers",
				"a:int,b:int",
				filepath.Join(tmpDir, "add.sh"),
			},
			expectError: false,
		},
		{
			name: "register with inline command",
			args: []string{
				"add_op",
				"Adds given numbers",
				"a:int,b:int",
				"-e",
				"echo \"$a + $b = $(($a+$b))\"",
			},
			expectError: false,
		},
		{
			name: "register without script or command",
			args: []string{
				"invalid",
				"Invalid tool",
				"x:int",
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := commands.ProxyToolCmd()
			err := cmd.RunE(cmd, tc.args)

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

			// Verify the tool was registered in the config
			config, err := commands.LoadProxyConfig()
			if err != nil {
				t.Fatalf("Error loading config: %v", err)
			}

			toolName := tc.args[0]
			if _, exists := config[toolName]; !exists {
				t.Errorf("Tool %s was not registered in config", toolName)
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

	// First register a tool
	cmd := commands.ProxyToolCmd()
	err := cmd.RunE(cmd, []string{
		"test_tool",
		"Test tool",
		"x:int",
		"-e",
		"echo $x",
	})
	if err != nil {
		t.Fatalf("Error registering tool: %v", err)
	}

	// Now try to unregister it
	if setErr := cmd.Flags().Set("unregister", "true"); setErr != nil {
		t.Fatalf("Failed to set unregister flag: %v", setErr)
	}
	err = cmd.RunE(cmd, []string{"test_tool"})
	if err != nil {
		t.Errorf("Error unregistering tool: %v", err)
	}

	// Verify the tool was removed from the config
	config, err := commands.LoadProxyConfig()
	if err != nil {
		t.Fatalf("Error loading config: %v", err)
	}

	if _, exists := config["test_tool"]; exists {
		t.Error("Tool was not unregistered from config")
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

func TestMain(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "mcptools-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test config file
	testConfig := commands.ServersConfig{
		MCPServers: map[string]commands.ServerConfig{
			"test-server": {
				Command: "test-command",
				Args:    []string{"arg1", "arg2"},
			},
		},
	}

	configPath := filepath.Join(tempDir, "mcp_servers.json")
	configData, err := json.Marshal(testConfig)
	if err != nil {
		t.Fatalf("Failed to marshal test config: %v", err)
	}

	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	tests := []struct {
		name           string
		configPath     string
		args           []string
		expectedOutput string
		expectError    bool
	}{
		{
			name:       "servers command with valid config",
			configPath: configPath,
			args:       []string{"servers"},
			expectedOutput: `test-server: test-command arg1 arg2
`,
			expectError: false,
		},
		{
			name:           "servers command with non-existent config",
			configPath:     filepath.Join(tempDir, "nonexistent.json"),
			args:           []string{"servers"},
			expectedOutput: `Servers command is not enabled, please create a server config at ` + filepath.Join(tempDir, "nonexistent.json") + "\n",
			expectError:    false,
		},
		{
			name:           "help command shows servers",
			configPath:     configPath,
			args:           []string{"--help"},
			expectedOutput: "servers",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test environment
			os.Setenv("HOME", tempDir)
			defer os.Unsetenv("HOME")

			// Create pipes for capturing output
			oldStdout := os.Stdout
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stdout = w
			os.Stderr = w
			defer func() {
				os.Stdout = oldStdout
				os.Stderr = oldStderr
			}()

			// Create and execute root command
			rootCmd := commands.RootCmd()
			rootCmd.AddCommand(
				commands.VersionCmd(),
				commands.ToolsCmd(),
				commands.ResourcesCmd(),
				commands.PromptsCmd(),
				commands.CallCmd(),
				commands.GetPromptCmd(),
				commands.ReadResourceCmd(),
				commands.ShellCmd(),
				commands.WebCmd(),
				commands.MockCmd(),
				commands.ProxyCmd(),
				commands.AliasCmd(),
				commands.ConfigsCmd(),
				commands.NewCmd(),
				commands.GuardCmd(),
				commands.ServersCmd(tt.configPath, true),
			)

			// Set the command arguments
			rootCmd.SetArgs(tt.args)

			// Execute the command
			err := rootCmd.Execute()

			// Close the write end of the pipe
			w.Close()

			// Read the output
			output, _ := io.ReadAll(r)

			// Check error conditions
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Check output
			if got := string(output); !strings.Contains(got, tt.expectedOutput) {
				t.Errorf("Expected output to contain:\n%s\nGot:\n%s", tt.expectedOutput, got)
			}
		})
	}
}
