package commands

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const toolsListMethod = "tools/list"

// setupTestCommand creates a command with a buffer for output and simulated stdin.
// It returns the command, output buffer, and a cleanup function.
func setupTestCommand(t *testing.T, input string) (*cobra.Command, *bytes.Buffer, func()) {
	cmd := ShellCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	// Create a pipe to simulate stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}

	// Save original stdin and restore it after test
	oldStdin := os.Stdin
	os.Stdin = r

	// This is just to prevent line.Prompt to print to stdout
	oldStdout := os.Stdout
	os.Stdout = nil

	// Write input to the pipe
	go func() {
		defer func() {
			if err := w.Close(); err != nil {
				t.Errorf("Failed to close pipe writer: %v", err)
			}
		}()
		_, err := w.Write([]byte(input))
		if err != nil {
			t.Errorf("Failed to write to pipe: %v", err)
		}
	}()

	cleanup := func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}

	return cmd, buf, cleanup
}

func TestShellBasicCommands(t *testing.T) {
	tests := []struct { //nolint:govet
		mockResponses   map[string]map[string]interface{}
		name            string
		expectedOutputs []string
		input           string
	}{
		{
			name:            "tools command",
			input:           "tools\n/q\n",
			expectedOutputs: []string{"test-tool", "A test tool"},
			mockResponses: map[string]map[string]any{
				"tools/list": {
					"tools": []any{
						map[string]any{
							"name":        "test-tool",
							"description": "A test tool",
						},
					},
				},
			},
		},
		{
			name:            "prompts command",
			input:           "prompts\n/q\n",
			expectedOutputs: []string{"test-prompt", "Test prompt description"},
			mockResponses: map[string]map[string]any{
				"prompts/list": {
					"prompts": []any{
						map[string]any{
							"name":        "test-prompt",
							"description": "Test prompt description",
						},
					},
				},
			},
		},
		{
			name:            "resources command",
			input:           "resources\n/q\n",
			expectedOutputs: []string{"test_resource", "A test resource"},
			mockResponses: map[string]map[string]any{
				"resources/list": {
					"resources": []any{
						map[string]any{
							"uri":         "test_resource",
							"description": "A test resource",
						},
					},
				},
			},
		},
		{
			name:            "help command",
			input:           "/h\n/q\n",
			expectedOutputs: []string{"MCP Shell Commands:"},
			mockResponses:   map[string]map[string]any{},
		},
		{
			name:            "quit command with /q",
			input:           "/q\n",
			expectedOutputs: []string{"Exiting MCP shell"},
			mockResponses:   map[string]map[string]any{},
		},
		{
			name:            "quit command with exit",
			input:           "exit\n",
			expectedOutputs: []string{"Exiting MCP shell"},
			mockResponses:   map[string]map[string]any{},
		},
		{
			name:            "quit command with /quit",
			input:           "/quit\n",
			expectedOutputs: []string{"Exiting MCP shell"},
			mockResponses:   map[string]map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, buf, cleanupSetup := setupTestCommand(t, tt.input)
			defer cleanupSetup()

			cleanupClient := setupMockClient(func(method string, _ any) (map[string]any, error) {
				mockResponse, ok := tt.mockResponses[method]
				if !ok {
					// Tools list is always called to make sure the server is reachable.
					if method == toolsListMethod {
						return map[string]any{}, nil
					}
					t.Errorf("expected method %q, got %q", method, mockResponse)
				}
				return mockResponse, nil
			})
			defer cleanupClient()

			err := cmd.Execute()
			if err != nil {
				t.Errorf("cmd.Execute() error = %v", err)
			}

			output := buf.String()
			for _, expectedOutput := range tt.expectedOutputs {
				if !strings.Contains(output, expectedOutput) {
					t.Errorf("Expected output to contain %q, got: \n%s", expectedOutput, output)
				}
			}
		})
	}
}

func TestShellCallCommand(t *testing.T) {
	// 1. call a tool with `<tool_name>`
	// 2. call a tool with `call <tool_name>`
	// 3. call a tool with `<tool_name> <params>`
	// 4. call a tool with `call <tool_name> <params>`
	// 5. call a tool with `<tool_name> '<params>'`
	// 6. call a tool with `call <tool_name> '<params>'`
	// 7. call a tool with `<tool_name> --params <params>`
	// 8. call a tool with `call <tool_name> --params <params>`
	// 9. call a tool with `<tool_name> --params '<params>'`
	// 10. call a tool with `call <tool_name> --params '<params>'`
	tests := []struct {
		name            string
		mockResponses   map[string]map[string]any
		expectedParams  map[string]any
		input           string
		expectedOutputs []string
	}{
		{
			name:            "tool_name without params",
			input:           "test-tool\n/q\n",
			expectedOutputs: []string{"Tool executed successfully"},
			expectedParams:  map[string]any{"name": "test-tool", "arguments": map[string]any{}},
			mockResponses: map[string]map[string]any{
				"tools/call": {
					"content": []any{map[string]any{"type": "text", "text": "Tool executed successfully"}},
				},
			},
		},
		{
			name:            "call tool without params",
			input:           "call test-tool\n/q\n",
			expectedOutputs: []string{"Tool executed successfully"},
			expectedParams:  map[string]any{"name": "test-tool", "arguments": map[string]any{}},
			mockResponses: map[string]map[string]any{
				"tools/call": {
					"content": []any{map[string]any{"type": "text", "text": "Tool executed successfully"}},
				},
			},
		},
		{
			name:            "tool_name with direct params",
			input:           "test-tool {\"foo\": \"bar\"}\n/q\n",
			expectedOutputs: []string{"Tool executed successfully"},
			expectedParams:  map[string]any{"name": "test-tool", "arguments": map[string]any{"foo": "bar"}},
			mockResponses: map[string]map[string]any{
				"tools/call": {
					"content": []any{map[string]any{"type": "text", "text": "Tool executed successfully"}},
				},
			},
		},
		{
			name:            "call tool with direct params",
			input:           "call test-tool {\"foo\": \"bar\"}\n/q\n",
			expectedOutputs: []string{"Tool executed successfully"},
			expectedParams:  map[string]any{"name": "test-tool", "arguments": map[string]any{"foo": "bar"}},
			mockResponses: map[string]map[string]any{
				"tools/call": {
					"content": []any{map[string]any{"type": "text", "text": "Tool executed successfully"}},
				},
			},
		},
		{
			name:            "tool_name with quoted direct params",
			input:           "test-tool '{\"foo\": \"bar\"}'\n/q\n",
			expectedOutputs: []string{"Tool executed successfully"},
			expectedParams:  map[string]any{"name": "test-tool", "arguments": map[string]any{"foo": "bar"}},
			mockResponses: map[string]map[string]any{
				"tools/call": {
					"content": []any{map[string]any{"type": "text", "text": "Tool executed successfully"}},
				},
			},
		},
		{
			name:            "call tool with quoted direct params",
			input:           "call test-tool '{\"foo\": \"bar\"}'\n/q\n",
			expectedOutputs: []string{"Tool executed successfully"},
			expectedParams:  map[string]any{"name": "test-tool", "arguments": map[string]any{"foo": "bar"}},
			mockResponses: map[string]map[string]any{
				"tools/call": {
					"content": []any{map[string]any{"type": "text", "text": "Tool executed successfully"}},
				},
			},
		},
		{
			name:            "tool_name with params flag",
			input:           "test-tool --params {\"foo\": \"bar\"}\n/q\n",
			expectedOutputs: []string{"Tool executed successfully"},
			expectedParams:  map[string]any{"name": "test-tool", "arguments": map[string]any{"foo": "bar"}},
			mockResponses: map[string]map[string]any{
				"tools/call": {
					"content": []any{map[string]any{"type": "text", "text": "Tool executed successfully"}},
				},
			},
		},
		{
			name:            "call tool with params flag",
			input:           "call test-tool --params {\"foo\": \"bar\"}\n/q\n",
			expectedOutputs: []string{"Tool executed successfully"},
			expectedParams:  map[string]any{"name": "test-tool", "arguments": map[string]any{"foo": "bar"}},
			mockResponses: map[string]map[string]any{
				"tools/call": {
					"content": []any{map[string]any{"type": "text", "text": "Tool executed successfully"}},
				},
			},
		},
		{
			name:            "tool_name with quoted params flag",
			input:           "test-tool --params '{\"foo\": \"bar\"}'\n/q\n",
			expectedOutputs: []string{"Tool executed successfully"},
			expectedParams:  map[string]any{"name": "test-tool", "arguments": map[string]any{"foo": "bar"}},
			mockResponses: map[string]map[string]any{
				"tools/call": {
					"content": []any{map[string]any{"type": "text", "text": "Tool executed successfully"}},
				},
			},
		},
		{
			name:            "call tool with quoted params flag",
			input:           "call test-tool --params '{\"foo\": \"bar\"}'\n/q\n",
			expectedOutputs: []string{"Tool executed successfully"},
			expectedParams:  map[string]any{"name": "test-tool", "arguments": map[string]any{"foo": "bar"}},
			mockResponses: map[string]map[string]any{
				"tools/call": {
					"content": []any{map[string]any{"type": "text", "text": "Tool executed successfully"}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, buf, cleanupSetup := setupTestCommand(t, tt.input)
			defer cleanupSetup()

			cleanupClient := setupMockClient(func(method string, params any) (map[string]any, error) {
				mockResponse, ok := tt.mockResponses[method]
				if !ok {
					// Tools list is always called to make sure the server is reachable.
					if method == toolsListMethod {
						return map[string]any{}, nil
					}
					t.Errorf("expected method %q, got %q", method, mockResponse)
				}
				jsonParams := ConvertJSONToMap(params)
				if !reflect.DeepEqual(jsonParams, tt.expectedParams) {
					t.Errorf("expected params %v, got %v", tt.expectedParams, jsonParams)
				}
				return mockResponse, nil
			})
			defer cleanupClient()

			err := cmd.Execute()
			if err != nil {
				t.Errorf("cmd.Execute() error = %v", err)
			}

			output := buf.String()
			for _, expectedOutput := range tt.expectedOutputs {
				if !strings.Contains(output, expectedOutput) {
					t.Errorf("Expected output to contain %q, got: %s", expectedOutput, output)
				}
			}
		})
	}
}

func TestShellExit(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"quit with /q", "/q\n"},
		{"quit with /quit", "/quit\n"},
		{"quit with exit", "exit\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, buf, cleanupSetup := setupTestCommand(t, tt.input)
			defer cleanupSetup()

			cleanupClient := setupMockClient(func(method string, _ any) (map[string]any, error) {
				if method != toolsListMethod {
					t.Errorf("Expected method 'tools/list', got %q", method)
				}
				return map[string]any{}, nil
			})
			defer cleanupClient()

			err := cmd.Execute()
			if err != nil {
				t.Errorf("cmd.Execute() error = %v", err)
			}

			output := buf.String()
			if !strings.Contains(output, "Exiting MCP shell") {
				t.Errorf("Expected output to contain 'Exiting MCP shell', got: %s", output)
			}
		})
	}
}

func TestShellQuietFlag(t *testing.T) {
	tests := []struct { //nolint:govet
		name             string
		args             []string
		unexpectedOutput []string
		expectStartup    bool
	}{
		{
			name:          "without quiet flag shows startup messages",
			args:          []string{"echo", "test"},
			expectStartup: true,
		},
		{
			name:             "with --quiet flag suppresses startup messages",
			args:             []string{"--quiet", "echo", "test"},
			expectStartup:    false,
			unexpectedOutput: []string{"MCP Tools Shell", "Connected to Server", "Type '/h' for help"},
		},
		{
			name:             "with -q flag suppresses startup messages",
			args:             []string{"-q", "echo", "test"},
			expectStartup:    false,
			unexpectedOutput: []string{"MCP Tools Shell", "Connected to Server", "Type '/h' for help"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset QuietMode before each test
			QuietMode = false

			cmd, buf, cleanupSetup := setupTestCommand(t, "/q\n")
			defer cleanupSetup()

			cleanupClient := setupMockClient(func(_ string, _ any) (map[string]any, error) {
				return map[string]any{}, nil
			})
			defer cleanupClient()

			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err != nil {
				t.Errorf("cmd.Execute() error = %v", err)
			}

			output := buf.String()
			if tt.expectStartup {
				if !strings.Contains(output, "MCP Tools Shell") {
					t.Errorf("Expected startup messages, got: %s", output)
				}
			} else {
				for _, unexpected := range tt.unexpectedOutput {
					if strings.Contains(output, unexpected) {
						t.Errorf("Expected output to NOT contain %q, got: %s", unexpected, output)
					}
				}
			}
		})
	}
}
