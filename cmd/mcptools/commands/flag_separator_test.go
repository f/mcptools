package commands

import (
	"io"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetFlagOptions(t *testing.T) {
	t.Helper()
	format, transport, logs, authUser, authHeader := FormatOption, TransportOption, ShowServerLogs, AuthUser, AuthHeader
	t.Cleanup(func() {
		FormatOption, TransportOption, ShowServerLogs, AuthUser, AuthHeader = format, transport, logs, authUser, authHeader
	})
	FormatOption, TransportOption, ShowServerLogs, AuthUser, AuthHeader = formatTable, TransportHTTP, false, "", ""
}

func TestProcessFlagsSeparator(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"short format", []string{"--", "server", "-f", "settings.yaml"}, []string{"server", "-f", "settings.yaml"}},
		{"long format", []string{"--", "server", "--format", "yaml"}, []string{"server", "--format", "yaml"}},
		{"transport", []string{"--", "server", "--transport", "stdio"}, []string{"server", "--transport", "stdio"}},
		{"server logs", []string{"--", "server", "--server-logs"}, []string{"server", "--server-logs"}},
		{"auth user", []string{"--", "server", "--auth-user", "server-user-option"}, []string{"server", "--auth-user", "server-user-option"}},
		{"auth header", []string{"--", "server", "--auth-header", "server-header-option"}, []string{"server", "--auth-header", "server-header-option"}},
		{"server separator", []string{"--", "server", "--", "-f", ""}, []string{"server", "--", "-f", ""}},
		{"separator after command", []string{"server", "--", "--format", "yaml"}, []string{"server", "--format", "yaml"}},
		{"trailing separator", []string{"server", "--"}, []string{"server"}},
		{"separator only", []string{"--"}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetFlagOptions(t)
			assert.Equal(t, tt.want, ProcessFlags(tt.args))
			assert.Equal(t, formatTable, FormatOption)
			assert.Equal(t, TransportHTTP, TransportOption)
			assert.False(t, ShowServerLogs)
			assert.Empty(t, AuthUser)
			assert.Empty(t, AuthHeader)
		})
	}
}

func TestListCommandsSeparator(t *testing.T) {
	for _, newCommand := range []func() *cobra.Command{ToolsCmd, ResourcesCmd, PromptsCmd} {
		command := newCommand()
		t.Run(command.Name(), func(t *testing.T) {
			resetFlagOptions(t)
			t.Cleanup(setupMockClient(func(_ string, _ any) (map[string]any, error) {
				return map[string]any{}, nil
			}))
			mockFactory := CreateClientFunc
			var received []string
			CreateClientFunc = func(args []string, options ...client.ClientOption) (*client.Client, error) {
				received = append([]string{}, args...)
				return mockFactory(args, options...)
			}

			root := RootCmd()
			root.AddCommand(command)
			root.SetOut(io.Discard)
			serverArgs := []string{"python3", "server.py", "-f", "settings.yaml", "--transport", "stdio", "--server-logs", "--"}
			root.SetArgs(append([]string{command.Name(), "-f", "json", "--"}, serverArgs...))
			require.NoError(t, root.Execute())
			assert.Equal(t, serverArgs, received)
			assert.Equal(t, "json", FormatOption)
			assert.Equal(t, TransportHTTP, TransportOption)
			assert.False(t, ShowServerLogs)
		})
	}
}

func TestGuardSeparator(t *testing.T) {
	resetFlagOptions(t)
	serverArgs := []string{"server", "--allow", "server-allow", "--deny", "server-deny", "-a", "short-allow", "-d", "short-deny", "-f", "settings.yaml", "--"}
	args := append([]string{"--allow", "tools:read_*", "--deny", "tools:write_*", "-f", "json", "--"}, serverArgs...)
	allow, deny, cmdArgs := extractPatterns(args)
	assert.Equal(t, []string{"read_*"}, allow[EntityTypeTool])
	assert.Equal(t, []string{"write_*"}, deny[EntityTypeTool])
	assert.Equal(t, serverArgs, ProcessFlags(cmdArgs))
	assert.Equal(t, "json", FormatOption)
}
