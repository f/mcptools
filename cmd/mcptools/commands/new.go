package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// Constants for template options.
const (
	sdkTypeScript  = "ts"
	transportStdio = "stdio"
	transportSSE   = "sse"
	transportHTTP  = "http"
)

// NewCmd returns a new 'new' command for scaffolding MCP projects.
func NewCmd() *cobra.Command {
	var sdkFlag string
	var transportFlag string

	cmd := &cobra.Command{
		Use:   "new [component:name...]",
		Short: "Create a new MCP project component",
		Long: `Create a new MCP component (tool, resource, or prompt) from a template.

Examples:
  mcp new tool:hello_world resource:file prompt:hello
  mcp new tool:hello_world --sdk=ts
  mcp new tool:hello_world --transport=stdio|sse|http`,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("at least one component must be specified (e.g., tool:hello_world)")
			}

			// Validate SDK flag
			if sdkFlag != "" && sdkFlag != sdkTypeScript {
				return fmt.Errorf("unsupported SDK: %s (only ts is currently supported)", sdkFlag)
			}

			// Set default SDK if not specified
			if sdkFlag == "" {
				sdkFlag = sdkTypeScript
			}

			// Validate transport flag
			if transportFlag != "" && transportFlag != transportStdio && transportFlag != transportSSE && transportFlag != transportHTTP {
				return fmt.Errorf("unsupported transport: %s (supported options: stdio, sse, http)", transportFlag)
			}

			// Set default transport if not specified
			if transportFlag == "" {
				transportFlag = transportStdio
			}

			// Parse components from args
			components := make(map[string]string)
			for _, arg := range args {
				parts := strings.SplitN(arg, ":", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid component format: %s (expected format: type:name)", arg)
				}

				componentType := parts[0]
				componentName := parts[1]

				// Validate component type
				switch componentType {
				case "tool", "resource", "prompt": // nolint
					components[componentType] = componentName
				default:
					return fmt.Errorf("unsupported component type: %s (supported types: tool, resource, prompt)", componentType)
				}
			}

			// Create project structure
			return createProjectStructure(components, sdkFlag, transportFlag)
		},
	}

	// Add flags
	cmd.Flags().StringVar(&sdkFlag, "sdk", "", "Specify the SDK to use (ts)")
	cmd.Flags().StringVar(&transportFlag, "transport", "", "Specify the transport to use (stdio, sse, http)")

	return cmd
}

// createProjectStructure creates the project directory and files based on components.
func createProjectStructure(components map[string]string, sdk, transport string) error {
	// Create project directory
	projectDir := "."
	srcDir := filepath.Join(projectDir, "src")

	// Ensure src directory exists
	if err := os.MkdirAll(srcDir, 0o750); err != nil {
		return fmt.Errorf("error creating src directory: %w", err)
	}

	// Look for templates in multiple locations
	templatesDir := findTemplatesDir(sdk)

	if templatesDir == "" {
		return fmt.Errorf("could not find templates directory for SDK: %s", sdk)
	}

	// Copy config files
	if err := copyFile(
		filepath.Join(templatesDir, "package.json"),
		filepath.Join(projectDir, "package.json"),
		map[string]string{"PROJECT_NAME": filepath.Base(projectDir)},
	); err != nil {
		return err
	}

	if err := copyFile(
		filepath.Join(templatesDir, "tsconfig.json"),
		filepath.Join(projectDir, "tsconfig.json"),
		nil,
	); err != nil {
		return err
	}

	// Create index.ts with the server setup
	var serverTemplateFile string
	if transport == transportSSE {
		serverTemplateFile = filepath.Join(templatesDir, "server_sse.ts")
	} else if transport == transportHTTP {
		serverTemplateFile = filepath.Join(templatesDir, "server_http.ts")
	} else {
		// Use stdio by default
		serverTemplateFile = filepath.Join(templatesDir, "server_stdio.ts")
	}

	if err := copyFile(
		serverTemplateFile,
		filepath.Join(srcDir, "index.ts"),
		map[string]string{"PROJECT_NAME": filepath.Base(projectDir)},
	); err != nil {
		return err
	}

	// Create component files
	for componentType, componentName := range components {
		componentFile := filepath.Join(srcDir, componentName+".ts")
		templateFile := filepath.Join(templatesDir, componentType+".ts")

		replacements := map[string]string{
			"TOOL_NAME":        componentName,
			"RESOURCE_NAME":    componentName,
			"PROMPT_NAME":      componentName,
			"TOOL_DESCRIPTION": fmt.Sprintf("The %s tool", componentName),
			"RESOURCE_URI":     fmt.Sprintf("%s://data", componentName),
		}

		if err := copyFile(templateFile, componentFile, replacements); err != nil {
			return err
		}

		// Add import to index.ts
		if err := appendImport(filepath.Join(srcDir, "index.ts"), componentName); err != nil {
			return err
		}
	}

	fmt.Printf("MCP project created successfully with %s SDK and %s transport.\n", sdk, transport)
	fmt.Println("Run the following commands to build and start your MCP server:")
	fmt.Println("npm install")
	fmt.Println("npm run build")
	fmt.Println("npm start")

	return nil
}

// copyFile copies a template file to the destination with replacements.
func copyFile(srcPath, destPath string, replacements map[string]string) error {
	// Read template file

	content, err := os.ReadFile(srcPath) //nolint
	if err != nil {
		return fmt.Errorf("error reading template file %s: %w", srcPath, err)
	}

	// Apply replacements
	fileContent := string(content)
	for key, value := range replacements {
		fileContent = strings.ReplaceAll(fileContent, key, value)
	}

	// Ensure parent directory exists
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0o755); err != nil { //nolint
		return fmt.Errorf("error creating directory %s: %w", destDir, err)
	}

	// Write the processed content to the destination file
	if err := os.WriteFile(destPath, []byte(fileContent), 0o600); err != nil { //nolint
		return fmt.Errorf("error writing to %s: %w", destPath, err)
	}

	return nil
}

// appendImport appends an import statement for the component to the main index.ts file.
func appendImport(indexFile, componentName string) error {
	// Read the index file
	content, err := os.ReadFile(indexFile) //nolint
	if err != nil {
		return fmt.Errorf("error reading index file: %w", err)
	}

	// Check if import is already present
	importRegex := regexp.MustCompile(fmt.Sprintf(`import\s+.*\s+from\s+['"]\.\/%s['"]`, componentName))
	if importRegex.Match(content) {
		// Import already exists, no need to add it
		return nil
	}

	// Add import statement after the last import
	fileContent := string(content)
	importPattern := regexp.MustCompile(`^import.*$`)
	lastImportIndex := 0

	for _, line := range strings.Split(fileContent, "\n") {
		if importPattern.MatchString(line) {
			lastImportIndex += len(line) + 1 // +1 for newline character
		}
	}

	// Insert the new import after the last import
	importStatement := fmt.Sprintf("import %s from \"./%s.js\";\n", componentName, componentName)
	updatedContent := fileContent[:lastImportIndex] + importStatement + fileContent[lastImportIndex:]

	// Find the position to insert component initialization
	transportPattern := regexp.MustCompile(`(?m)^const\s+transport\s*=`)
	match := transportPattern.FindStringIndex(updatedContent)

	var finalContent string
	if match != nil {
		// Insert component initialization before the transport line
		componentInit := fmt.Sprintf("// Initialize the %s component\n%s(server);\n\n", componentName, componentName)
		finalContent = updatedContent[:match[0]] + componentInit + updatedContent[match[0]:]
	} else {
		// Fallback: append to the end of the file if transport line not found
		fileEnd := len(updatedContent)
		for fileEnd > 0 && (updatedContent[fileEnd-1] == '\n' || updatedContent[fileEnd-1] == '\r') {
			fileEnd--
		}
		componentInit := fmt.Sprintf("\n\n// Initialize the %s component\n%s(server);\n", componentName, componentName)
		finalContent = updatedContent[:fileEnd] + componentInit
	}

	// Write the updated content back to the file
	if err := os.WriteFile(indexFile, []byte(finalContent), 0o644); err != nil { // nolint
		return fmt.Errorf("error writing updated index file: %w", err)
	}

	return nil
}

// findTemplatesDir searches for templates in multiple standard locations.
func findTemplatesDir(sdk string) string {
	// Check locations in order of preference
	possibleLocations := []string{
		// Local directory
		filepath.Join("templates", sdk),

		// User home directory
		filepath.Join(getHomeDirectory(), ".mcpt", "templates", sdk),

		// TemplatesPath from env
		filepath.Join(TemplatesPath, sdk),

		// Executable directory
		func() string {
			execPath, err := os.Executable()
			if err != nil {
				return ""
			}
			return filepath.Join(filepath.Dir(execPath), "..", "templates", sdk)
		}(),
	}

	for _, location := range possibleLocations {
		if location == "" {
			continue
		}

		// Check if directory exists
		if stat, err := os.Stat(location); err == nil && stat.IsDir() {
			return location
		}
	}

	return ""
}
