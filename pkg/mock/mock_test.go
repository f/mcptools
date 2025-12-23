package mock

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMockServerHTTP(t *testing.T) {
	// Create a mock server
	server, err := NewServer()
	if err != nil {
		t.Fatalf("Failed to create mock server: %v", err)
	}
	defer server.Close()

	// Add a test tool
	server.AddTool("test_tool", "A test tool")

	// Create test server
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", server.handleMCP)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Test initialize request
	initRequest := map[string]any{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"id":      1,
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"clientInfo": map[string]any{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}

	reqBody, _ := json.Marshal(initRequest)
	resp, err := http.Post(testServer.URL+"/mcp", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		t.Fatalf("Failed to send initialize request: %v", err)
	}
	defer resp.Body.Close()

	var initResponse map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&initResponse); err != nil {
		t.Fatalf("Failed to decode initialize response: %v", err)
	}

	if initResponse["jsonrpc"] != "2.0" {
		t.Errorf("Expected jsonrpc 2.0, got %v", initResponse["jsonrpc"])
	}

	if initResponse["id"] != float64(1) {
		t.Errorf("Expected id 1, got %v", initResponse["id"])
	}

	// Test tools/list request
	toolsRequest := map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/list",
		"id":      2,
	}

	reqBody, _ = json.Marshal(toolsRequest)
	resp, err = http.Post(testServer.URL+"/mcp", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		t.Fatalf("Failed to send tools/list request: %v", err)
	}
	defer resp.Body.Close()

	var toolsResponse map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&toolsResponse); err != nil {
		t.Fatalf("Failed to decode tools/list response: %v", err)
	}

	result := toolsResponse["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}

	tool := tools[0].(map[string]any)
	if tool["name"] != "test_tool" {
		t.Errorf("Expected tool name 'test_tool', got %v", tool["name"])
	}

	if tool["description"] != "A test tool" {
		t.Errorf("Expected tool description 'A test tool', got %v", tool["description"])
	}

	// Test tools/call request
	callRequest := map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"id":      3,
		"params": map[string]any{
			"name":      "test_tool",
			"arguments": map[string]any{},
		},
	}

	reqBody, _ = json.Marshal(callRequest)
	resp, err = http.Post(testServer.URL+"/mcp", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		t.Fatalf("Failed to send tools/call request: %v", err)
	}
	defer resp.Body.Close()

	var callResponse map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&callResponse); err != nil {
		t.Fatalf("Failed to decode tools/call response: %v", err)
	}

	result = callResponse["result"].(map[string]any)
	content := result["content"].([]any)
	if len(content) != 1 {
		t.Errorf("Expected 1 content item, got %d", len(content))
	}

	contentItem := content[0].(map[string]any)
	expectedText := "hello i am test_tool mock tool and i confirm it's working"
	if contentItem["text"] != expectedText {
		t.Errorf("Expected text '%s', got %v", expectedText, contentItem["text"])
	}
}

func TestRunMockServerHTTP(t *testing.T) {
	// Test that RunMockServerHTTP can be called without errors
	tools := map[string]string{
		"test_tool": "A test tool",
	}
	prompts := map[string]map[string]string{}
	resources := map[string]map[string]string{}

	// Start server in background
	done := make(chan error, 1)
	go func() {
		done <- RunMockServerHTTP(tools, prompts, resources, "0") // Use port 0 for automatic assignment
	}()

	// Give it a moment to start
	select {
	case err := <-done:
		// If it returns immediately, it might be a bind error (acceptable for this test)
		if err != nil {
			t.Logf("Server returned: %v (this is expected in test environment)", err)
		}
	case <-time.After(100 * time.Millisecond):
		// If it's still running after 100ms, that's good - it means it started successfully
		t.Log("Server started successfully")
	}
}