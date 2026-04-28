// Copyright 2025 The MCP Variants Authors. All rights reserved.
// Use of this source code is governed by a Apache-2.0
// license that can be found in the LICENSE file.

package variants

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// isNilInterface
// ---------------------------------------------------------------------------

func TestIsNilInterface(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want bool
	}{
		{"untyped nil", nil, true},
		{"typed nil pointer", (*mcp.ListToolsParams)(nil), true},
		{"non-nil pointer", &mcp.ListToolsParams{}, false},
		{"non-pointer value", 42, false},
		{"string value", "hello", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isNilInterface(tt.val))
		})
	}
}

// ---------------------------------------------------------------------------
// handleList
// ---------------------------------------------------------------------------

// newTestDispatcher builds a dispatcher with a single variant backed by a
// custom mcpMethodHandler. The handler receives the request exactly as the
// dispatcher sends it, so tests can inspect params/session mutations.
func newTestDispatcher(variantID string, handler mcp.MethodHandler) *dispatcher {
	return &dispatcher{
		server: NewServer(&mcp.Implementation{Name: "test", Version: "v0.0.1"}).
			WithVariant(ServerVariant{ID: variantID, Status: Stable}, mcp.NewServer(&mcp.Implementation{Name: "inner", Version: "v0.0.1"}, nil), 0),
		connections: map[string]*innerConnection{
			variantID: {
				backendSession: &backendSession{
					variantID:        variantID,
					mcpMethodHandler: handler,
				},
			},
		},
	}
}

func TestHandleList_NilResult(t *testing.T) {
	const variantID = "test-variant"

	tests := []struct {
		name    string
		handler func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error)
	}{
		{
			name: "untyped nil",
			handler: func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				return nil, nil
			},
		},
		{
			name: "typed nil",
			handler: func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				return (*mcp.ListToolsResult)(nil), nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDispatcher(variantID, tt.handler)
			req := &mcp.ListToolsRequest{
				Params: &mcp.ListToolsParams{
					Meta: mcp.Meta{metaKeyVariant: variantID},
				},
			}

			result, err := d.handleList(context.Background(), "tools/list", req)
			require.NoError(t, err)
			assert.Nil(t, result)
		})
	}
}

func TestHandleList_CursorRoundTrip(t *testing.T) {
	const variantID = "v1"
	const innerCursor = "page-2-token"

	tests := []struct {
		name   string
		method string
		req    func(wrappedCursor string) mcp.Request
		result func() mcp.Result // result returned by backend with NextCursor set
	}{
		{
			name:   "tools/list",
			method: "tools/list",
			req: func(c string) mcp.Request {
				return &mcp.ListToolsRequest{
					Params: &mcp.ListToolsParams{
						Meta:   mcp.Meta{metaKeyVariant: variantID},
						Cursor: c,
					},
				}
			},
			result: func() mcp.Result {
				return &mcp.ListToolsResult{NextCursor: "next-inner"}
			},
		},
		{
			name:   "resources/list",
			method: "resources/list",
			req: func(c string) mcp.Request {
				return &mcp.ListResourcesRequest{
					Params: &mcp.ListResourcesParams{
						Meta:   mcp.Meta{metaKeyVariant: variantID},
						Cursor: c,
					},
				}
			},
			result: func() mcp.Result {
				return &mcp.ListResourcesResult{NextCursor: "next-inner"}
			},
		},
		{
			name:   "prompts/list",
			method: "prompts/list",
			req: func(c string) mcp.Request {
				return &mcp.ListPromptsRequest{
					Params: &mcp.ListPromptsParams{
						Meta:   mcp.Meta{metaKeyVariant: variantID},
						Cursor: c,
					},
				}
			},
			result: func() mcp.Result {
				return &mcp.ListPromptsResult{NextCursor: "next-inner"}
			},
		},
		{
			name:   "resources/templates/list",
			method: "resources/templates/list",
			req: func(c string) mcp.Request {
				return &mcp.ListResourceTemplatesRequest{
					Params: &mcp.ListResourceTemplatesParams{
						Meta:   mcp.Meta{metaKeyVariant: variantID},
						Cursor: c,
					},
				}
			},
			result: func() mcp.Result {
				return &mcp.ListResourceTemplatesResult{NextCursor: "next-inner"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrappedCursor := wrapCursor(innerCursor, variantID)
			var receivedCursor string

			d := newTestDispatcher(variantID, func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				// Capture what cursor the backend actually receives.
				params := req.GetParams()
				f := reflect.ValueOf(params).Elem().FieldByName("Cursor")
				receivedCursor = f.String()
				return tt.result(), nil
			})

			result, err := d.handleList(context.Background(), tt.method, tt.req(wrappedCursor))
			require.NoError(t, err)

			// Backend should have received the inner (unwrapped) cursor.
			assert.Equal(t, innerCursor, receivedCursor, "cursor should be unwrapped before reaching backend")

			// Result's NextCursor should now be wrapped.
			nextCursor := reflect.ValueOf(result).Elem().FieldByName("NextCursor").String()
			unwrapped, err := unwrapCursor(nextCursor, variantID)
			require.NoError(t, err)
			assert.Equal(t, "next-inner", unwrapped, "NextCursor should be wrapped with variant ID")
		})
	}
}

func TestHandleList_NoCursor(t *testing.T) {
	const variantID = "v1"

	d := newTestDispatcher(variantID, func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		return &mcp.ListToolsResult{}, nil
	})

	req := &mcp.ListToolsRequest{
		Params: &mcp.ListToolsParams{
			Meta: mcp.Meta{metaKeyVariant: variantID},
		},
	}

	result, err := d.handleList(context.Background(), "tools/list", req)
	require.NoError(t, err)

	// No NextCursor should be set when backend returns empty cursor.
	r := result.(*mcp.ListToolsResult)
	assert.Empty(t, r.NextCursor)
}

func TestHandleList_InvalidCursor(t *testing.T) {
	const variantID = "v1"

	d := newTestDispatcher(variantID, func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		t.Fatal("backend should not be called with invalid cursor")
		return nil, nil
	})

	req := &mcp.ListToolsRequest{
		Params: &mcp.ListToolsParams{
			Meta:   mcp.Meta{metaKeyVariant: variantID},
			Cursor: "not-valid-base64!@#$",
		},
	}

	_, err := d.handleList(context.Background(), "tools/list", req)
	require.Error(t, err)

	var jErr *jsonrpc.Error
	require.True(t, errors.As(err, &jErr))
	assert.EqualValues(t, jsonrpc.CodeInvalidParams, jErr.Code)
}

func TestHandleList_CrossVariantCursor(t *testing.T) {
	const variantID = "v1"
	otherVariantCursor := wrapCursor("page2", "other-variant")

	d := newTestDispatcher(variantID, func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		t.Fatal("backend should not be called with cross-variant cursor")
		return nil, nil
	})

	req := &mcp.ListToolsRequest{
		Params: &mcp.ListToolsParams{
			Meta:   mcp.Meta{metaKeyVariant: variantID},
			Cursor: otherVariantCursor,
		},
	}

	_, err := d.handleList(context.Background(), "tools/list", req)
	require.Error(t, err)

	var jErr *jsonrpc.Error
	require.True(t, errors.As(err, &jErr))
	assert.EqualValues(t, jsonrpc.CodeInvalidParams, jErr.Code)
	assert.Contains(t, jErr.Message, "Cursor invalid for requested variant")
}

func TestHandleList_ErrorEnrichment(t *testing.T) {
	const variantID = "v1"

	backendErr := &jsonrpc.Error{
		Code:    jsonrpc.CodeInvalidParams,
		Message: "unknown tool",
	}
	d := newTestDispatcher(variantID, func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		return nil, backendErr
	})

	req := &mcp.ListToolsRequest{
		Params: &mcp.ListToolsParams{
			Meta: mcp.Meta{metaKeyVariant: variantID},
		},
	}

	_, err := d.handleList(context.Background(), "tools/list", req)
	require.Error(t, err)

	var jErr *jsonrpc.Error
	require.True(t, errors.As(err, &jErr))
	assert.Contains(t, string(jErr.Data), `"activeVariant"`)
	assert.Contains(t, string(jErr.Data), variantID)
}

func TestHandleList_NilParams(t *testing.T) {
	const variantID = "v1"
	called := false

	d := newTestDispatcher(variantID, func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		called = true
		return &mcp.ListToolsResult{}, nil
	})

	// Typed-nil params — the SDK produces these for parameterless requests.
	req := &mcp.ListToolsRequest{
		Params: (*mcp.ListToolsParams)(nil),
	}

	_, err := d.handleList(context.Background(), "tools/list", req)
	require.NoError(t, err)
	assert.True(t, called, "backend should still be called with nil params")
}

// ---------------------------------------------------------------------------
// handleDirect
// ---------------------------------------------------------------------------

func TestHandleDirect_MethodRouting(t *testing.T) {
	const variantID = "v1"

	tests := []struct {
		name   string
		method string
		req    mcp.Request
	}{
		{
			name:   "tools/call",
			method: "tools/call",
			req: &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Meta: mcp.Meta{metaKeyVariant: variantID},
					Name: "my-tool",
				},
			},
		},
		{
			name:   "resources/read",
			method: "resources/read",
			req: &mcp.ReadResourceRequest{
				Params: &mcp.ReadResourceParams{
					Meta: mcp.Meta{metaKeyVariant: variantID},
					URI:  "file:///test",
				},
			},
		},
		{
			name:   "prompts/get",
			method: "prompts/get",
			req: &mcp.GetPromptRequest{
				Params: &mcp.GetPromptParams{
					Meta: mcp.Meta{metaKeyVariant: variantID},
					Name: "my-prompt",
				},
			},
		},
		{
			name:   "resources/subscribe",
			method: "resources/subscribe",
			req: &mcp.SubscribeRequest{
				Params: &mcp.SubscribeParams{
					Meta: mcp.Meta{metaKeyVariant: variantID},
					URI:  "file:///watch",
				},
			},
		},
		{
			name:   "resources/unsubscribe",
			method: "resources/unsubscribe",
			req: &mcp.UnsubscribeRequest{
				Params: &mcp.UnsubscribeParams{
					Meta: mcp.Meta{metaKeyVariant: variantID},
					URI:  "file:///watch",
				},
			},
		},
		{
			name:   "completion/complete",
			method: "completion/complete",
			req: &mcp.CompleteRequest{
				Params: &mcp.CompleteParams{
					Meta: mcp.Meta{metaKeyVariant: variantID},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedMethod string
			d := newTestDispatcher(variantID, func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				receivedMethod = method
				return nil, nil
			})

			_, err := d.handleDirect(context.Background(), tt.method, tt.req)
			require.NoError(t, err)
			assert.Equal(t, tt.method, receivedMethod, "backend should receive the correct method")
		})
	}
}

func TestHandleDirect_MetaInjection(t *testing.T) {
	const variantID = "v1"

	var receivedMeta mcp.Meta
	d := newTestDispatcher(variantID, func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		receivedMeta = req.GetParams().GetMeta()
		return &mcp.CallToolResult{}, nil
	})

	// Start without variant meta — handleDirect should inject it.
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "my-tool",
		},
	}

	_, err := d.handleDirect(context.Background(), "tools/call", req)
	require.NoError(t, err)
	assert.Equal(t, variantID, receivedMeta[metaKeyVariant], "variant meta should be injected into params")
}

func TestHandleDirect_ErrorEnrichment(t *testing.T) {
	const variantID = "v1"

	d := newTestDispatcher(variantID, func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		return nil, &jsonrpc.Error{
			Code:    jsonrpc.CodeInvalidParams,
			Message: "tool not found",
		}
	})

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Meta: mcp.Meta{metaKeyVariant: variantID},
			Name: "nonexistent",
		},
	}

	_, err := d.handleDirect(context.Background(), "tools/call", req)
	require.Error(t, err)

	var jErr *jsonrpc.Error
	require.True(t, errors.As(err, &jErr))
	assert.Contains(t, string(jErr.Data), `"activeVariant"`)
	assert.Contains(t, string(jErr.Data), variantID)
}

func TestHandleDirect_NonEnrichableError(t *testing.T) {
	const variantID = "v1"
	plainErr := errors.New("internal error")

	d := newTestDispatcher(variantID, func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		return nil, plainErr
	})

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Meta: mcp.Meta{metaKeyVariant: variantID},
			Name: "my-tool",
		},
	}

	_, err := d.handleDirect(context.Background(), "tools/call", req)
	assert.Equal(t, plainErr, err, "non-jsonrpc errors should pass through unmodified")
}

func TestHandleDirect_NilParams(t *testing.T) {
	const variantID = "v1"
	called := false

	d := newTestDispatcher(variantID, func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		called = true
		return nil, nil
	})

	req := &mcp.SubscribeRequest{
		Params: (*mcp.SubscribeParams)(nil),
	}

	_, err := d.handleDirect(context.Background(), "resources/subscribe", req)
	require.NoError(t, err)
	assert.True(t, called, "backend should still be called when params are typed-nil")
}

// ---------------------------------------------------------------------------
// handle (top-level router)
// ---------------------------------------------------------------------------

func TestHandle_UnknownMethodPassthrough(t *testing.T) {
	const variantID = "v1"

	d := newTestDispatcher(variantID, func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		t.Fatal("dispatcher handler should not be called for unknown methods")
		return nil, nil
	})

	nextCalled := false
	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		nextCalled = true
		return nil, nil
	}

	req := &mcp.ListToolsRequest{
		Params: &mcp.ListToolsParams{},
	}

	_, err := d.handle(context.Background(), "custom/method", req, next)
	require.NoError(t, err)
	assert.True(t, nextCalled, "unknown methods should fall through to next")
}

func TestHandle_RoutesToCorrectHandler(t *testing.T) {
	const variantID = "v1"

	tests := []struct {
		name   string
		method string
		req    mcp.Request
	}{
		{"list routes to handleList", "tools/list", &mcp.ListToolsRequest{
			Params: &mcp.ListToolsParams{Meta: mcp.Meta{metaKeyVariant: variantID}},
		}},
		{"call routes to handleDirect", "tools/call", &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{metaKeyVariant: variantID}, Name: "t"},
		}},
		{"subscribe routes to handleDirect", "resources/subscribe", &mcp.SubscribeRequest{
			Params: &mcp.SubscribeParams{Meta: mcp.Meta{metaKeyVariant: variantID}, URI: "u"},
		}},
		{"unsubscribe routes to handleDirect", "resources/unsubscribe", &mcp.UnsubscribeRequest{
			Params: &mcp.UnsubscribeParams{Meta: mcp.Meta{metaKeyVariant: variantID}, URI: "u"},
		}},
		{"complete routes to handleDirect", "completion/complete", &mcp.CompleteRequest{
			Params: &mcp.CompleteParams{Meta: mcp.Meta{metaKeyVariant: variantID}},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedMethod string
			d := newTestDispatcher(variantID, func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				receivedMethod = method
				if method == "tools/list" {
					return &mcp.ListToolsResult{}, nil
				}
				return nil, nil
			})

			next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				t.Fatalf("next should not be called for method %s", method)
				return nil, nil
			}

			_, err := d.handle(context.Background(), tt.method, tt.req, next)
			require.NoError(t, err)
			assert.Equal(t, tt.method, receivedMethod)
		})
	}
}

// ---------------------------------------------------------------------------
// handleReceive (session.go)
// ---------------------------------------------------------------------------

func TestHandleReceive_SessionInjection(t *testing.T) {
	originalSession := &mcp.ServerSession{}
	targetSession := &mcp.ServerSession{}

	var receivedSession *mcp.ServerSession
	bs := &backendSession{
		variantID:     "v1",
		serverSession: targetSession,
		mcpMethodHandler: func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			receivedSession = req.GetSession().(*mcp.ServerSession)
			return &mcp.ListToolsResult{}, nil
		},
	}

	req := &mcp.ListToolsRequest{
		Session: originalSession,
		Params:  &mcp.ListToolsParams{},
	}

	_, err := bs.handleReceive(context.Background(), "tools/list", req)
	require.NoError(t, err)
	assert.Same(t, targetSession, receivedSession, "handleReceive should replace Session with inner server session")
	assert.Same(t, originalSession, req.Session, "handleReceive should not mutate the original request")
}

// ---------------------------------------------------------------------------
// getConnection
// ---------------------------------------------------------------------------

func TestGetConnection_InvalidVariant(t *testing.T) {
	d := newTestDispatcher("v1", nil)

	req := &mcp.ListToolsRequest{
		Params: &mcp.ListToolsParams{
			Meta: mcp.Meta{metaKeyVariant: "nonexistent"},
		},
	}

	_, err := d.getConnection(context.Background(), req)
	require.Error(t, err)

	var jErr *jsonrpc.Error
	require.True(t, errors.As(err, &jErr))
	assert.EqualValues(t, jsonrpc.CodeInvalidParams, jErr.Code)
	assert.Contains(t, jErr.Message, "Invalid server variant")
	assert.Contains(t, string(jErr.Data), "nonexistent")
}

func TestGetConnection_DefaultVariant(t *testing.T) {
	d := newTestDispatcher("v1", nil)

	// No variant in meta — should fall back to default (first-ranked).
	req := &mcp.ListToolsRequest{
		Params: &mcp.ListToolsParams{},
	}

	conn, err := d.getConnection(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "v1", conn.backendSession.variantID)
}

// ---------------------------------------------------------------------------
// enrichError
// ---------------------------------------------------------------------------

func TestEnrichError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantEnrich bool
	}{
		{
			name: "InvalidParams enriched",
			err: &jsonrpc.Error{
				Code:    jsonrpc.CodeInvalidParams,
				Message: "bad param",
			},
			wantEnrich: true,
		},
		{
			name: "MethodNotFound enriched",
			err: &jsonrpc.Error{
				Code:    jsonrpc.CodeMethodNotFound,
				Message: "no method",
			},
			wantEnrich: true,
		},
		{
			name: "InternalError not enriched",
			err: &jsonrpc.Error{
				Code:    jsonrpc.CodeInternalError,
				Message: "server error",
			},
			wantEnrich: false,
		},
		{
			name:       "plain error not enriched",
			err:        errors.New("some error"),
			wantEnrich: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := enrichError(tt.err, "v1")

			var jErr *jsonrpc.Error
			if errors.As(result, &jErr) && tt.wantEnrich {
				assert.Contains(t, string(jErr.Data), `"activeVariant"`)
				assert.Contains(t, string(jErr.Data), `"v1"`)
			} else if !tt.wantEnrich {
				if errors.As(result, &jErr) {
					// Should NOT have activeVariant
					assert.NotContains(t, string(jErr.Data), `"activeVariant"`)
				}
			}
		})
	}
}
