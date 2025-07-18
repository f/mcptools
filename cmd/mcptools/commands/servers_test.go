package commands

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestServersCmd(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "mcptools-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test config file
	testConfig := ServersConfig{
		MCPServers: map[string]ServerConfig{
			"test-server": {
				Command: "test-command",
				Args:    []string{"arg1", "arg2"},
			},
			"another-server": {
				Command: "another-command",
				Args:    []string{"arg3"},
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
		enableServers  bool
		expectedOutput string
		expectError    bool
	}{
		{
			name:          "successful server list",
			configPath:    configPath,
			enableServers: true,
			expectedOutput: `another-server: another-command arg3
test-server: test-command arg1 arg2
`,
			expectError: false,
		},
		{
			name:           "servers disabled",
			configPath:     configPath,
			enableServers:  false,
			expectedOutput: `Servers command is not enabled, please create a server config at ` + configPath + "\n",
			expectError:    false,
		},
		{
			name:          "non-existent config file",
			configPath:    filepath.Join(tempDir, "nonexistent.json"),
			enableServers: true,
			expectError:   true,
		},
		{
			name:          "invalid json config",
			configPath:    filepath.Join(tempDir, "invalid.json"),
			enableServers: true,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create invalid JSON file for the invalid json test
			if tt.name == "invalid json config" {
				if err := os.WriteFile(tt.configPath, []byte("invalid json"), 0644); err != nil {
					t.Fatalf("Failed to write invalid config: %v", err)
				}
			}

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

			// Create a command with the test configuration
			cmd := ServersCmd(tt.configPath, tt.enableServers)

			// Execute the command
			err := cmd.Execute()

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
			if got := string(output); got != tt.expectedOutput {
				t.Errorf("Expected output:\n%s\nGot:\n%s", tt.expectedOutput, got)
			}
		})
	}
}
