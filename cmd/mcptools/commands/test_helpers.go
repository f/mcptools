// Package commands implements individual commands for the MCP CLI.
package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// MockTransport implements the transport.Transport interface for testing.
type MockTransport struct {
	ExecuteFunc func(method string, params any) (map[string]any, error)
}

// Start is a no-op for the mock transport.
func (m *MockTransport) Start(_ context.Context) error {
	return nil
}

// SendRequest overrides the default implementation of the transport.SendRequest method.
func (m *MockTransport) SendRequest(_ context.Context, request transport.JSONRPCRequest) (*transport.JSONRPCResponse, error) {
	if request.Method == "initialize" {
		return &transport.JSONRPCResponse{Result: json.RawMessage(`{}`)}, nil
	}
	response, err := m.ExecuteFunc(request.Method, request.Params)
	if err != nil {
		return nil, err
	}
	responseBytes, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	fmt.Println("Returning response:", string(responseBytes))
	return &transport.JSONRPCResponse{Result: json.RawMessage(responseBytes)}, nil
}

// SendNotification is a no-op for the mock transport.
func (m *MockTransport) SendNotification(_ context.Context, _ mcp.JSONRPCNotification) error {
	return nil
}

// SetNotificationHandler is a no-op for the mock transport.
func (m *MockTransport) SetNotificationHandler(_ func(notification mcp.JSONRPCNotification)) {
}

// Close is a no-op for the mock transport.
func (m *MockTransport) Close() error {
	return nil
}

// GetSessionId is a no-op for the mock transport
func (m *MockTransport) GetSessionId() string {
	return ""
}

// setupMockClient creates a mock client with the given execute function and returns cleanup function.
func setupMockClient(executeFunc func(method string, _ any) (map[string]any, error)) func() {
	// Save original function and restore later
	originalFunc := CreateClientFunc

	mockTransport := &MockTransport{
		ExecuteFunc: executeFunc,
	}

	mockClient := client.NewClient(mockTransport)
	_, _ = mockClient.Initialize(context.Background(), mcp.InitializeRequest{})

	// Override the function that creates clients
	CreateClientFunc = func(_ []string, _ ...client.ClientOption) (*client.Client, error) {
		return mockClient, nil
	}

	// Return a cleanup function
	return func() {
		CreateClientFunc = originalFunc
	}
}

// assertContains checks if the output contains the expected string.
func assertContains(t *testing.T, output string, expected string) {
	t.Helper()
	if !bytes.Contains([]byte(output), []byte(expected)) {
		t.Errorf("Expected output to contain %q, got: %s", expected, output)
	}
}

// assertEqual checks if two values are equal.
func assertEquals(t *testing.T, output string, expected string) {
	t.Helper()
	if output != expected {
		t.Errorf("Expected %v, got %v", expected, output)
	}
}
