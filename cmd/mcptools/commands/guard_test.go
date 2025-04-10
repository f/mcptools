package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractPatterns(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		wantAllowTools   []string
		wantAllowPrompts []string
		wantAllowRes     []string
		wantDenyTools    []string
		wantDenyPrompts  []string
		wantDenyRes      []string
		wantCmdArgs      []string
	}{
		{
			name:        "No patterns",
			args:        []string{"npx", "-y", "@modelcontextprotocol/server-filesystem", "~"},
			wantCmdArgs: []string{"npx", "-y", "@modelcontextprotocol/server-filesystem", "~"},
		},
		{
			name:           "Allow tools patterns",
			args:           []string{"--allow", "tools:read_*", "npx", "-y", "@modelcontextprotocol/server-filesystem", "~"},
			wantAllowTools: []string{"read_*"},
			wantCmdArgs:    []string{"npx", "-y", "@modelcontextprotocol/server-filesystem", "~"},
		},
		{
			name:          "Deny tools patterns",
			args:          []string{"--deny", "tools:edit_*,write_*,create_*", "npx", "-y", "@modelcontextprotocol/server-filesystem", "~"},
			wantDenyTools: []string{"edit_*", "write_*", "create_*"},
			wantCmdArgs:   []string{"npx", "-y", "@modelcontextprotocol/server-filesystem", "~"},
		},
		{
			name:           "Both allow and deny patterns",
			args:           []string{"--allow", "tools:read_*", "--deny", "tools:edit_*,write_*,create_*", "npx", "-y", "@modelcontextprotocol/server-filesystem", "~"},
			wantAllowTools: []string{"read_*"},
			wantDenyTools:  []string{"edit_*", "write_*", "create_*"},
			wantCmdArgs:    []string{"npx", "-y", "@modelcontextprotocol/server-filesystem", "~"},
		},
		{
			name:             "All entity types",
			args:             []string{"--allow", "tools:read_*,prompts:system_*,resource:files_*", "--deny", "tools:edit_*", "npx", "-y", "@modelcontextprotocol/server-filesystem", "~"},
			wantAllowTools:   []string{"read_*"},
			wantAllowPrompts: []string{"system_*"},
			wantAllowRes:     []string{"files_*"},
			wantDenyTools:    []string{"edit_*"},
			wantCmdArgs:      []string{"npx", "-y", "@modelcontextprotocol/server-filesystem", "~"},
		},
		{
			name:           "Short flag forms",
			args:           []string{"-a", "tools:read_*", "-d", "tools:edit_*", "npx", "-y", "@modelcontextprotocol/server-filesystem", "~"},
			wantAllowTools: []string{"read_*"},
			wantDenyTools:  []string{"edit_*"},
			wantCmdArgs:    []string{"npx", "-y", "@modelcontextprotocol/server-filesystem", "~"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowPatterns, denyPatterns, cmdArgs := extractPatterns(tt.args)

			// Check tool patterns
			assert.ElementsMatch(t, tt.wantAllowTools, allowPatterns[EntityTypeTool])
			assert.ElementsMatch(t, tt.wantAllowPrompts, allowPatterns[EntityTypePrompt])
			assert.ElementsMatch(t, tt.wantAllowRes, allowPatterns[EntityTypeRes])

			assert.ElementsMatch(t, tt.wantDenyTools, denyPatterns[EntityTypeTool])
			assert.ElementsMatch(t, tt.wantDenyPrompts, denyPatterns[EntityTypePrompt])
			assert.ElementsMatch(t, tt.wantDenyRes, denyPatterns[EntityTypeRes])

			assert.Equal(t, tt.wantCmdArgs, cmdArgs)
		})
	}
}

func TestProcessPatternString(t *testing.T) {
	tests := []struct {
		name       string
		patternStr string
		wantTools  []string
		wantPrompt []string
		wantRes    []string
	}{
		{
			name:       "Single tool pattern",
			patternStr: "tools:read_*",
			wantTools:  []string{"read_*"},
		},
		{
			name:       "Multiple tool patterns",
			patternStr: "tools:read_*,edit_*,write_*",
			wantTools:  []string{"read_*", "edit_*", "write_*"},
		},
		{
			name:       "Different entity types",
			patternStr: "tools:read_*,prompts:system_*,resource:files_*",
			wantTools:  []string{"read_*"},
			wantPrompt: []string{"system_*"},
			wantRes:    []string{"files_*"},
		},
		{
			name:       "No type specified defaults to tool",
			patternStr: "read_*",
			wantTools:  []string{"read_*"},
		},
		{
			name:       "Unknown type treated as tool pattern",
			patternStr: "unknown:pattern",
			wantTools:  []string{"unknown:pattern"},
		},
		{
			name:       "Empty string",
			patternStr: "",
		},
		{
			name:       "Empty patterns are skipped",
			patternStr: "tools:read_*,,prompts:system_*",
			wantTools:  []string{"read_*"},
			wantPrompt: []string{"system_*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patternMap := map[string][]string{
				EntityTypeTool:   {},
				EntityTypePrompt: {},
				EntityTypeRes:    {},
			}

			processPatternString(tt.patternStr, patternMap)

			assert.ElementsMatch(t, tt.wantTools, patternMap[EntityTypeTool])
			assert.ElementsMatch(t, tt.wantPrompt, patternMap[EntityTypePrompt])
			assert.ElementsMatch(t, tt.wantRes, patternMap[EntityTypeRes])
		})
	}
}
