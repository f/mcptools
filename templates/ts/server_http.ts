import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import express from "express";
import { z } from "zod";
import { randomUUID } from "crypto";

// Initialize server
const server = new McpServer({
  name: "PROJECT_NAME",
  version: "1.0.0"
});

// Initialize components here
// COMPONENT_INITIALIZATION

// Setup Express app
const app = express();

// Enable JSON parsing for request bodies
app.use(express.json());

// Create a streamable HTTP transport
const transport = new StreamableHTTPServerTransport({
  // Enable session management with auto-generated UUIDs
  sessionIdGenerator: () => randomUUID(),
  // Optional: Enable JSON response mode for simple request/response
  enableJsonResponse: false
});

// Handle all HTTP methods on the root path
app.all("/", async (req, res) => {
  await transport.handleRequest(req, res, req.body);
});

// Connect the server to the transport
await server.connect(transport);

// Start HTTP server
const port = 3000;
app.listen(port, () => {
  console.log(`MCP server running on http://localhost:${port}`);
}); 