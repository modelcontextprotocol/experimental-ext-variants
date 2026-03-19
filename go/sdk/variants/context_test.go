package variants

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contextKey string

const testCtxKey contextKey = "test-value"

type checkContextOutput struct {
	ContextValue string `json:"contextValue"`
	VariantID    string `json:"variantId"`
	HasExtra     bool   `json:"hasExtra"`
	CustomHeader string `json:"customHeader"`
}

type emptyInput struct{}

// headerRoundTripper injects custom headers into every outgoing HTTP request.
type headerRoundTripper struct {
	base   http.RoundTripper
	header http.Header
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	for k, v := range h.header {
		req.Header[k] = v
	}
	return h.base.RoundTrip(req)
}

func newContextTestInnerServer() *mcp.Server {
	inner := mcp.NewServer(&mcp.Implementation{Name: "ctx-test", Version: "v1.0.0"}, nil)

	mcp.AddTool(inner, &mcp.Tool{
		Name:        "check_context",
		Description: "Reports context and request metadata",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, checkContextOutput, error) {
		out := checkContextOutput{}

		if v, ok := ctx.Value(testCtxKey).(string); ok {
			out.ContextValue = v
		}

		if req.Extra != nil {
			out.HasExtra = true
			if req.Extra.Header != nil {
				out.CustomHeader = req.Extra.Header.Get("X-Test-Header")
			}
		}

		if req.Params != nil {
			meta := req.Params.GetMeta()
			if v, ok := meta[metaKeyVariant].(string); ok {
				out.VariantID = v
			}
		}

		return nil, out, nil
	})

	return inner
}

func parseCheckContextOutput(t *testing.T, result *mcp.CallToolResult) checkContextOutput {
	t.Helper()
	require.NotEmpty(t, result.Content)
	tc, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])

	var out checkContextOutput
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &out))
	return out
}

// TestContextPreservation verifies that context values set by middleware on
// the front (outer) mcp.Server and HTTP request headers are preserved through
// the variant dispatch layer and reach the inner server's tool handler.
func TestContextPreservation(t *testing.T) {
	vs := NewServer(&mcp.Implementation{Name: "ctx-test", Version: "v1.0.0"}).
		WithVariant(ServerVariant{
			ID:          "test-variant",
			Description: "Context test variant",
			Status:      Stable,
		}, newContextTestInnerServer(), 0)

	frontServer, err := vs.mcpServer(false)
	require.NoError(t, err)

	frontServer.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			ctx = context.WithValue(ctx, testCtxKey, "from-middleware")
			return next(ctx, method, req)
		}
	})

	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return frontServer },
		nil,
	)
	httpSrv := httptest.NewServer(handler)
	t.Cleanup(httpSrv.Close)

	client := mcp.NewClient(
		&mcp.Implementation{Name: "ctx-test-client", Version: "v0.0.1"},
		nil,
	)

	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: httpSrv.URL,
		HTTPClient: &http.Client{
			Transport: &headerRoundTripper{
				base: http.DefaultTransport,
				header: http.Header{
					"X-Test-Header": []string{"test-header-value"},
				},
			},
		},
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "check_context",
		Arguments: map[string]json.RawMessage{},
	})
	require.NoError(t, err)

	out := parseCheckContextOutput(t, result)

	assert.Equal(t, "from-middleware", out.ContextValue,
		"context value from front server middleware should be preserved through dispatch")
	assert.Equal(t, "test-variant", out.VariantID,
		"variant ID should be injected into request _meta")
	assert.True(t, out.HasExtra,
		"RequestExtra should be forwarded to inner tool handler")
	assert.Equal(t, "test-header-value", out.CustomHeader,
		"HTTP headers should be captured in RequestExtra and forwarded through dispatch")
}
