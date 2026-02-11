// Example: Setting up a variant-aware MCP server with multiple variants.
package main

import (
	"context"
	"log"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/modelcontextprotocol/experimental-ext-variants/go/sdk/variants"
)

// -- Coding variant tool types ------------------------------------------------

type AnalyzeCodeInput struct {
	Code     string `json:"code" jsonschema:"source code to analyze"`
	Language string `json:"language" jsonschema:"programming language"`
}

type AnalyzeCodeOutput struct {
	Issues      []string `json:"issues"`
	Suggestions []string `json:"suggestions"`
}

func AnalyzeCode(_ context.Context, _ *mcp.CallToolRequest, in AnalyzeCodeInput) (*mcp.CallToolResult, AnalyzeCodeOutput, error) {
	return nil, AnalyzeCodeOutput{
		Issues:      []string{"unused variable on line 3"},
		Suggestions: []string{"consider using a switch statement"},
	}, nil
}

type RefactorInput struct {
	Code   string `json:"code" jsonschema:"source code to refactor"`
	Action string `json:"action" jsonschema:"refactoring action, e.g. extract-function"`
}

type RefactorOutput struct {
	Refactored string `json:"refactored"`
}

func Refactor(_ context.Context, _ *mcp.CallToolRequest, in RefactorInput) (*mcp.CallToolResult, RefactorOutput, error) {
	return nil, RefactorOutput{Refactored: "// refactored\n" + in.Code}, nil
}

// -- Compact variant tool types -----------------------------------------------

type SummarizeInput struct {
	Text string `json:"text" jsonschema:"text to summarize"`
}

type SummarizeOutput struct {
	Summary string `json:"summary"`
}

func Summarize(_ context.Context, _ *mcp.CallToolRequest, in SummarizeInput) (*mcp.CallToolResult, SummarizeOutput, error) {
	return nil, SummarizeOutput{Summary: in.Text[:min(len(in.Text), 50)]}, nil
}

type LookupInput struct {
	Query string `json:"query" jsonschema:"lookup query"`
}

type LookupOutput struct {
	Result string `json:"result"`
}

func Lookup(_ context.Context, _ *mcp.CallToolRequest, in LookupInput) (*mcp.CallToolResult, LookupOutput, error) {
	return nil, LookupOutput{Result: "result for: " + in.Query}, nil
}

// -----------------------------------------------------------------------------

func main() {
	// Server 1: coding-focused tools
	server1 := mcp.NewServer(&mcp.Implementation{Name: "my-server", Version: "v1.0.0"}, nil)
	mcp.AddTool(server1, &mcp.Tool{Name: "analyze_code", Description: "Perform static analysis on source code"}, AnalyzeCode)
	mcp.AddTool(server1, &mcp.Tool{Name: "refactor", Description: "Apply a refactoring action to source code"}, Refactor)

	// Server 2: compact / minimal tools
	server2 := mcp.NewServer(&mcp.Implementation{Name: "my-server", Version: "v1.0.0"}, nil)
	mcp.AddTool(server2, &mcp.Tool{Name: "summarize", Description: "Summarize text"}, Summarize)
	mcp.AddTool(server2, &mcp.Tool{Name: "lookup", Description: "Quick fact lookup"}, Lookup)

	coding := variants.ServerVariant{
		ID:          "coding-assistant",
		Description: "Optimized for coding workflows",
		Hints:       map[string]string{"useCase": "coding", "contextSize": "standard"},
		Status:      variants.Stable,
	}

	compact := variants.ServerVariant{
		ID:          "compact",
		Description: "Minimal token usage",
		Hints:       map[string]string{"contextSize": "small"},
		Status:      variants.Experimental,
	}

	vs := variants.NewServer().
		WithVariant(coding, server1, 0).
		WithVariant(compact, server2, 1).
		WithRanking(func(_ context.Context, _ variants.VariantHints, vs []variants.ServerVariant) []variants.ServerVariant {
			// rank based on client hints, return sorted
			// note: this is optional, defults to ranking by priority.
			return vs
		})

	ctx := context.Background()
	if err := vs.Run(ctx, nil); err != nil {
		log.Fatal(err)
	}
}
