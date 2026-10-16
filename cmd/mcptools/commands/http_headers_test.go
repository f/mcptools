package commands

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessCustomHeaders(t *testing.T) {
	original := CustomHTTPHeaders
	defer func() { CustomHTTPHeaders = original }()
	for _, args := range [][]string{
		{"--headers", "X-Project=demo", "server"},
		{"server", "--headers", "X-Project=demo"},
		{"--headers", "X-Project=demo", "server", "--headers", "X-Mode=unit"},
	} {
		CustomHTTPHeaders = nil
		assert.Equal(t, []string{"server"}, ProcessFlags(args))
		assert.NotEmpty(t, CustomHTTPHeaders)
	}
}

func TestCallCustomHeaders(t *testing.T) {
	original := CustomHTTPHeaders
	defer func() { CustomHTTPHeaders = original }()
	CustomHTTPHeaders = nil
	entity, args := parseCallArgs([]string{"lookup", "--headers", "X-Project=demo", "server"})
	assert.Equal(t, "lookup", entity)
	assert.Equal(t, []string{"server"}, args)
	assert.Equal(t, []string{"X-Project=demo"}, CustomHTTPHeaders)
}

func TestAddCustomHTTPHeaders(t *testing.T) {
	headers := map[string]string{"Accept": "application/json, text/event-stream"}
	require.NoError(t, addCustomHTTPHeaders(headers, []string{"X-Project=demo", "x-project=updated", "X-Expression=a=b", "X-Empty="}))
	assert.Equal(t, "updated", headers["X-Project"])
	assert.Equal(t, "a=b", headers["X-Expression"])
	assert.Equal(t, "", headers["X-Empty"])
	assert.Equal(t, "application/json, text/event-stream", headers["Accept"])

	for _, option := range []string{"missing-value", "=empty-name", "Bad Name=private-value", "X-Test=private-value\r\nInjected: true", "X-Test=private-value\x00"} {
		err := addCustomHTTPHeaders(map[string]string{}, []string{option})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "private-value")
	}
}

func TestCustomHTTPHeadersTransports(t *testing.T) {
	originalHeaders, originalTransport := CustomHTTPHeaders, TransportOption
	originalUser, originalAuth := AuthUser, AuthHeader
	defer func() {
		CustomHTTPHeaders, TransportOption = originalHeaders, originalTransport
		AuthUser, AuthHeader = originalUser, originalAuth
	}()
	for _, transport := range []string{TransportHTTP, TransportSSE} {
		t.Run(transport, func(t *testing.T) {
			CustomHTTPHeaders, TransportOption, AuthUser, AuthHeader = nil, transport, "", t.Name()
			mcpServer := server.NewMCPServer("header-test", "1.0.0", server.WithToolCapabilities(false))
			var handler http.Handler = server.NewStreamableHTTPServer(mcpServer)
			if transport == TransportSSE {
				handler = server.NewSSEServer(mcpServer)
			}
			var requests atomic.Int32
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, "demo", r.Header.Get("X-Project"))
				assert.Equal(t, "a=b", r.Header.Get("X-Expression"))
				assert.Equal(t, t.Name(), r.Header.Get("Authorization"))
				handler.ServeHTTP(w, r)
			}))
			defer httpServer.Close()
			url := httpServer.URL
			if transport == TransportSSE {
				url += "/sse"
			}
			args := ProcessFlags([]string{url, "--headers", "X-Project=demo", "--headers", "X-Expression=a=b", "--headers", "Authorization=overridden-option"})
			mcpClient, err := CreateClientFunc(args)
			require.NoError(t, err)
			defer func() { assert.NoError(t, mcpClient.Close()) }()
			_, err = mcpClient.ListTools(context.Background(), mcp.ListToolsRequest{})
			require.NoError(t, err)
			assert.Positive(t, requests.Load())
		})
	}
}

func TestCustomHTTPHeadersOrigin(t *testing.T) {
	var received atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer other.Close()
	current := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/same" {
			assert.Equal(t, "demo", r.Header.Get("X-Project"))
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/local" {
			http.Redirect(w, r, "/same", http.StatusTemporaryRedirect)
			return
		}
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer current.Close()
	httpClient := customHeaderClient(current.URL, map[string]string{"X-Project": "demo"})
	for _, path := range []string{"/same", "/local"} {
		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, current.URL+path, nil)
		require.NoError(t, err)
		request.Header.Set("X-Project", "demo")
		response, err := httpClient.Do(request)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, response.Body)
		require.NoError(t, response.Body.Close())
	}
	_, err := httpClient.Get(current.URL + "/cross-origin")
	require.Error(t, err)
	assert.Zero(t, received.Load())
	_, err = httpClient.Get(other.URL)
	require.Error(t, err)
	assert.Zero(t, received.Load())
	_, err = httpClient.Get("https" + current.URL[len("http"):])
	require.ErrorContains(t, err, "different origin")
}
