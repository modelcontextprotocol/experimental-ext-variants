// Copyright 2025 The MCP Variants Authors. All rights reserved.
// Use of this source code is governed by a Apache-2.0
// license that can be found in the LICENSE file.

package variants

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
			d := &dispatcher{
				server: NewServer(&mcp.Implementation{Name: "test", Version: "v0.0.1"}).
					WithVariant(ServerVariant{ID: variantID, Status: Stable}, mcp.NewServer(&mcp.Implementation{Name: "inner", Version: "v0.0.1"}, nil), 0),
				connections: map[string]*innerConnection{
					variantID: {
						backendSession: &backendSession{
							variantID:        variantID,
							mcpMethodHandler: tt.handler,
						},
					},
				},
			}

			req := &mcp.ServerRequest[*mcp.ListToolsParams]{
				Params: &mcp.ListToolsParams{
					Meta: mcp.Meta{metaKeyVariant: variantID},
				},
			}

			result, err := d.handleList(context.Background(), "tools/list", req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != nil {
				t.Fatalf("expected nil result, got %v", result)
			}
		})
	}
}
