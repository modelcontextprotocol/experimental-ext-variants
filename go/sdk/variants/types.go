// Copyright 2025 The MCP Variants Authors. All rights reserved.
// Use of this source code is governed by a Apache-2.0
// license that can be found in the LICENSE file.

package variants

import "context"

// ---------------------------------------------------------------------------
// Variant status
// ---------------------------------------------------------------------------

// VariantStatus represents the stability status of a server variant.
type VariantStatus string

const (
	// Stable indicates a production-ready variant recommended for general use.
	Stable VariantStatus = "stable"
	// Experimental indicates a variant that may change without notice; use for testing.
	Experimental VariantStatus = "experimental"
	// Deprecated indicates a variant that will be removed in a future release.
	Deprecated VariantStatus = "deprecated"
)

// ---------------------------------------------------------------------------
// Deprecation info
// ---------------------------------------------------------------------------

// DeprecationInfo provides migration guidance for deprecated variants.
type DeprecationInfo struct {
	// Message is a human-readable message explaining why this variant is
	// deprecated and how to migrate.
	Message string `json:"message"`

	// Replacement is the suggested replacement variant identifier.
	Replacement string `json:"replacement,omitempty"`

	// RemovalDate is an optional ISO 8601 date when this variant is planned
	// to be removed. Servers SHOULD continue to support the variant until
	// that date.
	RemovalDate string `json:"removalDate,omitempty"`
}

// ---------------------------------------------------------------------------
// Server variant
// ---------------------------------------------------------------------------

// ServerVariant describes a server capability variant that clients can select.
// Each variant represents a distinct configuration of all server capabilities
// (tools, resources, prompts, subscriptions).
type ServerVariant struct {
	// ID is a unique identifier for this variant. Freeform string that servers
	// define. Examples: "claude-optimized", "gpt-optimized", "compact",
	// "agent-plan".
	//
	// Each variant's ID MUST be unique within availableVariants.
	ID string `json:"id"`

	// Description is a human-readable description of this variant, suitable
	// for display to users or for LLM reasoning about variant selection.
	//
	// SHOULD include:
	//   - Target use case or model family
	//   - Key characteristics or optimizations
	//   - Trade-offs compared to other variants
	Description string `json:"description"`

	// Hints are key-value pairs providing structured metadata for intelligent
	// variant selection. Clients and LLMs can use these hints to
	// programmatically filter and rank variants.
	//
	// Unknown hint keys MUST be ignored by clients and servers.
	Hints map[string]string `json:"hints,omitempty"`

	// priority determines the default ordering when no RankingFunc is set;
	// lower values indicate higher importance (0 = highest priority).
	// This field is set by Server.WithVariant and is readable via the
	// Priority() getter for use in custom RankingFunc implementations.
	priority int

	// Status is the stability status of this variant.
	// Defaults to Stable if empty.
	Status VariantStatus `json:"status,omitempty"`

	// DeprecationInfo provides migration guidance when Status is Deprecated.
	DeprecationInfo *DeprecationInfo `json:"deprecationInfo,omitempty"`
}

// Priority returns the variant's priority value. Lower values indicate
// higher importance (0 = highest priority).
func (v ServerVariant) Priority() int {
	return v.priority
}

// ---------------------------------------------------------------------------
// Hint keys
// ---------------------------------------------------------------------------

// HintKey is a well-known key for use in VariantHints.Hints and
// ServerVariant.Hints.
type HintKey = string

// Well-known hint keys defined by the SEP (Common Hint Vocabulary).
const (
	// HintModelFamily identifies the target model family/provider.
	// Common values: "anthropic", "openai", "google", "meta", "local", "any".
	HintModelFamily HintKey = "modelFamily"

	// HintUseCase identifies the intended usage scenario.
	// Common values: "autonomous-agent", "human-assistant", "ide", "api",
	// "chat", "planning", "execution".
	HintUseCase HintKey = "useCase"

	// HintContextSize indicates the desired verbosity / token efficiency.
	// Common values: "compact", "standard", "verbose".
	HintContextSize HintKey = "contextSize"

	// HintRenderingCapabilities describes the client's rendering support.
	// Common values: "rich", "markdown", "text-only".
	HintRenderingCapabilities HintKey = "renderingCapabilities"

	// HintLanguageOptimization indicates natural language optimization.
	// Common values: "en", "multilingual", "code-focused".
	HintLanguageOptimization HintKey = "languageOptimization"
)

// ---------------------------------------------------------------------------
// Variant hints
// ---------------------------------------------------------------------------

// VariantHints are structured hints provided by the client to help the server
// rank available variants.
type VariantHints struct {
	// Description is a human-readable description of the client's context and
	// requirements.
	Description string `json:"description,omitempty"`

	// Hints are key-value pairs providing structured metadata for variant
	// selection. Values can be a single string or an array of strings (in
	// order of preference).
	//
	// Unknown hint keys MUST be ignored by clients and servers.
	Hints map[string]any `json:"hints,omitempty"`
}

// HintValue retrieves a typed value from the Hints map for the given key.
// It returns the zero value and false if the key is missing, the map is nil,
// or the stored value is not assignable to T.
func HintValue[T any](h VariantHints, key string) (T, bool) {
	var zero T
	if h.Hints == nil {
		return zero, false
	}
	v, ok := h.Hints[key]
	if !ok {
		return zero, false
	}
	t, ok := v.(T)
	return t, ok
}

// ---------------------------------------------------------------------------
// Ranking function
// ---------------------------------------------------------------------------

// RankingFunc is called during initialization to rank available variants based
// on the client's hints. It receives the client-provided hints and the full
// set of registered variants, and must return them sorted by relevance with
// the highest-priority variant first. The first variant in the returned slice
// is the recommended default and will be used when a client does not
// explicitly select a variant via _meta.
//
// Each ServerVariant carries its Priority field (set via WithVariant), which
// the ranking function may use as a baseline signal alongside client hints.
//
// Note: The default (first) variant is also used when the client does not
// support variants at all.
type RankingFunc func(ctx context.Context, hints VariantHints, variants []ServerVariant) []ServerVariant
