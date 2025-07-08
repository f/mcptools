package main

import (
	"testing"

	"github.com/f/mcptools/cmd/mcptools/commands"
)

func TestTransportFlag(t *testing.T) {
	// Save original values
	origTransport := commands.TransportOption

	// Test default value
	if commands.TransportOption != "http" {
		t.Errorf("Expected default transport to be 'http', got '%s'", commands.TransportOption)
	}

	// Test ProcessFlags with transport flag
	args := []string{"tools", "--transport", "sse", "http://localhost:3000"}
	remainingArgs := commands.ProcessFlags(args)

	expectedArgs := []string{"tools", "http://localhost:3000"}
	if len(remainingArgs) != len(expectedArgs) {
		t.Errorf("Expected %d args, got %d", len(expectedArgs), len(remainingArgs))
	}

	for i, arg := range expectedArgs {
		if remainingArgs[i] != arg {
			t.Errorf("Expected arg %d to be '%s', got '%s'", i, arg, remainingArgs[i])
		}
	}

	if commands.TransportOption != "sse" {
		t.Errorf("Expected transport to be 'sse', got '%s'", commands.TransportOption)
	}

	// Restore original values
	commands.TransportOption = origTransport
}

func TestIsHTTP(t *testing.T) {
	testCases := []struct {
		url      string
		expected bool
	}{
		{"http://localhost:3000", true},
		{"https://example.com", true},
		{"localhost:3000", true},
		{"stdio", false},
		{"", false},
		{"file:///path", false},
	}

	for _, tc := range testCases {
		result := commands.IsHTTP(tc.url)
		if result != tc.expected {
			t.Errorf("IsHTTP(%s) = %v, expected %v", tc.url, result, tc.expected)
		}
	}
}