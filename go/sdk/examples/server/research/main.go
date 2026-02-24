// Example: Research Assistant — context budget management with the same tools
// at different verbosity levels. Demonstrates how variants can optimize tool
// descriptions for different context window sizes and use cases.
//
// Capability demonstrated: Context budget management, description verbosity.
//
// Variants:
//   - deep-research: Verbose multi-paragraph descriptions with usage examples
//   - quick-lookup: Concise 1-sentence descriptions for fast Q&A
//   - synthesis: Balanced descriptions for report generation
//
// Run:
//
//	go run ./examples/server/research
//
// Then connect any MCP client to http://localhost:8080.
package main

import (
	"context"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/modelcontextprotocol/experimental-ext-variants/go/sdk/variants"
)

func main() {
	// Deep-research variant: verbose descriptions with usage examples
	// for thorough research workflows with large context windows.
	deepServer := mcp.NewServer(&mcp.Implementation{Name: "research-assistant", Version: "v1.0.0"}, nil)
	mcp.AddTool(deepServer, &mcp.Tool{
		Name: "search_papers",
		Description: "Search academic papers across multiple databases including arXiv, Semantic Scholar, and PubMed. " +
			"Use this tool to find relevant literature for a research topic. Supports filtering by year and field. " +
			"Returns paper metadata including titles, authors, abstracts, and citation counts. " +
			"For best results, use specific technical terms rather than broad queries. " +
			"Example queries: 'transformer attention mechanisms in computer vision', " +
			"'reinforcement learning from human feedback alignment'. " +
			"Follow up with get_paper to retrieve full details for promising results.",
	}, searchPapers)
	mcp.AddTool(deepServer, &mcp.Tool{
		Name: "get_paper",
		Description: "Retrieve full details for a specific academic paper by its identifier. " +
			"Accepts arXiv IDs (e.g., arxiv:2301.01234), DOIs (e.g., doi:10.1234/5678), or Semantic Scholar IDs. " +
			"Returns complete metadata including abstract, references, keywords, and publication venue. " +
			"Use this after search_papers to get deeper context on papers of interest. " +
			"The references list can be used to find related work by calling get_paper recursively. " +
			"Combine with summarize to generate concise summaries of lengthy papers.",
	}, getPaper)
	mcp.AddTool(deepServer, &mcp.Tool{
		Name: "summarize",
		Description: "Generate a summary of the provided text using various styles and lengths. " +
			"Styles: 'abstract' produces a formal academic abstract, 'bullet-points' extracts key findings " +
			"as a structured list, 'executive' creates a high-level overview for non-technical audiences. " +
			"Lengths: 'short' (1-2 sentences), 'medium' (1 paragraph), 'long' (2-3 paragraphs). " +
			"Best used with paper abstracts or full text retrieved via get_paper. " +
			"For multi-paper synthesis, concatenate abstracts and use the 'executive' style.",
	}, summarize)

	// Quick-lookup variant: concise descriptions for fast Q&A sessions
	// where context budget is limited.
	quickServer := mcp.NewServer(&mcp.Implementation{Name: "research-assistant", Version: "v1.0.0"}, nil)
	mcp.AddTool(quickServer, &mcp.Tool{
		Name:        "search_papers",
		Description: "Search academic papers by query, with optional year and field filters.",
	}, searchPapers)
	mcp.AddTool(quickServer, &mcp.Tool{
		Name:        "get_paper",
		Description: "Get full details for a paper by its ID (arXiv, DOI, etc.).",
	}, getPaper)
	mcp.AddTool(quickServer, &mcp.Tool{
		Name:        "summarize",
		Description: "Summarize text in abstract, bullet-point, or executive style.",
	}, summarize)

	// Synthesis variant: balanced descriptions for report generation
	// workflows that need moderate detail.
	synthesisServer := mcp.NewServer(&mcp.Implementation{Name: "research-assistant", Version: "v1.0.0"}, nil)
	mcp.AddTool(synthesisServer, &mcp.Tool{
		Name: "search_papers",
		Description: "Search academic papers across arXiv, Semantic Scholar, and PubMed. " +
			"Returns titles, authors, abstracts, and citation counts. Filter by year and field.",
	}, searchPapers)
	mcp.AddTool(synthesisServer, &mcp.Tool{
		Name: "get_paper",
		Description: "Retrieve full paper details including abstract, references, keywords, and venue. " +
			"Accepts arXiv IDs, DOIs, or Semantic Scholar IDs.",
	}, getPaper)
	mcp.AddTool(synthesisServer, &mcp.Tool{
		Name: "summarize",
		Description: "Generate a summary in abstract, bullet-point, or executive style. " +
			"Configurable length: short (1-2 sentences), medium (paragraph), or long (2-3 paragraphs).",
	}, summarize)

	vs := variants.NewServer(&mcp.Implementation{Name: "research-assistant", Version: "v1.0.0"}).
		WithVariant(variants.ServerVariant{
			ID:          "deep-research",
			Description: "Verbose tool descriptions with usage examples and guidance for thorough research workflows. Best for agents with large context windows performing literature reviews or deep analysis.",
			Hints:       map[string]string{"contextSize": "verbose", "useCase": "research"},
			Status:      variants.Stable,
		}, deepServer, 0).
		WithVariant(variants.ServerVariant{
			ID:          "quick-lookup",
			Description: "Concise 1-sentence tool descriptions for fast question-answering. Minimal context usage for agents with limited token budgets or simple lookup tasks.",
			Hints:       map[string]string{"contextSize": "compact", "useCase": "qa"},
			Status:      variants.Stable,
		}, quickServer, 1).
		WithVariant(variants.ServerVariant{
			ID:          "synthesis",
			Description: "Balanced tool descriptions for report generation and multi-paper synthesis. Moderate detail level suitable for structured writing workflows.",
			Hints:       map[string]string{"contextSize": "standard", "useCase": "synthesis"},
			Status:      variants.Experimental,
		}, synthesisServer, 2).
		// Custom ranking: match by contextSize hint, fall back to priority.
		WithRanking(func(_ context.Context, hints variants.VariantHints, vs []variants.ServerVariant) []variants.ServerVariant {
			requested, _ := variants.HintValue[string](hints, "contextSize")
			slices.SortStableFunc(vs, func(a, b variants.ServerVariant) int {
				aMatch := strings.EqualFold(a.Hints["contextSize"], requested)
				bMatch := strings.EqualFold(b.Hints["contextSize"], requested)
				if aMatch != bMatch {
					if aMatch {
						return -1
					}
					return 1
				}
				return a.Priority() - b.Priority()
			})
			return vs
		})

	handler := variants.NewStreamableHTTPHandler(vs, nil)

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}
