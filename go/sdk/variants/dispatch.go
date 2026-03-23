// Copyright 2025 The MCP Variants Authors. All rights reserved.
// Use of this source code is governed by a Apache-2.0
// license that can be found in the LICENSE file.

package variants

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// dispatcher routes requests to inner variant servers. In stateful mode one
// dispatcher exists per client session; in stateless mode a single dispatcher
// is shared across all requests.
type dispatcher struct {
	server      *Server
	connections map[string]*innerConnection
}

// handle dispatches a request to the appropriate inner variant server.
// Unknown methods are passed through to next.
func (d *dispatcher) handle(ctx context.Context, method string, req mcp.Request, next mcp.MethodHandler) (mcp.Result, error) {
	switch method {
	case "tools/list", "resources/list", "prompts/list", "resources/templates/list":
		return d.handleList(ctx, method, req)
	default:
		return d.handleReceiveRedirect(ctx, method, req)
	}
}

// createInvalidVariantError creates a spec-compliant error for invalid variant selection.
func (d *dispatcher) createInvalidVariantError(ctx context.Context, requestedVariant string) error {
	ranked := d.server.RankedVariants(ctx, VariantHints{})
	availableIDs := make([]string, len(ranked))
	for i, v := range ranked {
		availableIDs[i] = v.ID
	}

	errorData := map[string]any{
		"requestedVariant":  requestedVariant,
		"availableVariants": availableIDs,
	}
	dataJSON, err := json.Marshal(errorData)
	if err != nil {
		dataJSON = []byte("{}")
	}

	return &jsonrpc.Error{
		Code:    jsonrpc.CodeInvalidParams,
		Message: "Invalid server variant",
		Data:    json.RawMessage(dataJSON),
	}
}

// isParamsNil checks if params is nil or a typed-nil (a nil pointer wrapped in an interface).
// The SDK can produce typed-nil params for requests with no parameters.
func isParamsNil(params mcp.Params) bool {
	if params == nil {
		return true
	}
	v := reflect.ValueOf(params)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

// variantIDFromMeta extracts the variant ID from the request's _meta field.
// Returns empty string if no variant is specified. Guards against typed-nil
// params (e.g. (*ListToolsParams)(nil) wrapped in the mcp.Params interface)
// which the SDK can produce for requests with no parameters.
func variantIDFromMeta(req mcp.Request) string {
	params := req.GetParams()
	if isParamsNil(params) {
		return ""
	}
	meta := params.GetMeta()
	if meta == nil {
		return ""
	}
	id, _ := meta[metaKeyVariant].(string)
	return id
}

// getConnection extracts the variant ID from request _meta and returns the
// corresponding innerConnection for dispatching. Falls back to the
// first-ranked variant when no variant is specified.
func (d *dispatcher) getConnection(ctx context.Context, req mcp.Request) (*innerConnection, error) {
	variantID := variantIDFromMeta(req)

	// If no variant specified, use first-ranked (default).
	//
	// BUG: This re-ranks with empty hints, which may differ from the
	// ranking returned during initialize (where client hints were used).
	// Per SEP-2053, the default should be the first variant from the
	// initialize response. To fix this properly, the per-session ranked
	// order should be stored during initialize and reused here.
	if variantID == "" {
		ranked := d.server.RankedVariants(ctx, VariantHints{})
		if len(ranked) == 0 {
			return nil, errors.New("no variants available")
		}
		variantID = ranked[0].ID
	}

	conn, ok := d.connections[variantID]
	if !ok {
		return nil, d.createInvalidVariantError(ctx, variantID)
	}

	return conn, nil
}

// enrichError adds activeVariant to a jsonrpc.Error's Data field for
// variant-scoped resolution failures. Per SEP-2053 Implementation Notes:
// "Servers SHOULD include activeVariant in error data for variant-scoped
// resolution failures (unknown tool/prompt/resource, invalid cursor, invalid
// subscription context)."
//
// Only errors with codes -32602 (InvalidParams) or -32601 (MethodNotFound)
// are enriched; business-logic errors from tool execution are passed through
// unmodified.
func enrichError(err error, variantID string) error {
	var jErr *jsonrpc.Error
	if !errors.As(err, &jErr) {
		return err
	}

	// Only enrich resolution-class errors.
	switch jErr.Code {
	case jsonrpc.CodeInvalidParams, jsonrpc.CodeMethodNotFound:
		// fall through to enrich
	default:
		return err
	}

	// Parse existing data (if any) and inject activeVariant.
	// Copy the error to avoid mutating the original
	data := make(map[string]any)
	if len(jErr.Data) > 0 {
		_ = json.Unmarshal(jErr.Data, &data)
	}
	data["activeVariant"] = variantID

	enriched := &jsonrpc.Error{
		Code:    jErr.Code,
		Message: jErr.Message,
	}
	if encoded, mErr := json.Marshal(data); mErr == nil {
		enriched.Data = json.RawMessage(encoded)
	}
	return enriched
}

// ---------------------------------------------------------------------------
// List methods
// ---------------------------------------------------------------------------

// handleList handles list methods using the generic backend session call method.
// Implements cursor scoping per SEP-2053: unwraps incoming cursors and wraps outgoing cursors.
// Uses type switches for type-safe cursor field access.
func (d *dispatcher) handleList(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
	conn, err := d.getConnection(ctx, req)
	if err != nil {
		return nil, err
	}

	backendSession := conn.backendSession
	variantID := backendSession.variantID
	params := req.GetParams()

	// Inject variant metadata and handle cursor unwrapping (guard against typed-nil params)
	if !isParamsNil(params) {
		injectVariantMeta(params, variantID)

		// Handle cursor unwrapping using type switch for type-safe field access
		switch p := params.(type) {
		case *mcp.ListToolsParams:
			if p.Cursor != "" {
				innerCursor, err := unwrapCursor(p.Cursor, variantID)
				if err != nil {
					return nil, err
				}
				p.Cursor = innerCursor
			}
		case *mcp.ListResourcesParams:
			if p.Cursor != "" {
				innerCursor, err := unwrapCursor(p.Cursor, variantID)
				if err != nil {
					return nil, err
				}
				p.Cursor = innerCursor
			}
		case *mcp.ListPromptsParams:
			if p.Cursor != "" {
				innerCursor, err := unwrapCursor(p.Cursor, variantID)
				if err != nil {
					return nil, err
				}
				p.Cursor = innerCursor
			}
		case *mcp.ListResourceTemplatesParams:
			if p.Cursor != "" {
				innerCursor, err := unwrapCursor(p.Cursor, variantID)
				if err != nil {
					return nil, err
				}
				p.Cursor = innerCursor
			}
		}
	}

	// Generic dispatch - pass entire request object
	result, err := backendSession.handleReceive(ctx, method, req)
	if err != nil {
		return nil, enrichError(err, variantID)
	}

	// Handle cursor wrapping in result using type switch for type-safe field access
	switch r := result.(type) {
	case *mcp.ListToolsResult:
		if r.NextCursor != "" {
			r.NextCursor = wrapCursor(r.NextCursor, variantID)
		}
	case *mcp.ListResourcesResult:
		if r.NextCursor != "" {
			r.NextCursor = wrapCursor(r.NextCursor, variantID)
		}
	case *mcp.ListPromptsResult:
		if r.NextCursor != "" {
			r.NextCursor = wrapCursor(r.NextCursor, variantID)
		}
	case *mcp.ListResourceTemplatesResult:
		if r.NextCursor != "" {
			r.NextCursor = wrapCursor(r.NextCursor, variantID)
		}
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Simple methods (no pagination)
// ---------------------------------------------------------------------------

// handleReceiveRedirect handles all simple methods (call, subscribe, unsubscribe, completion)
// that don't require special cursor handling. This consolidates what were previously
// separate handlers for handleCall, handleSubscribe, handleUnsubscribe, and handleCompletion.
func (d *dispatcher) handleReceiveRedirect(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
	conn, err := d.getConnection(ctx, req)
	if err != nil {
		return nil, err
	}

	backendSession := conn.backendSession
	variantID := backendSession.variantID
	params := req.GetParams()

	// Inject variant metadata (guard against typed-nil params)
	if !isParamsNil(params) {
		injectVariantMeta(params, variantID)
	}

	// Generic dispatch - pass entire request object
	result, err := backendSession.handleReceive(ctx, method, req)
	if err != nil {
		return nil, enrichError(err, variantID)
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Cursor scoping
// ---------------------------------------------------------------------------

// variantCursor wraps pagination cursors with variant ID for scoping.
// Per SEP-2053: "Cursors MUST be treated as opaque and variant-scoped"
type variantCursor struct {
	VariantID   string `json:"v"`
	InnerCursor string `json:"c"`
}

// wrapCursor wraps a cursor from an inner server with the variant ID.
// Returns empty string if the inner cursor is empty.
func wrapCursor(cursor string, variantID string) string {
	if cursor == "" {
		return ""
	}
	wrapped := variantCursor{
		VariantID:   variantID,
		InnerCursor: cursor,
	}
	data, err := json.Marshal(wrapped)
	if err != nil {
		// Should never happen with simple struct
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

// unwrapCursor validates and unwraps a cursor for the expected variant.
// Returns the inner cursor if valid, or an error if the cursor is invalid
// or belongs to a different variant.
func unwrapCursor(cursor string, expectedVariant string) (string, error) {
	if cursor == "" {
		return "", nil
	}

	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return "", &jsonrpc.Error{
			Code:    jsonrpc.CodeInvalidParams,
			Message: "Invalid cursor format",
		}
	}

	var wrapped variantCursor
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return "", &jsonrpc.Error{
			Code:    jsonrpc.CodeInvalidParams,
			Message: "Invalid cursor format",
		}
	}

	if wrapped.VariantID != expectedVariant {
		errorData := map[string]any{
			"cursorVariant":    wrapped.VariantID,
			"requestedVariant": expectedVariant,
		}
		dataJSON, _ := json.Marshal(errorData)

		return "", &jsonrpc.Error{
			Code:    jsonrpc.CodeInvalidParams,
			Message: "Cursor invalid for requested variant",
			Data:    json.RawMessage(dataJSON),
		}
	}

	return wrapped.InnerCursor, nil
}
