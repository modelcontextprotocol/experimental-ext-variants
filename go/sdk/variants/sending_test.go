// Copyright 2025 The MCP Variants Authors. All rights reserved.
// Use of this source code is governed by a Apache-2.0
// license that can be found in the LICENSE file.

package variants

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sendingInput is the tool input for the sending-methods test tool.
type sendingInput struct {
	// Which server-to-client methods to call: "elicit", "createMessage", "listRoots".
	Methods []string `json:"methods"`
}

// sendingResult is returned by the tool with results from each method call.
type sendingResult struct {
	ElicitAction  string         `json:"elicitAction,omitempty"`
	ElicitContent map[string]any `json:"elicitContent,omitempty"`
	SamplingModel string         `json:"samplingModel,omitempty"`
	SamplingText  string         `json:"samplingText,omitempty"`
	RootURIs      []string       `json:"rootURIs,omitempty"`
}

// sendingHandler calls server-to-client methods via req.Session and returns
// the results so the test can verify they were forwarded through the variant layer.
func sendingHandler(ctx context.Context, req *mcp.CallToolRequest, input sendingInput) (*mcp.CallToolResult, sendingResult, error) {
	ss := req.Session
	var out sendingResult

	for _, m := range input.Methods {
		switch m {
		case "elicit":
			result, err := ss.Elicit(ctx, &mcp.ElicitParams{
				Message: "Please confirm",
			})
			if err != nil {
				return nil, out, err
			}
			out.ElicitAction = result.Action
			out.ElicitContent = result.Content

		case "createMessage":
			result, err := ss.CreateMessage(ctx, &mcp.CreateMessageParams{
				MaxTokens: 100,
				Messages: []*mcp.SamplingMessage{
					{Role: "user", Content: &mcp.TextContent{Text: "Hello"}},
				},
			})
			if err != nil {
				return nil, out, err
			}
			out.SamplingModel = result.Model
			if tc, ok := result.Content.(*mcp.TextContent); ok {
				out.SamplingText = tc.Text
			}

		case "listRoots":
			result, err := ss.ListRoots(ctx, &mcp.ListRootsParams{})
			if err != nil {
				return nil, out, err
			}
			for _, r := range result.Roots {
				out.RootURIs = append(out.RootURIs, r.URI)
			}
		}
	}

	return nil, out, nil
}

func newSendingVariantServer() *Server {
	inner := mcp.NewServer(
		&mcp.Implementation{Name: "sending-test", Version: "v1.0.0"},
		nil,
	)
	mcp.AddTool(inner, &mcp.Tool{
		Name:        "send",
		Description: "Calls server-to-client methods and returns results",
	}, sendingHandler)

	return NewServer(&mcp.Implementation{Name: "sending-test", Version: "v1.0.0"}).
		WithVariant(ServerVariant{
			ID:          "default",
			Description: "Sending methods test variant",
			Status:      Stable,
		}, inner, 0)
}

// TestSendingMethodsForwarding verifies that server-to-client request methods
// (Elicit, CreateMessage, ListRoots) are correctly forwarded through the
// variant layer's sending redirect middleware to the real client.
//
// Stateless mode is excluded: the StreamableHTTP transport rejects
// server-to-client requests in stateless mode ("stateless servers cannot
// make requests").
func TestSendingMethodsForwarding(t *testing.T) {
	vs := newSendingVariantServer()
	ctx := context.Background()

	handler := NewStreamableHTTPHandler(vs, nil)
	httpSrv := httptest.NewServer(handler)
	t.Cleanup(httpSrv.Close)

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "v0.0.1"},
		&mcp.ClientOptions{
			CreateMessageHandler: func(_ context.Context, req *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
				return &mcp.CreateMessageResult{
					Model:   "test-model",
					Role:    "assistant",
					Content: &mcp.TextContent{Text: "mock response"},
				}, nil
			},
			ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{
					Action:  "accept",
					Content: map[string]any{"confirmed": true},
				}, nil
			},
		},
	)

	client.AddRoots(
		&mcp.Root{URI: "file:///workspace/project-a", Name: "Project A"},
		&mcp.Root{URI: "file:///workspace/project-b", Name: "Project B"},
	)

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: httpSrv.URL,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { session.Close() })

	t.Run("Elicit", func(t *testing.T) {
		result := callSend(t, ctx, session, "elicit")
		assert.Equal(t, "accept", result.ElicitAction)
		assert.Equal(t, true, result.ElicitContent["confirmed"])
	})

	t.Run("CreateMessage", func(t *testing.T) {
		result := callSend(t, ctx, session, "createMessage")
		assert.Equal(t, "test-model", result.SamplingModel)
		assert.Equal(t, "mock response", result.SamplingText)
	})

	t.Run("ListRoots", func(t *testing.T) {
		result := callSend(t, ctx, session, "listRoots")
		assert.Equal(t, []string{
			"file:///workspace/project-a",
			"file:///workspace/project-b",
		}, result.RootURIs)
	})

	t.Run("AllMethods", func(t *testing.T) {
		result := callSend(t, ctx, session, "elicit", "createMessage", "listRoots")
		assert.Equal(t, "accept", result.ElicitAction)
		assert.Equal(t, "test-model", result.SamplingModel)
		assert.Equal(t, "mock response", result.SamplingText)
		assert.Len(t, result.RootURIs, 2)
	})
}

// callSend invokes the "send" tool with the given methods and returns
// the parsed result.
func callSend(t *testing.T, ctx context.Context, session *mcp.ClientSession, methods ...string) sendingResult {
	t.Helper()

	methodsJSON, err := json.Marshal(methods)
	require.NoError(t, err)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "send",
		Arguments: map[string]json.RawMessage{
			"methods": json.RawMessage(methodsJSON),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	var out sendingResult
	require.NoError(t, unmarshalToolResult(result, &out))
	return out
}

// unmarshalToolResult extracts the JSON content from a CallToolResult.
func unmarshalToolResult(result *mcp.CallToolResult, v any) error {
	if len(result.Content) == 0 {
		return nil
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return nil
	}
	return json.Unmarshal([]byte(tc.Text), v)
}
