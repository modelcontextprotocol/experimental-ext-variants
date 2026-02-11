// Copyright 2025 The MCP Variants Authors. All rights reserved.
// Use of this source code is governed by a Apache-2.0
// license that can be found in the LICENSE file.

package variants

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// variantEntry binds a ServerVariant to its backing mcp.Server.
type variantEntry struct {
	variant ServerVariant
	server  *mcp.Server
}

// Server is a variant-aware MCP server that multiplexes across multiple
// mcp.Server instances, each associated with a ServerVariant. During
// initialization, clients provide VariantHints and the server responds with a
// ranked list of available variants. Per-request variant selection is carried
// in the _meta field.
type Server struct {
	variants    []variantEntry
	variantMap  map[string]*mcp.Server // ID -> server for O(1) lookup
	rankingFunc RankingFunc
}

// NewServer creates a new variant-aware server with no registered variants.
func NewServer() *Server {
	return &Server{
		variantMap: make(map[string]*mcp.Server),
	}
}

// WithVariant registers a ServerVariant backed by the given mcp.Server.
// priority determines the default ordering when no RankingFunc is set;
// lower values indicate higher importance (0 = highest priority). By
// default, the variant with the lowest priority value will appear first
// in the list and serve as the recommended default for clients. This
// behavior can be overridden by providing a custom RankingFunc.
//
// Per the SEP, the first variant in the ranked list MUST have status
// "stable" (unless the client explicitly requested experimental variants
// via variantHints). Priority ordering is respected within that
// constraint: variants are sorted by priority, but the first stable
// variant is always promoted to the front of the list.
//
// Variant IDs must be unique; registering a duplicate panics.
func (s *Server) WithVariant(v ServerVariant, mcpServer *mcp.Server, priority int) *Server {
	if _, exists := s.variantMap[v.ID]; exists {
		panic("variants: duplicate variant ID: " + v.ID)
	}
	v.priority = priority
	s.variants = append(s.variants, variantEntry{variant: v, server: mcpServer})
	s.variantMap[v.ID] = mcpServer
	return s
}

// WithRanking sets a custom ranking function used to order variants based
// on client hints during initialization. The function should return variants
// sorted by relevance, with the most appropriate variant first. If nil,
// variants are ordered by their priority value (lowest first).
//
// The ranking function does not need to enforce the SEP's stable-first
// invariant. After the function returns, the SDK will automatically promote
// the first stable variant to the front of the list (per the SEP: "The first
// element MUST have status stable unless the client explicitly requested
// experimental variants via variantHints"). The rest of the ranking order
// is preserved.
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

// ServerFor returns the mcp.Server associated with the given variant ID, or
// nil if no variant with that ID is registered.
func (s *Server) ServerFor(id string) *mcp.Server {
	return s.variantMap[id]
}

// Run starts the variant server.
func (s *Server) Run(ctx context.Context, t mcp.Transport) error {
	// TODO: Implement Server.Run using a method-dispatching proxy pattern.
	//
	// Architecture: the variant Server acts as a "front" MCP server that owns
	// the real transport, handles lifecycle/discovery methods itself, and
	// dispatches tool/resource/prompt calls to the correct inner *mcp.Server
	// based on _meta["io.modelcontextprotocol/server-variant"].
	//
	// ---------------------------------------------------------------------------
	// Constants
	// ---------------------------------------------------------------------------
	//
	// Extension ID (plural, for capability negotiation):
	//   "io.modelcontextprotocol/server-variants"
	//
	// Per-request _meta key (singular, for variant selection):
	//   "io.modelcontextprotocol/server-variant"
	//
	// ---------------------------------------------------------------------------
	// Assumptions (v1)
	// ---------------------------------------------------------------------------
	//
	// We need to make sure the SEP's session stability invariant is satisfied:
	//
	// "The server MUST ensure that the same variant is recommended and selected for the entire session, unless the client explicitly requests otherwise via variantHints."
	//
	// For the first version we will relax this constraint to avoid maintaining state.
	//
	// ---------------------------------------------------------------------------
	// Steps
	// ---------------------------------------------------------------------------
	//
	// 1. Create a "front" *mcp.Server inside Run. Configure it with
	//    ServerOptions.Capabilities set to the union of all variants'
	//    capability flags. For example, if any variant has tools, set
	//    Capabilities.Tools = &ToolCapabilities{ListChanged: true}. This
	//    causes the SDK to advertise the capability during initialize
	//    without registering any actual tools, resources, or prompts.
	//    The inner *mcp.Server instances are never connected to a
	//    transport — they are only used as handler registries.
	//
	// 2. Add a receiving middleware via AddReceivingMiddleware on the
	//    front server. This single middleware handles all variant-aware
	//    dispatch:
	//
	//    a. "initialize": read variantHints from the client's extension
	//       payload at capabilities.extensions[
	//       "io.modelcontextprotocol/server-variants"]. Call
	//       RankedVariants() and call next() to let the SDK produce
	//       the base InitializeResult, then inject availableVariants
	//       (including id, description, hints, status, and
	//       deprecationInfo where applicable) into the response at
	//       capabilities.extensions[
	//       "io.modelcontextprotocol/server-variants"]. Include
	//       moreVariantsAvailable if only a subset is returned.
	//
	//    b. "tools/list", "resources/list", "prompts/list",
	//       "resources/templates/list", "completion/complete":
	//       read _meta["io.modelcontextprotocol/server-variant"] to
	//       determine the active variant (default to first-ranked if
	//       absent). Look up the corresponding inner *mcp.Server and
	//       forward the request to its handler. Return only that
	//       variant's items. Cursors returned in paginated responses
	//       MUST be variant-scoped — embed the variant ID so that
	//       cross-variant cursor reuse is detectable.
	//
	//    c. "tools/call", "resources/read", "prompts/get":
	//       read _meta["io.modelcontextprotocol/server-variant"] from
	//       the request. Validate the variant ID:
	//         - If the ID is not in the session's availableVariants,
	//           return JSON-RPC error -32602 with message
	//           "Invalid server variant" and data containing
	//           { requestedVariant, availableVariants } (scoped to
	//           the authenticated principal's visible variants).
	//         - If absent, default to the first-ranked variant.
	//       Resolve the inner *mcp.Server via ServerFor(id) and
	//       forward the request.
	//
	//    d. "resources/subscribe": read the variant from _meta,
	//       validate as in (c). Resource notifications for this subscription
	//       MUST carry _meta["io.modelcontextprotocol/server-variant"]
	//       matching the bound variant. If the resource disappears
	//       from the variant, send a resource_updated notification
	//       followed by list_changed.
	//
	//    e. "notifications/tools/list_changed",
	//       "notifications/prompts/list_changed",
	//       "notifications/resources/list_changed": when an inner
	//       server emits a list_changed notification, the middleware
	//       re-emits it on the front server with
	//       _meta["io.modelcontextprotocol/server-variant"] set to
	//       the originating variant's ID.
	//
	//    f. All other methods ("ping", etc.):
	//       pass through to the front server's default handler.
	//
	// 3. Call frontServer.Run(ctx, t) to own the transport and manage
	//    the session lifecycle. The front server handles the JSON-RPC
	//    connection, SSE streaming, progress notifications, and
	//    session teardown.
	//
	// ---------------------------------------------------------------------------
	// Backward compatibility
	// ---------------------------------------------------------------------------
	//
	// If the client does not send the extension payload during
	// initialize, the server omits availableVariants from the response
	// and uses the first-ranked variant for all requests. The server
	// behaves identically to a non-variant-aware server.
	//
	return errors.New("variants: Run not implemented")
}
