// Copyright 2025 The MCP Variants Authors. All rights reserved.
// Use of this source code is governed by a Apache-2.0
// license that can be found in the LICENSE file.

package variants

import (
	"context"
	"errors"
	"reflect"
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

// handleReceive invokes mcpMethodHandler for any MCP method by modifying the request's
// Session field to point to the inner server session. This replaces the explicit
// per-method functions (ListTools, CallTool, etc.) with a single generic handler.
//
// The dispatcher is responsible for modifying params (metadata injection,
// cursor unwrapping) before calling this method.
func (s *backendSession) handleReceive(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
	// Shallow-copy the concrete request struct so we don't mutate the caller's object
	// while replacing the Session field with our inner server session.
	// We can't use a wrapper (like the sending side's sessionSwappedRequest) because
	// the SDK's receiving handler does type assertions on the concrete request type.
	reqVal := reflect.ValueOf(req)
	if reqVal.Kind() != reflect.Ptr {
		return nil, errors.New("variants: expected pointer to request struct")
	}

	// Allocate a new struct of the same concrete type and shallow-copy all fields.
	copyPtr := reflect.New(reqVal.Elem().Type())
	copyPtr.Elem().Set(reqVal.Elem())

	// Set Session on the copy to point to the inner server session.
	sessionField := copyPtr.Elem().FieldByName("Session")
	if !sessionField.IsValid() || !sessionField.CanSet() {
		return nil, errors.New("variants: request type missing settable Session field")
	}
	sessionField.Set(reflect.ValueOf(s.serverSession))

	return s.mcpMethodHandler(ctx, method, copyPtr.Interface().(mcp.Request))
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

// frontSessionKeyType is the context key for the front-facing ServerSession.
type frontSessionKeyType struct{}

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
