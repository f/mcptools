# MCP Project Templates

This directory contains templates for creating MCP (Model Context Protocol) servers with different capabilities.

## Installation

The templates will be automatically found if they are in one of these locations:

1. `./templates/`: Local project directory
2. `~/.mcpt/templates/`: User's home directory
3. Next to the executable in the installed location

To install templates to your home directory:

```bash
make templates
```

## Usage

Create a new MCP project with the `mcp new` command:

```bash
# Create a project with a tool, resource, and prompt
mcp new tool:hello_world resource:file prompt:hello

# Create a project with a specific SDK (currently only TypeScript/ts supported)
mcp new tool:hello_world --sdk=ts

# Create a project with a specific transport (stdio, sse, or http)
mcp new tool:hello_world --transport=stdio
mcp new tool:hello_world --transport=sse
mcp new tool:hello_world --transport=http
```

## Available Templates

### TypeScript (ts)

- **tool**: Basic tool implementation template
- **resource**: Resource implementation template
- **prompt**: Prompt implementation template
- **server_stdio**: Server with stdio transport
- **server_sse**: Server with SSE transport
- **server_http**: Server with streamable HTTP transport
- **full_server**: Complete server with all three capabilities

## Project Structure

The scaffolding creates the following structure:

```
my-project/
├── package.json
├── tsconfig.json
└── src/
    ├── index.ts
    └── [component].ts
```

After scaffolding, run:

```bash
npm install
npm run build
npm start
``` 