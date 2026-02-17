// Copyright 2025 The MCP Variants Authors. All rights reserved.
// Use of this source code is governed by a Apache-2.0
// license that can be found in the LICENSE file.

package variants

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// variantEntry binds a ServerVariant to its backing backend.
type variantEntry struct {
	variant ServerVariant
	backend backend
}

// Server is a variant-aware MCP server that multiplexes across multiple
// mcp.Server instances, each associated with a ServerVariant. During
// initialization, clients provide VariantHints and the server responds with a
// ranked list of available variants. Per-request variant selection is carried
// in the _meta field.
//
// Server holds only configuration and is safe for concurrent use. In stateful
// mode (the default), per-session inner connections are created during
// initialize and scoped to the front session's lifetime. In stateless mode
// (via [NewStreamableHTTPHandler] with Stateless option), a single set of
// shared connections is created at construction and reused across all requests.
type Server struct {
	impl        *mcp.Implementation
	variants    []variantEntry
	rankingFunc RankingFunc
	shared      *sessionState // non-nil in stateless mode; cleaned up by Close
}

// NewServer creates a new variant-aware server with no registered variants.
//
// The first argument must not be nil.
func NewServer(impl *mcp.Implementation) *Server {
	if impl == nil {
		panic("variants: nil Implementation")
	}
	return &Server{
		impl: impl,
	}
}

// NewStreamableHTTPHandler returns a new [mcp.StreamableHTTPHandler] for
// serving multiple concurrent clients over HTTP. It mirrors
// [mcp.NewStreamableHTTPHandler].
//
//	handler := variants.NewStreamableHTTPHandler(vs, nil)
//	http.ListenAndServe(":8080", handler)
func NewStreamableHTTPHandler(vs *Server, opts *mcp.StreamableHTTPOptions) *mcp.StreamableHTTPHandler {
	if vs == nil {
		panic("variants: nil Server")
	}
	stateless := opts != nil && opts.Stateless
	srv, err := vs.mcpServer(stateless)
	if err != nil {
		panic("variants: " + err.Error())
	}
	return mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return srv },
		opts,
	)
}

// addVariant is the shared registration logic for all With* methods.
// It checks for duplicates, sets priority, and appends the entry.
func (s *Server) addVariant(v ServerVariant, b backend, priority int) *Server {
	for _, e := range s.variants {
		if e.variant.ID == v.ID {
			panic("variants: duplicate variant ID: " + v.ID)
		}
	}
	v.priority = priority
	s.variants = append(s.variants, variantEntry{variant: v, backend: b})
	return s
}

// WithVariant registers a ServerVariant backed by the given mcp.Server.
// priority determines the default ordering when no RankingFunc is set;
// lower values indicate higher importance (0 = highest priority). By
// default, the variant with the lowest priority value will appear first
// in the list and serve as the recommended default for clients. This
// behavior can be overridden by providing a custom RankingFunc.
//
// Variant IDs must be unique; registering a duplicate panics.
func (s *Server) WithVariant(v ServerVariant, mcpServer *mcp.Server, priority int) *Server {
	return s.addVariant(v, &inMemoryBackend{server: mcpServer}, priority)
}

// WithHTTPVariant registers a ServerVariant backed by an mcp.Server exposed
// over HTTP. Not yet implemented.
func (s *Server) WithHTTPVariant(v ServerVariant, mcpServer *mcp.Server, priority int) *Server {
	panic("variants: WithHTTPVariant not yet implemented")
}

// WithRemoteVariant registers a ServerVariant backed by a remote MCP server
// at the given endpoint URL. Not yet implemented.
func (s *Server) WithRemoteVariant(v ServerVariant, endpoint string, priority int) *Server {
	panic("variants: WithRemoteVariant not yet implemented")
}

// WithRanking sets a custom ranking function used to order variants based
// on client hints during initialization. The function should return variants
// sorted by relevance, with the most appropriate variant first. If nil,
// variants are ordered by their priority value (lowest first).
//
// Returns the receiver for chaining.
func (s *Server) WithRanking(fn RankingFunc) *Server {
	s.rankingFunc = fn
	return s
}

// Variants returns a copy of all registered ServerVariant values in
// registration order.
func (s *Server) Variants() []ServerVariant {
	out := make([]ServerVariant, len(s.variants))
	for i, e := range s.variants {
		out[i] = e.variant
	}
	return out
}

// RankedVariants returns the registered variants ranked according to the
// configured RankingFunc (or the default priority-based ranking if none is
// set).
func (s *Server) RankedVariants(ctx context.Context, hints VariantHints) []ServerVariant {
	all := s.Variants()
	if len(all) == 0 {
		return all
	}
	rankFn := s.rankingFunc
	if rankFn == nil {
		rankFn = defaultRankingFunc
	}
	return rankFn(ctx, hints, all)
}

// Close releases resources held by all registered backends and, in stateless
// mode, tears down the shared inner connections.
func (s *Server) Close() error {
	if s.shared != nil {
		s.shared.close()
		s.shared = nil
	}
	var firstErr error
	for _, entry := range s.variants {
		if err := entry.backend.close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Run starts the variant server on the given transport (e.g., stdio).
// For multi-client HTTP support, use [NewStreamableHTTPHandler] instead.
func (s *Server) Run(ctx context.Context, t mcp.Transport) error {
	defer s.Close()
	srv, err := s.mcpServer(false)
	if err != nil {
		return err
	}
	return srv.Run(ctx, t)
}

// discoverCapabilities probes each backend to determine its advertised
// capabilities. The results are merged into a single set for the front
// proxy server.
func (s *Server) discoverCapabilities() (*mcp.ServerCapabilities, error) {
	ctx := context.Background()
	var allCaps []*mcp.ServerCapabilities

	for _, entry := range s.variants {
		caps, err := entry.backend.capabilities(ctx)
		if err != nil {
			return nil, err
		}
		if caps != nil {
			allCaps = append(allCaps, caps)
		}
	}

	return unionCapabilities(allCaps), nil
}

// mcpServer returns a configured *mcp.Server that routes requests to the
// appropriate inner variant server based on the _meta variant field.
//
// The returned server is a thin proxy: it handles initialization (injecting
// variant metadata into the response) and delegates all other requests to the
// backing inner servers via dispatchers.
//
// Request flow (stateful mode):
//
//	Client ── transport ──▸ frontServer ──▸ sessionMiddleware
//	                                           │
//	                              ┌────────────┼────────────┐
//	                              │            │            │
//	                        initialize    route by     pass through
//	                        (create        _meta       (ping, etc.)
//	                         per-session   variant
//	                         connections)     │
//	                                          ▼
//	                                     dispatcher
//	                                          │
//	                              ┌───────────┴───────────┐
//	                              ▼                       ▼
//	                       innerConnection         innerConnection
//	                       (variant "A")           (variant "B")
//	                              │                       │
//	                          backend                 backend
//	                         (in-memory,             (in-memory,
//	                           HTTP, or                HTTP, or
//	                           Remote)                 remote)
//
// Each variant's [backend] determines how the inner connection is
// established. See [inMemoryBackend] for the co-located case.
//
// In stateless mode (see [NewStreamableHTTPHandler]), the inner connections
// are created once and shared across all requests instead of per-session.
func (s *Server) mcpServer(stateless bool) (*mcp.Server, error) {
	if len(s.variants) == 0 {
		return nil, errors.New("variants: no variants registered")
	}

	caps, err := s.discoverCapabilities()
	if err != nil {
		return nil, err
	}

	// Per-session state, keyed by *mcp.ServerSession pointer identity.
	sessions := &sync.Map{}

	// In stateless mode, create shared connections once and reuse them
	// across all requests (no per-session state). The shared state is
	// stored on the Server so Close() can release it.
	var shared *sessionState
	if stateless {
		shared, err = s.createSessionState(context.Background(), nil)
		if err != nil {
			return nil, err
		}
		s.shared = shared
	}

	frontServer := mcp.NewServer(s.impl, &mcp.ServerOptions{
		Capabilities: caps,
	})

	frontServer.AddReceivingMiddleware(s.sessionMiddleware(sessions, shared))

	return frontServer, nil
}

// ---------------------------------------------------------------------------
// Initialize helpers
// ---------------------------------------------------------------------------

// extractVariantHints extracts client-provided variant hints from the
// initialize request's extension payload. Per SEP-2053, the client sends:
//
//	experimental["io.modelcontextprotocol/server-variants"]["variantHints"]
func extractVariantHints(req mcp.Request) VariantHints {
	params, _ := req.GetParams().(*mcp.InitializeParams)
	if params == nil || params.Capabilities == nil || params.Capabilities.Experimental == nil {
		return VariantHints{}
	}
	ext, ok := params.Capabilities.Experimental[extensionID]
	if !ok {
		return VariantHints{}
	}
	extMap, _ := ext.(map[string]any)
	vh, _ := extMap["variantHints"].(map[string]any)
	if vh == nil {
		return VariantHints{}
	}
	var hints VariantHints
	hints.Description, _ = vh["description"].(string)
	if h, ok := vh["hints"].(map[string]any); ok {
		hints.Hints = h
	}
	return hints
}

// enrichInitResult injects variant information into the initialize response.
func (s *Server) enrichInitResult(ctx context.Context, result mcp.Result, req mcp.Request) (mcp.Result, error) {
	initResult, ok := result.(*mcp.InitializeResult)
	if !ok {
		return result, nil
	}

	ranked := s.RankedVariants(ctx, extractVariantHints(req))

	// Build availableVariants payload
	availableVariants := make([]map[string]any, len(ranked))
	for i, v := range ranked {
		variant := map[string]any{
			"id":          v.ID,
			"description": v.Description,
		}
		if v.Hints != nil {
			variant["hints"] = v.Hints
		}
		if v.Status != "" {
			variant["status"] = v.Status
		}
		if v.DeprecationInfo != nil {
			variant["deprecationInfo"] = v.DeprecationInfo
		}
		availableVariants[i] = variant
	}

	if initResult.Capabilities == nil {
		initResult.Capabilities = &mcp.ServerCapabilities{}
	}
	if initResult.Capabilities.Experimental == nil {
		initResult.Capabilities.Experimental = make(map[string]any)
	}
	initResult.Capabilities.Experimental[extensionID] = map[string]any{
		"availableVariants":     availableVariants,
		"moreVariantsAvailable": len(ranked) < len(s.variants),
	}

	return initResult, nil
}

// unionCapabilities merges multiple ServerCapabilities into a single set
// that the front proxy server advertises to clients. The merge strategy is:
//
//   - Tools, Resources, Prompts: the capability is advertised if any variant
//     advertises it. Boolean sub-fields (ListChanged, Subscribe) are OR-ed
//     across all variants so the front server enables the feature if at least
//     one inner server supports it.
//   - Completions, Logging: marker capabilities (empty structs). Advertised
//     if any variant advertises them; the first non-nil value is kept.
//   - Experimental: keys are merged into a single map. The first variant to
//     register a given key wins; later duplicates are ignored.
func unionCapabilities(allCaps []*mcp.ServerCapabilities) *mcp.ServerCapabilities {
	union := &mcp.ServerCapabilities{}

	for _, caps := range allCaps {
		if caps == nil {
			continue
		}

		if caps.Tools != nil {
			if union.Tools == nil {
				union.Tools = &mcp.ToolCapabilities{}
			}
			union.Tools.ListChanged = union.Tools.ListChanged || caps.Tools.ListChanged
		}

		if caps.Resources != nil {
			if union.Resources == nil {
				union.Resources = &mcp.ResourceCapabilities{}
			}
			union.Resources.Subscribe = union.Resources.Subscribe || caps.Resources.Subscribe
			union.Resources.ListChanged = union.Resources.ListChanged || caps.Resources.ListChanged
		}

		if caps.Prompts != nil {
			if union.Prompts == nil {
				union.Prompts = &mcp.PromptCapabilities{}
			}
			union.Prompts.ListChanged = union.Prompts.ListChanged || caps.Prompts.ListChanged
		}

		if caps.Completions != nil && union.Completions == nil {
			union.Completions = caps.Completions
		}
		if caps.Logging != nil && union.Logging == nil {
			union.Logging = caps.Logging
		}

		if caps.Experimental != nil {
			if union.Experimental == nil {
				union.Experimental = make(map[string]any)
			}
			for k, v := range caps.Experimental {
				if _, exists := union.Experimental[k]; !exists {
					union.Experimental[k] = v
				}
			}
		}
	}

	return union
}
