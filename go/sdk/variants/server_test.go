// Copyright 2025 The MCP Variants Authors. All rights reserved.
// Use of this source code is governed by a Apache-2.0
// license that can be found in the LICENSE file.

package variants

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestServers creates two mcp.Servers with distinct tools:
//   - "coding": analyze_code, refactor
//   - "compact": summarize, lookup
func newTestServers() (coding *mcp.Server, compact *mcp.Server) {
	coding = mcp.NewServer(&mcp.Implementation{Name: "coding-server", Version: "v1.0.0"}, nil)
	mcp.AddTool(coding, &mcp.Tool{Name: "analyze_code", Description: "Static analysis"}, analyzeCode)
	mcp.AddTool(coding, &mcp.Tool{Name: "refactor", Description: "Refactor code"}, refactor)

	compact = mcp.NewServer(&mcp.Implementation{Name: "compact-server", Version: "v1.0.0"}, nil)
	mcp.AddTool(compact, &mcp.Tool{Name: "summarize", Description: "Summarize text"}, summarize)
	mcp.AddTool(compact, &mcp.Tool{Name: "lookup", Description: "Quick lookup"}, lookup)

	return coding, compact
}

// newTestVariantServer creates a variant server with two variants.
func newTestVariantServer() *Server {
	codingServer, compactServer := newTestServers()

	return NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}).
		WithVariant(ServerVariant{
			ID:          "coding",
			Description: "Optimized for coding workflows",
			Status:      Stable,
		}, codingServer, 0).
		WithVariant(ServerVariant{
			ID:          "compact",
			Description: "Minimal token usage",
			Status:      Experimental,
		}, compactServer, 1)
}

// connectTestClient starts the variant server and connects a test client.
// Returns the client session; cleanup is handled via t.Cleanup.
func connectTestClient(t *testing.T, vs *Server, clientOpts *mcp.ClientOptions) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	errCh := make(chan error, 1)
	go func() {
		errCh <- vs.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "v0.0.1"},
		clientOpts,
	)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		t.Fatalf("client.Connect: %v", err)
	}

	t.Cleanup(func() {
		session.Close()
		cancel()
		<-errCh
	})

	return session
}

// TestIntegration_EndToEnd verifies the full variant server lifecycle with a
// variant-aware client: the client advertises variant support and sends hints
// during initialization, then selects variants per-request via _meta.
func TestIntegration_EndToEnd(t *testing.T) {
	vs := newTestVariantServer()
	session := connectTestClient(t, vs, nil)
	ctx := context.Background()

	// --- 1. Verify initialize returned variant information ---
	initResult := session.InitializeResult()
	require.NotNil(t, initResult)
	require.NotNil(t, initResult.Capabilities)

	ext, ok := initResult.Capabilities.Experimental[extensionID]
	require.True(t, ok, "expected experimental capability with extension ID")

	extJSON, err := json.Marshal(ext)
	require.NoError(t, err)

	var extData struct {
		AvailableVariants []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
			Status      string `json:"status"`
		} `json:"availableVariants"`
		MoreVariantsAvailable bool `json:"moreVariantsAvailable"`
	}
	require.NoError(t, json.Unmarshal(extJSON, &extData))
	require.Len(t, extData.AvailableVariants, 2)

	// Default ranking: coding (priority 0, stable) first
	assert.Equal(t, "coding", extData.AvailableVariants[0].ID)
	assert.Equal(t, "compact", extData.AvailableVariants[1].ID)
	assert.False(t, extData.MoreVariantsAvailable)

	// --- 2. List tools for the "coding" variant ---
	codingTools, err := session.ListTools(ctx, &mcp.ListToolsParams{
		Meta: mcp.Meta{metaKeyVariant: "coding"},
	})
	require.NoError(t, err)
	codingNames := toolNames(codingTools.Tools)
	assert.Contains(t, codingNames, "analyze_code")
	assert.Contains(t, codingNames, "refactor")
	assert.NotContains(t, codingNames, "summarize")

	// --- 3. List tools for the "compact" variant ---
	compactTools, err := session.ListTools(ctx, &mcp.ListToolsParams{
		Meta: mcp.Meta{metaKeyVariant: "compact"},
	})
	require.NoError(t, err)
	compactNames := toolNames(compactTools.Tools)
	assert.Contains(t, compactNames, "summarize")
	assert.Contains(t, compactNames, "lookup")
	assert.NotContains(t, compactNames, "analyze_code")

	// --- 4. Call a tool on the "coding" variant ---
	callResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "analyze_code",
		Meta: mcp.Meta{metaKeyVariant: "coding"},
		Arguments: map[string]json.RawMessage{
			"code":     json.RawMessage(`"fmt.Println(x)"`),
			"language": json.RawMessage(`"go"`),
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, callResult.Content)

	// --- 5. Call a tool on the "compact" variant ---
	callResult2, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "summarize",
		Meta: mcp.Meta{metaKeyVariant: "compact"},
		Arguments: map[string]json.RawMessage{
			"text": json.RawMessage(`"This is a long text that should be summarized into something shorter."`),
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, callResult2.Content)

	// --- 6. Cross-variant tool call: coding tool fails on compact variant ---
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "analyze_code",
		Meta: mcp.Meta{metaKeyVariant: "compact"},
		Arguments: map[string]json.RawMessage{
			"code":     json.RawMessage(`"x := 1"`),
			"language": json.RawMessage(`"go"`),
		},
	})
	assert.Error(t, err, "coding tool should not be reachable via compact variant")

	// --- 7. Invalid variant should return an error ---
	_, err = session.ListTools(ctx, &mcp.ListToolsParams{
		Meta: mcp.Meta{metaKeyVariant: "nonexistent"},
	})
	assert.Error(t, err)
}

// TestIntegration_ClientWithoutVariantSupport verifies that a client that
// doesn't know about variants can still use the server normally. All requests
// go to the first-ranked (default) variant without any _meta selection.
func TestIntegration_ClientWithoutVariantSupport(t *testing.T) {
	vs := newTestVariantServer()
	session := connectTestClient(t, vs, nil)
	ctx := context.Background()

	// --- 1. Initialize still succeeds and advertises variants (server always does) ---
	initResult := session.InitializeResult()
	require.NotNil(t, initResult)
	require.NotNil(t, initResult.Capabilities)
	assert.Contains(t, initResult.Capabilities.Experimental, extensionID,
		"server should advertise variants even for unaware clients")

	// --- 2. List tools without _meta — should get default variant ("coding") tools ---
	tools, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	names := toolNames(tools.Tools)
	assert.Contains(t, names, "analyze_code")
	assert.Contains(t, names, "refactor")
	assert.NotContains(t, names, "summarize")
	assert.NotContains(t, names, "lookup")

	// --- 3. Call a tool without _meta — should route to default variant ---
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "analyze_code",
		Arguments: map[string]json.RawMessage{
			"code":     json.RawMessage(`"x := 1"`),
			"language": json.RawMessage(`"go"`),
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.Content)

	// --- 4. Calling a tool that only exists on a non-default variant should fail ---
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "summarize",
		Arguments: map[string]json.RawMessage{
			"text": json.RawMessage(`"hello"`),
		},
	})
	assert.Error(t, err, "tool from non-default variant should not be reachable without _meta")
}

// connectHTTPTestClient connects a client to an existing httptest server via
// StreamableClientTransport. Returns the client session; cleanup is handled
// via t.Cleanup.
func connectHTTPTestClient(t *testing.T, httpSrv *httptest.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-http-client", Version: "v0.0.1"},
		nil,
	)

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: httpSrv.URL,
	}, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		session.Close()
	})

	return session
}

// TestIntegration_HTTP verifies the variant server works over HTTP with
// multiple concurrent clients. Each client gets its own session with
// independent variant routing.
func TestIntegration_HTTP(t *testing.T) {
	vs := newTestVariantServer()

	handler := NewStreamableHTTPHandler(vs, nil)

	httpSrv := httptest.NewServer(handler)
	t.Cleanup(httpSrv.Close)

	// --- Connect two clients concurrently ---
	session1 := connectHTTPTestClient(t, httpSrv)
	session2 := connectHTTPTestClient(t, httpSrv)

	ctx := context.Background()

	// --- 1. Both clients see variant metadata ---
	for _, session := range []*mcp.ClientSession{session1, session2} {
		initResult := session.InitializeResult()
		require.NotNil(t, initResult)
		require.NotNil(t, initResult.Capabilities)
		assert.Contains(t, initResult.Capabilities.Experimental, extensionID)
	}

	// --- 2. Client 1 uses "coding" variant, Client 2 uses "compact" ---
	var wg sync.WaitGroup
	wg.Add(2)

	// Client 1: coding variant
	go func() {
		defer wg.Done()

		tools, err := session1.ListTools(ctx, &mcp.ListToolsParams{
			Meta: mcp.Meta{metaKeyVariant: "coding"},
		})
		if !assert.NoError(t, err) {
			return
		}
		names := toolNames(tools.Tools)
		assert.Contains(t, names, "analyze_code")
		assert.Contains(t, names, "refactor")
		assert.NotContains(t, names, "summarize")

		result, err := session1.CallTool(ctx, &mcp.CallToolParams{
			Name: "analyze_code",
			Meta: mcp.Meta{metaKeyVariant: "coding"},
			Arguments: map[string]json.RawMessage{
				"code":     json.RawMessage(`"x := 1"`),
				"language": json.RawMessage(`"go"`),
			},
		})
		if !assert.NoError(t, err) {
			return
		}
		assert.NotEmpty(t, result.Content)
	}()

	// Client 2: compact variant
	go func() {
		defer wg.Done()

		tools, err := session2.ListTools(ctx, &mcp.ListToolsParams{
			Meta: mcp.Meta{metaKeyVariant: "compact"},
		})
		if !assert.NoError(t, err) {
			return
		}
		names := toolNames(tools.Tools)
		assert.Contains(t, names, "summarize")
		assert.Contains(t, names, "lookup")
		assert.NotContains(t, names, "analyze_code")

		result, err := session2.CallTool(ctx, &mcp.CallToolParams{
			Name: "summarize",
			Meta: mcp.Meta{metaKeyVariant: "compact"},
			Arguments: map[string]json.RawMessage{
				"text": json.RawMessage(`"A long text to summarize"`),
			},
		})
		if !assert.NoError(t, err) {
			return
		}
		assert.NotEmpty(t, result.Content)
	}()

	wg.Wait()

	// --- 3. Cross-variant isolation: coding tool fails on compact ---
	_, err := session1.CallTool(ctx, &mcp.CallToolParams{
		Name: "analyze_code",
		Meta: mcp.Meta{metaKeyVariant: "compact"},
		Arguments: map[string]json.RawMessage{
			"code":     json.RawMessage(`"x := 1"`),
			"language": json.RawMessage(`"go"`),
		},
	})
	assert.Error(t, err, "coding tool should not be reachable via compact variant")
}

// TestIntegration_HTTP_Stateless verifies the variant server works in
// stateless HTTP mode where shared connections are reused across requests.
func TestIntegration_HTTP_Stateless(t *testing.T) {
	vs := newTestVariantServer()

	handler := NewStreamableHTTPHandler(vs,
		&mcp.StreamableHTTPOptions{Stateless: true},
	)

	httpSrv := httptest.NewServer(handler)
	t.Cleanup(httpSrv.Close)

	session := connectHTTPTestClient(t, httpSrv)
	ctx := context.Background()

	// Variant metadata is still returned
	initResult := session.InitializeResult()
	require.NotNil(t, initResult)
	assert.Contains(t, initResult.Capabilities.Experimental, extensionID)

	// List tools for coding variant
	codingTools, err := session.ListTools(ctx, &mcp.ListToolsParams{
		Meta: mcp.Meta{metaKeyVariant: "coding"},
	})
	require.NoError(t, err)
	names := toolNames(codingTools.Tools)
	assert.Contains(t, names, "analyze_code")
	assert.NotContains(t, names, "summarize")

	// Call tool on compact variant
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "summarize",
		Meta: mcp.Meta{metaKeyVariant: "compact"},
		Arguments: map[string]json.RawMessage{
			"text": json.RawMessage(`"stateless test"`),
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.Content)
}

// ---------------------------------------------------------------------------
// Test tool handlers
// ---------------------------------------------------------------------------

type analyzeCodeInput struct {
	Code     string `json:"code"`
	Language string `json:"language"`
}

type analyzeCodeOutput struct {
	Issues []string `json:"issues"`
}

func analyzeCode(_ context.Context, _ *mcp.CallToolRequest, in analyzeCodeInput) (*mcp.CallToolResult, analyzeCodeOutput, error) {
	return nil, analyzeCodeOutput{Issues: []string{"unused variable"}}, nil
}

type refactorInput struct {
	Code   string `json:"code"`
	Action string `json:"action"`
}

type refactorOutput struct {
	Refactored string `json:"refactored"`
}

func refactor(_ context.Context, _ *mcp.CallToolRequest, in refactorInput) (*mcp.CallToolResult, refactorOutput, error) {
	return nil, refactorOutput{Refactored: "// refactored\n" + in.Code}, nil
}

type summarizeInput struct {
	Text string `json:"text"`
}

type summarizeOutput struct {
	Summary string `json:"summary"`
}

func summarize(_ context.Context, _ *mcp.CallToolRequest, in summarizeInput) (*mcp.CallToolResult, summarizeOutput, error) {
	n := len(in.Text)
	if n > 50 {
		n = 50
	}
	return nil, summarizeOutput{Summary: in.Text[:n]}, nil
}

type lookupInput struct {
	Query string `json:"query"`
}

type lookupOutput struct {
	Result string `json:"result"`
}

func lookup(_ context.Context, _ *mcp.CallToolRequest, in lookupInput) (*mcp.CallToolResult, lookupOutput, error) {
	return nil, lookupOutput{Result: "result for: " + in.Query}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func toolNames(tools []*mcp.Tool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}
