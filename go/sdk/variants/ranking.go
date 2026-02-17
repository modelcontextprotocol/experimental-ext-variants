// Copyright 2025 The MCP Variants Authors. All rights reserved.
// Use of this source code is governed by a Apache-2.0
// license that can be found in the LICENSE file.

package variants

import (
	"context"
	"slices"
)

// defaultRankingFunc is the built-in ranking function used when no custom
// RankingFunc is provided. It sorts variants by priority (lowest first),
// using stable-before-experimental-before-deprecated as a tiebreaker.
//
// The input slice is already a copy (from RankedVariants), so sorting
// in place is safe.
func defaultRankingFunc(_ context.Context, _ VariantHints, vs []ServerVariant) []ServerVariant {
	slices.SortStableFunc(vs, func(a, b ServerVariant) int {
		if a.Priority() != b.Priority() {
			return a.Priority() - b.Priority()
		}
		return statusWeight(a.Status) - statusWeight(b.Status)
	})
	return vs
}

// statusWeight returns a sort weight for a VariantStatus.
// Lower is better: stable < experimental < deprecated.
func statusWeight(s VariantStatus) int {
	switch s {
	case Stable, "":
		return 0
	case Experimental:
		return 1
	case Deprecated:
		return 2
	default:
		return 3
	}
}
