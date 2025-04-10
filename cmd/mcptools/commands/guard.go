package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/f/mcptools/pkg/guard"
	"github.com/spf13/cobra"
)

// Guard flags.
const (
	FlagAllow      = "--allow"
	FlagAllowShort = "-a"
	FlagDeny       = "--deny"
	FlagDenyShort  = "-d"
)

var entityTypes = []string{
	EntityTypeTool,
	EntityTypePrompt,
	EntityTypeRes,
}

// GuardCmd creates the guard command to filter tools, prompts, and resources.
func GuardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "guard [--allow type:pattern] [--deny type:pattern] command args...",
		Short: "Filter tools, prompts, and resources using allow and deny patterns",
		Long: `Filter tools, prompts, and resources using allow and deny patterns.

Examples:
  mcp guard --allow tools:read_* --deny edit_*,write_*,create_* npx run @modelcontextprotocol/server-filesystem ~
  mcp guard --allow prompts:system_* --deny tools:execute_* npx run @modelcontextprotocol/server-filesystem ~
  
Patterns can include wildcards:
  * matches any sequence of characters
  
Entity types:
  tools: filter available tools
  prompts: filter available prompts
  resource: filter available resources`,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		Run: func(thisCmd *cobra.Command, args []string) {
			if len(args) == 1 && (args[0] == FlagHelp || args[0] == FlagHelpShort) {
				_ = thisCmd.Help()
				return
			}

			// Process and extract the allow and deny patterns
			allowPatterns, denyPatterns, cmdArgs := extractPatterns(args)

			// Process regular flags (format)
			parsedArgs := ProcessFlags(cmdArgs)

			// Print filtering info
			fmt.Fprintf(os.Stderr, "Guard filtering configuration:\n")
			for _, entityType := range entityTypes {
				if len(allowPatterns[entityType]) > 0 {
					fmt.Fprintf(os.Stderr, "Allowing %s matching: %s\n", entityType, strings.Join(allowPatterns[entityType], ", "))
				}
				if len(denyPatterns[entityType]) > 0 {
					fmt.Fprintf(os.Stderr, "Denying %s matching: %s\n", entityType, strings.Join(denyPatterns[entityType], ", "))
				}
			}

			// Verify we have a command to run
			if len(parsedArgs) == 0 {
				fmt.Fprintf(os.Stderr, "Error: command to execute is required\n")
				fmt.Fprintf(os.Stderr, "Example: mcp guard --allow tools:read_* npx -y @modelcontextprotocol/server-filesystem ~\n")
				os.Exit(1)
			}

			// Map our entity types to the guard proxy entity types
			guardAllowPatterns := map[string][]string{
				"tool":     allowPatterns[EntityTypeTool],
				"prompt":   allowPatterns[EntityTypePrompt],
				"resource": allowPatterns[EntityTypeRes],
			}
			guardDenyPatterns := map[string][]string{
				"tool":     denyPatterns[EntityTypeTool],
				"prompt":   denyPatterns[EntityTypePrompt],
				"resource": denyPatterns[EntityTypeRes],
			}

			// Run the guard proxy with the filtered environment
			fmt.Fprintf(os.Stderr, "Running command with filtered environment: %s\n", strings.Join(parsedArgs, " "))
			if err := guard.RunFilterServer(guardAllowPatterns, guardDenyPatterns, parsedArgs); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}
}

// extractPatterns processes arguments to extract allow and deny patterns.
func extractPatterns(args []string) (map[string][]string, map[string][]string, []string) {
	allowPatterns := make(map[string][]string)
	denyPatterns := make(map[string][]string)

	// Initialize maps for all entity types
	for _, entityType := range entityTypes {
		allowPatterns[entityType] = []string{}
		denyPatterns[entityType] = []string{}
	}

	cmdArgs := []string{}
	i := 0
	for i < len(args) {
		switch {
		case (args[i] == FlagAllow || args[i] == FlagAllowShort) && i+1 < len(args):
			// Process --allow flag
			patternsStr := args[i+1]
			processPatternString(patternsStr, allowPatterns)
			i += 2
		case (args[i] == FlagDeny || args[i] == FlagDenyShort) && i+1 < len(args):
			// Process --deny flag
			patternsStr := args[i+1]
			processPatternString(patternsStr, denyPatterns)
			i += 2
		default:
			// Not a flag we recognize, pass it along
			cmdArgs = append(cmdArgs, args[i])
			i++
		}
	}

	return allowPatterns, denyPatterns, cmdArgs
}

// processPatternString processes a comma-separated pattern string.
func processPatternString(patternsStr string, patternMap map[string][]string) {
	patterns := strings.Split(patternsStr, ",")

	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		parts := strings.SplitN(pattern, ":", 2)
		if len(parts) != 2 {
			// If no type specified, assume it's a tool pattern
			patternMap[EntityTypeTool] = append(patternMap[EntityTypeTool], pattern)
			continue
		}

		entityType := strings.ToLower(parts[0])
		patternValue := parts[1]

		// Map entity type to known types
		switch entityType {
		case "tool", "tools":
			patternMap[EntityTypeTool] = append(patternMap[EntityTypeTool], patternValue)
		case "prompt", "prompts":
			patternMap[EntityTypePrompt] = append(patternMap[EntityTypePrompt], patternValue)
		case "resource", "resources", "res":
			patternMap[EntityTypeRes] = append(patternMap[EntityTypeRes], patternValue)
		default:
			// Unknown entity type, treat as tool pattern
			patternMap[EntityTypeTool] = append(patternMap[EntityTypeTool], pattern)
		}
	}
}
