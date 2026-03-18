// Copyright 2025 The MCP Variants Authors. All rights reserved.
// Use of this source code is governed by a Apache-2.0
// license that can be found in the LICENSE file.

package variants

import (
	"context"
	"fmt"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// Per-session types
// ---------------------------------------------------------------------------

// innerConnection holds the resources for communicating with one inner
// server. In stateful mode one is created per client session; in
// stateless mode a single instance is shared across all requests.
type innerConnection struct {
	backendSession *backendSession
	cleanupFn      func()
}

// close invokes the backend-specific cleanup function which tears down
// both the client and server sessions.
func (c *innerConnection) close() {
	if c.cleanupFn != nil {
		c.cleanupFn()
	}
}

// backendSession bypasses the in-memory transport and calls the inner
// server's handler chain directly, preserving the caller's
// context.Context values. It mirrors the mcp.ClientSession API so that
// dispatch code reads naturally.
type backendSession struct {
	variantID        string
	serverSession    *mcp.ServerSession
	mcpMethodHandler mcp.MethodHandler
}

func (s *backendSession) ListTools(ctx context.Context, p *mcp.ListToolsParams, extra *mcp.RequestExtra) (*mcp.ListToolsResult, error) {
	result, err := s.mcpMethodHandler(ctx, "tools/list", &mcp.ListToolsRequest{Session: s.serverSession, Params: p, Extra: extra})
	if err != nil {
		return nil, err
	}
	r, ok := result.(*mcp.ListToolsResult)
	if !ok && result != nil {
		return nil, fmt.Errorf("unexpected result type %T for tools/list", result)
	}
	return r, nil
}

func (s *backendSession) ListResources(ctx context.Context, p *mcp.ListResourcesParams, extra *mcp.RequestExtra) (*mcp.ListResourcesResult, error) {
	result, err := s.mcpMethodHandler(ctx, "resources/list", &mcp.ListResourcesRequest{Session: s.serverSession, Params: p, Extra: extra})
	if err != nil {
		return nil, err
	}
	r, ok := result.(*mcp.ListResourcesResult)
	if !ok && result != nil {
		return nil, fmt.Errorf("unexpected result type %T for resources/list", result)
	}
	return r, nil
}

func (s *backendSession) ListPrompts(ctx context.Context, p *mcp.ListPromptsParams, extra *mcp.RequestExtra) (*mcp.ListPromptsResult, error) {
	result, err := s.mcpMethodHandler(ctx, "prompts/list", &mcp.ListPromptsRequest{Session: s.serverSession, Params: p, Extra: extra})
	if err != nil {
		return nil, err
	}
	r, ok := result.(*mcp.ListPromptsResult)
	if !ok && result != nil {
		return nil, fmt.Errorf("unexpected result type %T for prompts/list", result)
	}
	return r, nil
}

func (s *backendSession) ListResourceTemplates(ctx context.Context, p *mcp.ListResourceTemplatesParams, extra *mcp.RequestExtra) (*mcp.ListResourceTemplatesResult, error) {
	result, err := s.mcpMethodHandler(ctx, "resources/templates/list", &mcp.ListResourceTemplatesRequest{Session: s.serverSession, Params: p, Extra: extra})
	if err != nil {
		return nil, err
	}
	r, ok := result.(*mcp.ListResourceTemplatesResult)
	if !ok && result != nil {
		return nil, fmt.Errorf("unexpected result type %T for resources/templates/list", result)
	}
	return r, nil
}

func (s *backendSession) CallTool(ctx context.Context, p *mcp.CallToolParamsRaw, extra *mcp.RequestExtra) (*mcp.CallToolResult, error) {
	result, err := s.mcpMethodHandler(ctx, "tools/call", &mcp.CallToolRequest{Session: s.serverSession, Params: p, Extra: extra})
	if err != nil {
		return nil, err
	}
	r, ok := result.(*mcp.CallToolResult)
	if !ok && result != nil {
		return nil, fmt.Errorf("unexpected result type %T for tools/call", result)
	}
	return r, nil
}

func (s *backendSession) ReadResource(ctx context.Context, p *mcp.ReadResourceParams, extra *mcp.RequestExtra) (*mcp.ReadResourceResult, error) {
	result, err := s.mcpMethodHandler(ctx, "resources/read", &mcp.ReadResourceRequest{Session: s.serverSession, Params: p, Extra: extra})
	if err != nil {
		return nil, err
	}
	r, ok := result.(*mcp.ReadResourceResult)
	if !ok && result != nil {
		return nil, fmt.Errorf("unexpected result type %T for resources/read", result)
	}
	return r, nil
}

func (s *backendSession) GetPrompt(ctx context.Context, p *mcp.GetPromptParams, extra *mcp.RequestExtra) (*mcp.GetPromptResult, error) {
	result, err := s.mcpMethodHandler(ctx, "prompts/get", &mcp.GetPromptRequest{Session: s.serverSession, Params: p, Extra: extra})
	if err != nil {
		return nil, err
	}
	r, ok := result.(*mcp.GetPromptResult)
	if !ok && result != nil {
		return nil, fmt.Errorf("unexpected result type %T for prompts/get", result)
	}
	return r, nil
}

func (s *backendSession) Subscribe(ctx context.Context, p *mcp.SubscribeParams, extra *mcp.RequestExtra) error {
	_, err := s.mcpMethodHandler(ctx, "resources/subscribe", &mcp.SubscribeRequest{Session: s.serverSession, Params: p, Extra: extra})
	return err
}

func (s *backendSession) Unsubscribe(ctx context.Context, p *mcp.UnsubscribeParams, extra *mcp.RequestExtra) error {
	_, err := s.mcpMethodHandler(ctx, "resources/unsubscribe", &mcp.UnsubscribeRequest{Session: s.serverSession, Params: p, Extra: extra})
	return err
}

func (s *backendSession) Complete(ctx context.Context, p *mcp.CompleteParams, extra *mcp.RequestExtra) (*mcp.CompleteResult, error) {
	result, err := s.mcpMethodHandler(ctx, "completion/complete", &mcp.CompleteRequest{Session: s.serverSession, Params: p, Extra: extra})
	if err != nil {
		return nil, err
	}
	r, ok := result.(*mcp.CompleteResult)
	if !ok && result != nil {
		return nil, fmt.Errorf("unexpected result type %T for completion/complete", result)
	}
	return r, nil
}

// sessionState holds all per-session state for one front client.
type sessionState struct {
	dispatcher *dispatcher
}

// close tears down all inner connections for this session.
func (ss *sessionState) close() {
	for _, c := range ss.dispatcher.connections {
		c.close()
	}
}

// injectVariantMeta sets the variant ID in a Params' _meta map,
// preserving any existing metadata.
func injectVariantMeta(p mcp.Params, variantID string) {
	meta := p.GetMeta()
	if meta == nil {
		meta = map[string]any{}
		p.SetMeta(meta)
	}
	meta[metaKeyVariant] = variantID
}

// ---------------------------------------------------------------------------
// Session lifecycle
// ---------------------------------------------------------------------------

// createSessionState sets up inner connections for all variants and returns
// the per-session state.
func (s *Server) createSessionState(ctx context.Context, frontSession *mcp.ServerSession) (*sessionState, error) {
	connections := make(map[string]*innerConnection, len(s.variants))

	for _, entry := range s.variants {
		conn, err := entry.backend.connect(ctx, entry.variant, frontSession)
		if err != nil {
			for _, c := range connections {
				c.close()
			}
			return nil, err
		}
		connections[entry.variant.ID] = conn
	}

	return &sessionState{
		dispatcher: &dispatcher{
			server:      s,
			connections: connections,
		},
	}, nil
}

// sessionMiddleware builds the receiving middleware that manages per-session
// state and delegates to the variant dispatcher.
//
// When shared is non-nil (stateless mode), all requests use the shared
// connections instead of creating per-session state.
func (s *Server) sessionMiddleware(sessions *sync.Map, shared *sessionState) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			ss := req.GetSession().(*mcp.ServerSession)

			if method == "initialize" {
				// Let the SDK handle init first (capability negotiation etc.)
				result, err := next(ctx, method, req)
				if err != nil {
					return nil, err
				}

				// In stateless mode, skip per-session connection creation;
				// requests will use the shared connections.
				if shared == nil {
					state, err := s.createSessionState(ctx, ss)
					if err != nil {
						return nil, err
					}
					sessions.Store(ss, state)

					// Clean up when the front session closes.
					go func() {
						ss.Wait()
						sessions.Delete(ss)
						state.close()
					}()
				}

				// Enrich the init result with variant information
				return s.enrichInitResult(ctx, result, req)
			}

			// Try per-session state first
			if v, ok := sessions.Load(ss); ok {
				state := v.(*sessionState)
				return state.dispatcher.handle(ctx, method, req, next)
			}

			// Fall back to shared state (stateless mode)
			if shared != nil {
				return shared.dispatcher.handle(ctx, method, req, next)
			}

			return next(ctx, method, req)
		}
	}
}
