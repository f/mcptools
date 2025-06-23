package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"
)

// WebCmd creates the web command.
func WebCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "web [command args...]",
		Short:              "Start a web interface for MCP commands",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		Run: func(thisCmd *cobra.Command, args []string) {
			if len(args) == 1 && (args[0] == FlagHelp || args[0] == FlagHelpShort) {
				_ = thisCmd.Help()
				return
			}

			cmdArgs := args
			parsedArgs := []string{}
			port := "41999" // Default port

			for i := 0; i < len(cmdArgs); i++ {
				switch {
				case (cmdArgs[i] == "--port" || cmdArgs[i] == "-p") && i+1 < len(cmdArgs):
					port = cmdArgs[i+1]
					i++
				case cmdArgs[i] == FlagServerLogs:
					ShowServerLogs = true
				default:
					parsedArgs = append(parsedArgs, cmdArgs[i])
				}
			}

			if len(parsedArgs) == 0 {
				fmt.Fprintln(os.Stderr, "Error: command to execute is required when using the web interface")
				fmt.Fprintln(os.Stderr, "Example: mcp web npx -y @modelcontextprotocol/server-filesystem ~")
				os.Exit(1)
			}

			mcpClient, clientErr := CreateClientFunc(parsedArgs)
			if clientErr != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", clientErr)
				os.Exit(1)
			}

			fmt.Fprintf(thisCmd.OutOrStdout(), "mcp > Starting MCP Tools Web Interface (%s)\n", Version)
			fmt.Fprintf(thisCmd.OutOrStdout(), "mcp > Connected to Server: %s\n", strings.Join(parsedArgs, " "))
			fmt.Fprintf(thisCmd.OutOrStdout(), "mcp > Web server running at http://localhost:%s\n", port)

			// Web server handler
			mux := http.NewServeMux()

			// Create a client cache that can be safely shared across goroutines
			clientCache := &MCPClientCache{
				client: mcpClient,
				mutex:  &sync.Mutex{},
			}

			// Serve static files
			mux.HandleFunc("/", handleIndex())
			mux.HandleFunc("/api/tools", handleTools(clientCache))
			mux.HandleFunc("/api/resources", handleResources(clientCache))
			mux.HandleFunc("/api/prompts", handlePrompts(clientCache))
			mux.HandleFunc("/api/call", handleCall(clientCache))

			// Start the server
			//nolint:gosec // Timeouts not implemented for this development/internal tool
			err := http.ListenAndServe(":"+port, mux)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error starting web server: %v\n", err)
				os.Exit(1)
			}
		},
	}
}

// MCPClientCache provides thread-safe access to the MCP client.
type MCPClientCache struct {
	client *client.Client
	mutex  *sync.Mutex
}

// handleIndex serves the main web interface.
func handleIndex() http.HandlerFunc {
	//nolint:revive // Parameter r is required by http.HandlerFunc signature
	return func(w http.ResponseWriter, r *http.Request) {
		// For simplicity, we'll embed a basic HTML page directly
		// In a production app, we'd use proper templates and static files
		html := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>MCP Tools</title>
    <!-- Add Tailwind CSS CDN -->
    <script src="https://cdn.tailwindcss.com"></script>
    <style>
        /* Custom styles that aren't easily handled by Tailwind */
        .hidden {
            display: none;
        }

        /* Only preserve critical styles that can't be easily done with Tailwind */
        #raw-output-container {
            white-space: pre;
        }
    </style>
</head>
<body class="h-screen flex bg-gray-50 text-gray-900 antialiased">
    <div id="sidebar" class="w-64 bg-white border-r border-gray-200 p-4 overflow-y-auto">
        <h1 class="text-xl font-semibold text-gray-800">MCP Tools</h1>

        <h2 class="mt-6 mb-2 text-sm font-medium text-gray-600 uppercase tracking-wider">Tools</h2>
        <ul id="tools-list" class="space-y-1"></ul>

        <h2 class="mt-6 mb-2 text-sm font-medium text-gray-600 uppercase tracking-wider">Resources</h2>
        <ul id="resources-list" class="space-y-1"></ul>

        <h2 class="mt-6 mb-2 text-sm font-medium text-gray-600 uppercase tracking-wider">Prompts</h2>
        <ul id="prompts-list" class="space-y-1"></ul>
    </div>

    <div id="main" class="flex-1 p-6 overflow-y-auto">
        <h1 id="main-title" class="text-2xl font-bold text-gray-800 mb-4">Select an item from the sidebar</h1>
        <p id="tool-description" class="hidden text-gray-600 mb-6"></p>

        <div id="tool-panel" class="hidden bg-white border border-gray-200 rounded-lg shadow-sm p-6 mb-6">
            <h2 class="text-lg font-medium text-gray-700 mb-4">Parameters:</h2>

            <div class="tab-container flex border-b border-gray-200 mb-4">
                <div class="tab active px-4 py-2 border-t border-l border-r border-gray-200 rounded-t-md bg-white text-blue-600 font-medium" id="form-tab">Form</div>
                <div class="tab px-4 py-2 border-t border-l border-r border-gray-200 rounded-t-md bg-gray-50 text-gray-500" id="json-tab">JSON</div>
            </div>

            <div id="form-container" class="mb-4"></div>

            <div id="json-editor-container" class="hidden mb-4">
                <textarea id="params-area" class="w-full min-h-[100px] p-3 border border-gray-300 rounded-md font-mono">{}</textarea>
            </div>

            <button id="execute-btn" class="px-4 py-2 bg-blue-600 text-white font-medium rounded-md hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50">Execute</button>
        </div>

        <div id="result" class="mt-6">
            <div class="tab-container flex border-b border-gray-200 mb-4">
                <div class="tab active px-4 py-2 border-t border-l border-r border-gray-200 rounded-t-md bg-white text-blue-600 font-medium" id="formatted-tab">Formatted</div>
                <div class="tab px-4 py-2 border-t border-l border-r border-gray-200 rounded-t-md bg-gray-50 text-gray-500" id="raw-tab">Raw JSON</div>
            </div>

            <div id="formatted-output-container" class="bg-white border border-gray-200 rounded-lg p-4"></div>
            <pre id="raw-output-container" class="hidden bg-gray-800 text-gray-100 p-4 rounded-lg overflow-x-auto font-mono text-sm"></pre>
        </div>
    </div>

    <script>
        // Fetch and display tools
        fetch('/api/tools')
            .then(response => response.json())
            .then(data => {
                const toolsList = document.getElementById('tools-list');
                if (data.result && data.result.tools) {
                    data.result.tools.forEach(tool => {
                        const li = document.createElement('li');
                        li.className = 'py-2 px-3 cursor-pointer text-blue-600 hover:bg-blue-50 rounded-md transition-colors duration-150';
                        li.textContent = tool.name;
                        li.onclick = () => showTool(tool);
                        toolsList.appendChild(li);
                    });
                }

                // Ensure formatted tab is visible by default
                document.getElementById('formatted-output-container').classList.remove('hidden');
                document.getElementById('raw-output-container').classList.add('hidden');
            })
            .catch(err => console.error('Error fetching tools:', err));

        // Fetch and display resources
        fetch('/api/resources')
            .then(response => response.json())
            .then(data => {
                const resourcesList = document.getElementById('resources-list');
                if (data.result && data.result.resources) {
                    data.result.resources.forEach(resource => {
                        const li = document.createElement('li');
                        li.className = 'py-2 px-3 cursor-pointer text-green-600 hover:bg-green-50 rounded-md transition-colors duration-150';
                        li.textContent = resource.uri;
                        li.onclick = () => callResource(resource.uri);
                        resourcesList.appendChild(li);
                    });
                }
            })
            .catch(err => console.error('Error fetching resources:', err));

        // Fetch and display prompts
        fetch('/api/prompts')
            .then(response => response.json())
            .then(data => {
                const promptsList = document.getElementById('prompts-list');
                if (data.result && data.result.prompts) {
                    data.result.prompts.forEach(prompt => {
                        const li = document.createElement('li');
                        li.className = 'py-2 px-3 cursor-pointer text-orange-600 hover:bg-orange-50 rounded-md transition-colors duration-150';
                        li.textContent = prompt.name;
                        li.onclick = () => callPrompt(prompt.name);
                        promptsList.appendChild(li);
                    });
                }
            })
            .catch(err => console.error('Error fetching prompts:', err));

        // Tab switching functionality
        document.getElementById('form-tab').addEventListener('click', () => {
            // First update the JSON to match any form changes
            updateJSONFromForm();

            // Then switch to form view
            document.getElementById('form-tab').classList.add('active');
            document.getElementById('form-tab').classList.remove('bg-gray-50');
            document.getElementById('form-tab').classList.add('bg-white', 'text-blue-600');

            document.getElementById('json-tab').classList.remove('active');
            document.getElementById('json-tab').classList.remove('bg-white', 'text-blue-600');
            document.getElementById('json-tab').classList.add('bg-gray-50', 'text-gray-500');

            document.getElementById('form-container').classList.remove('hidden');
            document.getElementById('json-editor-container').classList.add('hidden');
        });

        document.getElementById('json-tab').addEventListener('click', () => {
            // First update the form to match any JSON changes
            updateFormFromJSON();

            // Then switch to JSON view
            document.getElementById('json-tab').classList.add('active');
            document.getElementById('json-tab').classList.remove('bg-gray-50');
            document.getElementById('json-tab').classList.add('bg-white', 'text-blue-600');

            document.getElementById('form-tab').classList.remove('active');
            document.getElementById('form-tab').classList.remove('bg-white', 'text-blue-600');
            document.getElementById('form-tab').classList.add('bg-gray-50', 'text-gray-500');

            document.getElementById('json-editor-container').classList.remove('hidden');
            document.getElementById('form-container').classList.add('hidden');
        });

        document.getElementById('formatted-tab').addEventListener('click', () => {
            document.getElementById('formatted-tab').classList.add('active');
            document.getElementById('formatted-tab').classList.remove('bg-gray-50');
            document.getElementById('formatted-tab').classList.add('bg-white', 'text-blue-600');

            document.getElementById('raw-tab').classList.remove('active');
            document.getElementById('raw-tab').classList.remove('bg-white', 'text-blue-600');
            document.getElementById('raw-tab').classList.add('bg-gray-50', 'text-gray-500');

            document.getElementById('formatted-output-container').classList.remove('hidden');
            document.getElementById('raw-output-container').classList.add('hidden');
        });

        document.getElementById('raw-tab').addEventListener('click', () => {
            document.getElementById('raw-tab').classList.add('active');
            document.getElementById('raw-tab').classList.remove('bg-gray-50');
            document.getElementById('raw-tab').classList.add('bg-white', 'text-blue-600');

            document.getElementById('formatted-tab').classList.remove('active');
            document.getElementById('formatted-tab').classList.remove('bg-white', 'text-blue-600');
            document.getElementById('formatted-tab').classList.add('bg-gray-50', 'text-gray-500');

            document.getElementById('raw-output-container').classList.remove('hidden');
            document.getElementById('formatted-output-container').classList.add('hidden');
        });

        // Add live update to JSON editor with debounce
        let jsonUpdateTimeout = null;
        document.getElementById('params-area').addEventListener('input', () => {
            clearTimeout(jsonUpdateTimeout);

            // Use debounce to avoid excessive updates during typing
            jsonUpdateTimeout = setTimeout(() => {
                try {
                    // First validate the JSON syntax
                    JSON.parse(document.getElementById('params-area').value);
                    // Then update the form if valid
                    updateFormFromJSON();
                } catch (e) {
                    // Don't update if JSON is invalid
                    console.error('Invalid JSON:', e);
                }
            }, 500); // Wait 500ms after typing stops
        });

        // Current tool being edited
        let currentTool = null;

        // Show tool details
        function showTool(tool) {
            currentTool = tool;
            document.getElementById('main-title').textContent = tool.name;

            // Set and show description
            const descriptionElement = document.getElementById('tool-description');
            if (tool.description) {
                descriptionElement.textContent = tool.description;
                descriptionElement.classList.remove('hidden');
            } else {
                descriptionElement.classList.add('hidden');
            }

            document.getElementById('tool-panel').classList.remove('hidden');

            // Create form based on schema
            createFormFromSchema(tool);

            // Set default JSON parameters
            let defaultParams = {};
            if (tool.parameters && tool.parameters.properties) {
                Object.keys(tool.parameters.properties).forEach(key => {
                    defaultParams[key] = "";
                });
            } else if (tool.inputSchema && tool.inputSchema.properties) {
                Object.keys(tool.inputSchema.properties).forEach(key => {
                    defaultParams[key] = "";
                });
            }
            document.getElementById('params-area').value = JSON.stringify(defaultParams, null, 2);

            // Display initial information about the tool
            displayFormattedOutput({ tool: tool });
            document.getElementById('raw-output-container').textContent = JSON.stringify(tool, null, 2);

            // Set up execute button
            document.getElementById('execute-btn').onclick = () => {
                let params = {};

                // Check if we're using the form or JSON editor
                if (document.getElementById('form-container').classList.contains('hidden')) {
                    // Using JSON editor
                    try {
                        params = JSON.parse(document.getElementById('params-area').value);
                    } catch (e) {
                        alert('Error parsing JSON parameters: ' + e.message);
                        return;
                    }
                } else {
                    // Using form - collect values and update JSON view
                    params = collectFormValues(tool);
                    document.getElementById('params-area').value = JSON.stringify(params, null, 2);
                }

                callTool(tool.name, params);
            };
        }

        // Update JSON editor with values from form
        function updateJSONFromForm() {
            if (!currentTool) return;

            const params = collectFormValues(currentTool);
            document.getElementById('params-area').value = JSON.stringify(params, null, 2);
        }

        // Update form with values from JSON editor
        function updateFormFromJSON() {
            if (!currentTool) return;

            try {
                const params = JSON.parse(document.getElementById('params-area').value);
                populateFormFromJSON(params);
            } catch (e) {
                alert('Error parsing JSON: ' + e.message);
            }
        }

        // Populate form fields from JSON data
        function populateFormFromJSON(jsonData) {
            if (!currentTool || !jsonData) return;

            // Get schema
            let schema = null;
            if (currentTool.parameters && currentTool.parameters.properties) {
                schema = currentTool.parameters;
            } else if (currentTool.inputSchema && currentTool.inputSchema.properties) {
                schema = currentTool.inputSchema;
            }

            if (!schema) return;

            const properties = schema.properties;

            for (const propName in properties) {
                const prop = properties[propName];
                const value = jsonData[propName];

                if (value === undefined) continue;

                // Handle array of objects separately
                if (prop.type === 'array' && prop.items && prop.items.type === 'object' && prop.items.properties) {
                    const arrayContainer = document.getElementById('array-container-' + propName);
                    if (!arrayContainer) continue;

                    // Clear existing items
                    arrayContainer.innerHTML = '';

                    // Add new items based on the JSON data
                    if (Array.isArray(value)) {
                        value.forEach((itemData, index) => {
                            // Add new item to the DOM
                            addArrayItem(propName, prop.items);

                            // Set values for each field
                            for (const fieldName in prop.items.properties) {
                                if (itemData[fieldName] !== undefined) {
                                    const input = document.getElementById('param-' + propName + '-' + index + '-' + fieldName);
                                    if (input) {
                                        if (typeof itemData[fieldName] === 'boolean') {
                                            input.value = itemData[fieldName].toString();
                                        } else {
                                            input.value = itemData[fieldName];
                                        }
                                    }
                                }
                            }
                        });
                    }
                } else {
                    // Handle regular inputs
                    const input = document.getElementById('param-' + propName);
                    if (!input) continue;

                    if (prop.type === 'array' && Array.isArray(value)) {
                        // For textarea arrays, join with newlines
                        input.value = value.join('\n');
                    } else if (prop.type === 'object' && typeof value === 'object') {
                        // For object inputs, stringify the JSON
                        input.value = JSON.stringify(value, null, 2);
                    } else if (typeof value === 'boolean') {
                        // For boolean selects, convert to string
                        input.value = value.toString();
                    } else {
                        // For all other types
                        input.value = value;
                    }
                }
            }
        }

        // Create form inputs based on schema
        function createFormFromSchema(tool) {
            const formContainer = document.getElementById('form-container');
            formContainer.innerHTML = '';

            // Check for schema in either parameters or inputSchema
            let schema = null;
            if (tool.parameters && tool.parameters.properties) {
                schema = tool.parameters;
            } else if (tool.inputSchema && tool.inputSchema.properties) {
                schema = tool.inputSchema;
            }

            if (!schema) {
                formContainer.innerHTML = '<p class="text-gray-500 italic">No parameters required for this tool.</p>';
                return;
            }

            const properties = schema.properties;
            const required = schema.required || [];

            for (const propName in properties) {
                const prop = properties[propName];
                const formGroup = document.createElement('div');
                formGroup.className = 'mb-6';
                formGroup.dataset.propName = propName;

                // Create label
                const label = document.createElement('label');
                label.htmlFor = 'param-' + propName;
                label.textContent = propName;
                label.className = 'block text-sm font-medium text-gray-700 mb-1';

                if (required.includes(propName)) {
                    const requiredSpan = document.createElement('span');
                    requiredSpan.className = 'text-red-600 font-bold';
                    requiredSpan.textContent = ' *';
                    label.appendChild(requiredSpan);
                }
                formGroup.appendChild(label);

                // Add description if available
                if (prop.description) {
                    const description = document.createElement('div');
                    description.className = 'text-xs text-gray-500 mb-2';
                    description.textContent = prop.description;
                    formGroup.appendChild(description);
                }

                // Handle different types of inputs
                if (prop.type === 'array') {
                    // Create array container
                    const arrayContainer = document.createElement('div');
                    arrayContainer.className = 'mb-4';
                    arrayContainer.id = 'array-container-' + propName;
                    formGroup.appendChild(arrayContainer);

                    // Check if this is an array of objects with schema defined
                    const isObjectArray = prop.items && prop.items.type === 'object' && prop.items.properties;

                    if (isObjectArray) {
                        // Store the item schema for later use when adding new items
                        arrayContainer.dataset.itemSchema = JSON.stringify(prop.items);

                        // Add button for adding new items
                        const addButton = document.createElement('button');
                        addButton.type = 'button';
                        addButton.className = 'flex items-center px-3 py-2 mt-2 bg-green-600 text-white rounded-md hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-green-500';
                        addButton.innerHTML = '<svg class="w-5 h-5 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path></svg> Add Item';
                        addButton.onclick = () => {
                            addArrayItem(propName, prop.items);
                            updateJSONFromForm(); // Update JSON when adding items
                        };

                        const arrayActions = document.createElement('div');
                        arrayActions.className = 'mt-2';
                        arrayActions.appendChild(addButton);
                        formGroup.appendChild(arrayActions);

                        // Make sure to append the formGroup to the DOM before adding items
                        formContainer.appendChild(formGroup);

                        // Add initial empty item
                        addArrayItem(propName, prop.items);
                    } else {
                        // Simple array - use textarea with one item per line
                        const textarea = document.createElement('textarea');
                        textarea.id = 'param-' + propName;
                        textarea.name = propName;
                        textarea.className = 'w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 font-mono';
                        textarea.placeholder = 'Enter one item per line';
                        textarea.rows = 4;

                        // Add event listener to update JSON when textarea changes
                        textarea.addEventListener('input', () => updateJSONFromForm());

                        formGroup.appendChild(textarea);

                        formContainer.appendChild(formGroup);
                    }
                } else {
                    // Create input based on type (non-array)
                    let input;
                    switch (prop.type) {
                        case 'boolean':
                            input = document.createElement('select');
                            input.className = 'w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500';

                            const trueOption = document.createElement('option');
                            trueOption.value = 'true';
                            trueOption.textContent = 'true';

                            const falseOption = document.createElement('option');
                            falseOption.value = 'false';
                            falseOption.textContent = 'false';

                            input.appendChild(trueOption);
                            input.appendChild(falseOption);
                            break;

                        case 'number':
                        case 'integer':
                            input = document.createElement('input');
                            input.type = 'number';
                            input.className = 'w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500';
                            if (prop.minimum !== undefined) input.min = prop.minimum;
                            if (prop.maximum !== undefined) input.max = prop.maximum;
                            break;

                        case 'object':
                            input = document.createElement('textarea');
                            input.placeholder = 'Enter JSON object';
                            input.className = 'w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 font-mono';
                            input.rows = 4;
                            break;

                        default: // string or any other type
                            if (prop.enum) {
                                input = document.createElement('select');
                                input.className = 'w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500';

                                prop.enum.forEach(option => {
                                    const optionEl = document.createElement('option');
                                    optionEl.value = option;
                                    optionEl.textContent = option;
                                    input.appendChild(optionEl);
                                });
                            } else {
                                input = document.createElement('input');
                                input.type = 'text';
                                input.className = 'w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500';
                                if (prop.format === 'password') input.type = 'password';
                            }
                    }

                    input.id = 'param-' + propName;
                    input.name = propName;

                    // Add event listener to update JSON when input changes
                    input.addEventListener('input', () => updateJSONFromForm());
                    if (input.tagName === 'SELECT') {
                        input.addEventListener('change', () => updateJSONFromForm());
                    }

                    formGroup.appendChild(input);
                    formContainer.appendChild(formGroup);
                }
            }
        }

        // Add a new item to an array
        function addArrayItem(propName, itemSchema) {
            const container = document.getElementById('array-container-' + propName);
            const itemIndex = container.children.length;

            // Create item container
            const itemDiv = document.createElement('div');
            itemDiv.className = 'relative p-4 mb-4 bg-gray-50 border border-gray-200 rounded-lg';
            itemDiv.dataset.index = itemIndex;

            // Add remove button
            const removeButton = document.createElement('button');
            removeButton.type = 'button';
            removeButton.className = 'absolute top-2 right-2 p-1 w-6 h-6 flex items-center justify-center text-white bg-red-500 rounded-full hover:bg-red-600 focus:outline-none focus:ring-2 focus:ring-red-400';
            removeButton.textContent = '×';
            removeButton.onclick = () => {
                itemDiv.remove();
                // Update indices for remaining items
                updateArrayItemIndices(propName);
                // Update JSON when removing items
                updateJSONFromForm();
            };
            itemDiv.appendChild(removeButton);

            // Create form fields based on the item schema
            if (itemSchema && itemSchema.properties) {
                for (const fieldName in itemSchema.properties) {
                    const fieldProp = itemSchema.properties[fieldName];
                    const fieldGroup = document.createElement('div');
                    fieldGroup.className = 'mb-4';

                    // Label
                    const label = document.createElement('label');
                    label.htmlFor = 'param-' + propName + '-' + itemIndex + '-' + fieldName;
                    label.textContent = fieldName;
                    label.className = 'block text-sm font-medium text-gray-700 mb-1';

                    if (itemSchema.required && itemSchema.required.includes(fieldName)) {
                        const requiredSpan = document.createElement('span');
                        requiredSpan.className = 'text-red-600 font-bold';
                        requiredSpan.textContent = ' *';
                        label.appendChild(requiredSpan);
                    }
                    fieldGroup.appendChild(label);

                    // Description
                    if (fieldProp.description) {
                        const description = document.createElement('div');
                        description.className = 'text-xs text-gray-500 mb-1';
                        description.textContent = fieldProp.description;
                        fieldGroup.appendChild(description);
                    }

                    // Input
                    let input;
                    switch (fieldProp.type) {
                        case 'boolean':
                            input = document.createElement('select');
                            input.className = 'w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500';

                            const trueOption = document.createElement('option');
                            trueOption.value = 'true';
                            trueOption.textContent = 'true';

                            const falseOption = document.createElement('option');
                            falseOption.value = 'false';
                            falseOption.textContent = 'false';

                            input.appendChild(trueOption);
                            input.appendChild(falseOption);
                            break;

                        case 'number':
                        case 'integer':
                            input = document.createElement('input');
                            input.type = 'number';
                            input.className = 'w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500';
                            if (fieldProp.minimum !== undefined) input.min = fieldProp.minimum;
                            if (fieldProp.maximum !== undefined) input.max = fieldProp.maximum;
                            break;

                        default: // string, object, or any other type
                            if (fieldProp.enum) {
                                input = document.createElement('select');
                                input.className = 'w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500';

                                fieldProp.enum.forEach(option => {
                                    const optionEl = document.createElement('option');
                                    optionEl.value = option;
                                    optionEl.textContent = option;
                                    input.appendChild(optionEl);
                                });
                            } else {
                                input = document.createElement('input');
                                input.type = 'text';
                                input.className = 'w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500';
                                if (fieldProp.format === 'password') input.type = 'password';
                            }
                    }

                    input.id = 'param-' + propName + '-' + itemIndex + '-' + fieldName;
                    input.name = propName + '-' + itemIndex + '-' + fieldName;
                    input.dataset.field = fieldName;

                    // Add event listener to update JSON when item field changes
                    input.addEventListener('input', () => updateJSONFromForm());
                    if (input.tagName === 'SELECT') {
                        input.addEventListener('change', () => updateJSONFromForm());
                    }

                    fieldGroup.appendChild(input);
                    itemDiv.appendChild(fieldGroup);
                }
            }

            container.appendChild(itemDiv);
        }

        // Update indices for array items after removal
        function updateArrayItemIndices(propName) {
            const container = document.getElementById('array-container-' + propName);
            const items = container.querySelectorAll('.array-item');

            items.forEach((item, index) => {
                item.dataset.index = index;

                // Update all input IDs and names within this item
                const inputs = item.querySelectorAll('input, select, textarea');
                inputs.forEach(input => {
                    const fieldName = input.dataset.field;
                    input.id = 'param-' + propName + '-' + index + '-' + fieldName;
                    input.name = propName + '-' + index + '-' + fieldName;
                });
            });
        }

        // Collect values from form
        function collectFormValues(tool) {
            const params = {};

            // Check for schema in either parameters or inputSchema
            let schema = null;
            if (tool.parameters && tool.parameters.properties) {
                schema = tool.parameters;
            } else if (tool.inputSchema && tool.inputSchema.properties) {
                schema = tool.inputSchema;
            }

            if (!schema) {
                return params;
            }

            const properties = schema.properties;

            for (const propName in properties) {
                const prop = properties[propName];

                if (prop.type === 'array' && prop.items && prop.items.type === 'object' && prop.items.properties) {
                    // Handle array of objects using the specialized UI
                    const container = document.getElementById('array-container-' + propName);
                    if (!container) continue;

                    const items = container.querySelectorAll('.array-item');
                    const arrayValues = [];

                    items.forEach(item => {
                        const itemIndex = item.dataset.index;
                        const itemValue = {};

                        // Collect all field values for this item
                        for (const fieldName in prop.items.properties) {
                            const input = document.getElementById('param-' + propName + '-' + itemIndex + '-' + fieldName);
                            if (!input) continue;

                            let value = input.value;

                            // Convert types appropriately
                            const fieldProp = prop.items.properties[fieldName];
                            switch (fieldProp.type) {
                                case 'boolean':
                                    value = value === 'true';
                                    break;

                                case 'number':
                                case 'integer':
                                    if (value !== '') {
                                        value = fieldProp.type === 'integer' ? parseInt(value, 10) : parseFloat(value);
                                    } else {
                                        continue; // Skip empty values
                                    }
                                    break;

                                default:
                                    // For strings, just use as-is
                                    break;
                            }

                            // Only add non-empty values
                            if (value !== '' && value !== undefined) {
                                itemValue[fieldName] = value;
                            }
                        }

                        // Only add non-empty objects
                        if (Object.keys(itemValue).length > 0) {
                            arrayValues.push(itemValue);
                        }
                    });

                    params[propName] = arrayValues;
                } else {
                    // Handle regular inputs or simple arrays
                    const input = document.getElementById('param-' + propName);

                    if (!input) continue;

                    let value = input.value;

                    // Convert types appropriately
                    switch (prop.type) {
                        case 'boolean':
                            value = value === 'true';
                            break;

                        case 'number':
                        case 'integer':
                            if (value !== '') {
                                value = prop.type === 'integer' ? parseInt(value, 10) : parseFloat(value);
                            } else {
                                continue; // Skip empty values
                            }
                            break;

                        case 'array':
                            if (value) {
                                // Split by new lines and filter empty lines
                                value = value.split('\n')
                                    .map(item => item.trim())
                                    .filter(item => item !== '');
                            } else {
                                value = [];
                            }
                            break;

                        case 'object':
                            if (value) {
                                try {
                                    value = JSON.parse(value);
                                } catch (e) {
                                    alert('Invalid JSON in field "' + propName + '": ' + e.message);
                                    return null;
                                }
                            } else {
                                value = {};
                            }
                            break;

                        default:
                            // For strings, just use as-is
                            break;
                    }

                    // Only add non-empty values
                    if (value !== '' && value !== undefined) {
                        params[propName] = value;
                    }
                }
            }

            return params;
        }

        // Call a tool with parameters
        function callTool(name, params) {
            fetch('/api/call', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    type: 'tool',
                    name: name,
                    params: params
                })
            })
            .then(response => response.json())
            .then(data => {
                document.getElementById('raw-output-container').textContent = JSON.stringify(data, null, 2);
                displayFormattedOutput(data);
                // Activate formatted tab
                document.getElementById('formatted-tab').click();
            })
            .catch(err => {
                document.getElementById('raw-output-container').textContent = 'Error calling tool: ' + err.message;
                document.getElementById('formatted-output-container').innerHTML =
                    '<div class="result-object"><h3>Error</h3><div class="result-property">' +
                    err.message + '</div></div>';
            });
        }

        // Call a resource
        function callResource(uri) {
            document.getElementById('main-title').textContent = 'Resource: ' + uri;
            document.getElementById('tool-description').classList.add('hidden');
            document.getElementById('tool-panel').classList.add('hidden');

            fetch('/api/call', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    type: 'resource',
                    name: uri
                })
            })
            .then(response => response.json())
            .then(data => {
                document.getElementById('raw-output-container').textContent = JSON.stringify(data, null, 2);
                displayFormattedOutput(data);
                // Activate formatted tab
                document.getElementById('formatted-tab').click();
            })
            .catch(err => {
                document.getElementById('raw-output-container').textContent = 'Error reading resource: ' + err.message;
                document.getElementById('formatted-output-container').innerHTML =
                    '<div class="result-object"><h3>Error</h3><div class="result-property">' +
                    err.message + '</div></div>';
            });
        }

        // Call a prompt
        function callPrompt(name) {
            document.getElementById('main-title').textContent = 'Prompt: ' + name;
            document.getElementById('tool-description').classList.add('hidden');
            document.getElementById('tool-panel').classList.add('hidden');

            fetch('/api/call', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    type: 'prompt',
                    name: name
                })
            })
            .then(response => response.json())
            .then(data => {
                document.getElementById('raw-output-container').textContent = JSON.stringify(data, null, 2);
                displayFormattedOutput(data);
                // Activate formatted tab
                document.getElementById('formatted-tab').click();
            })
            .catch(err => {
                document.getElementById('raw-output-container').textContent = 'Error getting prompt: ' + err.message;
                document.getElementById('formatted-output-container').innerHTML =
                    '<div class="result-object"><h3>Error</h3><div class="result-property">' +
                    err.message + '</div></div>';
            });
        }

        // Display formatted output
        function displayFormattedOutput(data) {
            const container = document.getElementById('formatted-output-container');
            container.innerHTML = '';

            if (data.error) {
                const errorDiv = document.createElement('div');
                errorDiv.className = 'result-object';

                const errorTitle = document.createElement('h3');
                errorTitle.textContent = 'Error';
                errorDiv.appendChild(errorTitle);

                const errorText = document.createElement('div');
                errorText.className = 'result-property';
                errorText.textContent = data.error;
                errorDiv.appendChild(errorText);

                container.appendChild(errorDiv);
                return;
            }

            renderObject(data, container);
        }

        // Recursively render object
        function renderObject(obj, container, level = 0) {
            if (!obj || typeof obj !== 'object') return;

            for (const key in obj) {
                if (!obj.hasOwnProperty(key)) continue;

                const value = obj[key];

                if (value && typeof value === 'object' && !Array.isArray(value)) {
                    // This is an object
                    const objectDiv = document.createElement('div');
                    objectDiv.className = 'pl-4 border-l-2 border-blue-400 mb-4';
                    objectDiv.style.marginLeft = (level * 16) + 'px';

                    const objectTitle = document.createElement('h3');
                    objectTitle.className = 'text-lg font-semibold text-blue-600 mb-2';
                    objectTitle.textContent = key;
                    objectDiv.appendChild(objectTitle);

                    container.appendChild(objectDiv);

                    // Recursively render properties
                    renderObject(value, objectDiv, level + 1);
                } else if (key === "content" && Array.isArray(value)) {
                    // Special handling for content arrays that might contain parseable JSON
                    const contentDiv = document.createElement('div');
                    contentDiv.className = 'pl-4 border-l-2 border-blue-400 mb-4';
                    contentDiv.style.marginLeft = (level * 16) + 'px';

                    const contentTitle = document.createElement('h3');
                    contentTitle.className = 'text-lg font-semibold text-blue-600 mb-2';
                    contentTitle.textContent = key;
                    contentDiv.appendChild(contentTitle);

                    container.appendChild(contentDiv);

                    // Process each content item
                    value.forEach((item, index) => {
                        if (typeof item === 'object') {
                            // If it's already an object, render it directly
                            const itemDiv = document.createElement('div');
                            itemDiv.className = 'pl-4 border-l-2 border-gray-300 mb-2 pb-2';
                            itemDiv.style.marginLeft = '16px';

                            const itemTitle = document.createElement('h3');
                            itemTitle.className = 'text-md font-semibold text-gray-700 mb-1';
                            itemTitle.textContent = 'Item ' + (index + 1);
                            itemDiv.appendChild(itemTitle);

                            contentDiv.appendChild(itemDiv);
                            renderObject(item, itemDiv, level + 2);
                        } else if (typeof item === 'string') {
                            // Try to parse it as JSON
                            try {
                                const parsedItem = JSON.parse(item);
                                if (typeof parsedItem === 'object' && parsedItem !== null) {
                                    const itemDiv = document.createElement('div');
                                    itemDiv.className = 'pl-4 border-l-2 border-gray-300 mb-2 pb-2';
                                    itemDiv.style.marginLeft = '16px';

                                    const itemTitle = document.createElement('h3');
                                    itemTitle.className = 'text-md font-semibold text-gray-700 mb-1';
                                    itemTitle.textContent = 'Item ' + (index + 1);
                                    itemDiv.appendChild(itemTitle);

                                    contentDiv.appendChild(itemDiv);
                                    renderObject(parsedItem, itemDiv, level + 2);
                                } else {
                                    // Primitive value, render as is
                                    renderPrimitiveValue(contentDiv, 'Item ' + (index + 1), parsedItem, level + 1);
                                }
                            } catch (e) {
                                // Not valid JSON, render as string
                                renderPrimitiveValue(contentDiv, 'Item ' + (index + 1), item, level + 1);
                            }
                        } else {
                            // Other primitive types
                            renderPrimitiveValue(contentDiv, 'Item ' + (index + 1), item, level + 1);
                        }
                    });
                } else {
                    // This is a primitive or array
                    renderPrimitiveValue(container, key, value, level);
                }
            }
        }

        // Helper function to render primitive values
        function renderPrimitiveValue(container, key, value, level) {
            const propertyDiv = document.createElement('div');
            propertyDiv.className = 'py-1 flex flex-wrap';
            propertyDiv.style.marginLeft = (level * 16) + 'px';

            const nameSpan = document.createElement('span');
            nameSpan.className = 'text-gray-600 mr-2 font-medium';
            nameSpan.textContent = key + ': ';
            propertyDiv.appendChild(nameSpan);

            const valueSpan = document.createElement('span');
            valueSpan.className = 'font-mono';

            if (value === null) {
                valueSpan.classList.add('text-gray-500');
                valueSpan.classList.add('italic');
                valueSpan.textContent = 'null';
            } else if (Array.isArray(value)) {
                valueSpan.textContent = JSON.stringify(value);
            } else {
                const type = typeof value;

                if (type === 'string') {
                    valueSpan.classList.add('text-green-600');
                } else if (type === 'number') {
                    valueSpan.classList.add('text-orange-600');
                } else if (type === 'boolean') {
                    valueSpan.classList.add('text-red-600');
                }

                // Check if string might be parseable JSON
                if (type === 'string' && value.trim().startsWith('{') && value.trim().endsWith('}')) {
                    try {
                        // Try to parse and pretty print
                        const parsed = JSON.parse(value);
                        valueSpan.textContent = JSON.stringify(parsed, null, 2);

                        // Add a special class for JSON strings
                        valueSpan.className = 'font-mono p-3 mt-2 mb-2 block bg-gray-50 border border-gray-200 rounded-md overflow-x-auto whitespace-pre';
                    } catch (e) {
                        // If it fails to parse, display as regular string
                        valueSpan.textContent = '"' + value + '"';
                    }
                } else {
                    valueSpan.textContent = type === 'string' ? '"' + value + '"' : String(value);
                }
            }

            propertyDiv.appendChild(valueSpan);
            container.appendChild(propertyDiv);
        }
    </script>
</body>
</html>
`
		w.Header().Set("Content-Type", "text/html")
		//nolint:errcheck,gosec // No need to handle error from Write in this context
		w.Write([]byte(html))
	}
}

// handleTools handles API requests for listing tools.
func handleTools(cache *MCPClientCache) http.HandlerFunc {
	//nolint:revive // Parameter r is required by http.HandlerFunc signature
	return func(w http.ResponseWriter, r *http.Request) {
		cache.mutex.Lock()
		resp, err := cache.client.ListTools(context.Background(), mcp.ListToolsRequest{})
		cache.mutex.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			//nolint:errcheck,gosec // No need to handle error from Encode in this context
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		//nolint:errcheck,gosec // No need to handle error from Encode in this context
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": resp,
		})
	}
}

// handleResources handles API requests for listing resources.
func handleResources(cache *MCPClientCache) http.HandlerFunc {
	//nolint:revive // Parameter r is required by http.HandlerFunc signature
	return func(w http.ResponseWriter, r *http.Request) {
		cache.mutex.Lock()
		resp, err := cache.client.ListResources(context.Background(), mcp.ListResourcesRequest{})
		cache.mutex.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			//nolint:errcheck,gosec // No need to handle error from Encode in this context
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		//nolint:errcheck,gosec // No need to handle error from Encode in this context
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": resp,
		})
	}
}

// handlePrompts handles API requests for listing prompts.
func handlePrompts(cache *MCPClientCache) http.HandlerFunc {
	//nolint:revive // Parameter r is required by http.HandlerFunc signature
	return func(w http.ResponseWriter, r *http.Request) {
		cache.mutex.Lock()
		resp, err := cache.client.ListPrompts(context.Background(), mcp.ListPromptsRequest{})
		cache.mutex.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			//nolint:errcheck,gosec // No need to handle error from Encode in this context
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		//nolint:errcheck,gosec // No need to handle error from Encode in this context
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": resp,
		})
	}
}

// handleCall handles API requests for calling tools/resources/prompts.
func handleCall(cache *MCPClientCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var requestData struct {
			Params map[string]interface{} `json:"params"`
			Type   string                 `json:"type"`
			Name   string                 `json:"name"`
		}

		err := json.NewDecoder(r.Body).Decode(&requestData)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			//nolint:errcheck,gosec // No need to handle error from Encode in this context
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "Invalid request: " + err.Error(),
			})
			return
		}

		var resp map[string]interface{}
		var callErr error

		cache.mutex.Lock()
		defer cache.mutex.Unlock()

		switch requestData.Type {
		case EntityTypeTool:
			var toolResponse *mcp.CallToolResult
			request := mcp.CallToolRequest{}
			request.Params.Name = requestData.Name
			request.Params.Arguments = requestData.Params
			toolResponse, callErr = cache.client.CallTool(context.Background(), request)
			resp = ConvertJSONToMap(toolResponse)
		case EntityTypeRes:
			var resourceResponse *mcp.ReadResourceResult
			request := mcp.ReadResourceRequest{}
			request.Params.URI = requestData.Name
			resourceResponse, callErr = cache.client.ReadResource(context.Background(), request)
			resp = ConvertJSONToMap(resourceResponse)
		case EntityTypePrompt:
			var promptResponse *mcp.GetPromptResult
			request := mcp.GetPromptRequest{}
			request.Params.Name = requestData.Name
			promptResponse, callErr = cache.client.GetPrompt(context.Background(), request)
			resp = ConvertJSONToMap(promptResponse)
		default:
			w.WriteHeader(http.StatusBadRequest)
			//nolint:errcheck,gosec // No need to handle error from Encode in this context
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "Invalid entity type: " + requestData.Type,
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if callErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			//nolint:errcheck,gosec // No need to handle error from Encode in this context
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": callErr.Error(),
			})
			return
		}

		//nolint:errcheck,gosec // No need to handle error from Encode in this context
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": resp,
		})
	}
}
