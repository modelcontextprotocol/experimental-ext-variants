package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type SearchPapersInput struct {
	Query      string `json:"query" jsonschema:"search query for academic papers"`
	MaxResults int    `json:"maxResults,omitempty" jsonschema:"maximum number of results (1-50, default: 10)"`
	Year       int    `json:"year,omitempty" jsonschema:"filter by publication year"`
	Field      string `json:"field,omitempty" jsonschema:"filter by field: cs, math, physics, biology, medicine"`
}

type Paper struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Authors   []string `json:"authors"`
	Year      int      `json:"year"`
	Abstract  string   `json:"abstract"`
	Citations int      `json:"citations"`
}

type SearchPapersOutput struct {
	Papers     []Paper `json:"papers"`
	TotalFound int     `json:"totalFound"`
}

func searchPapers(_ context.Context, _ *mcp.CallToolRequest, in SearchPapersInput) (*mcp.CallToolResult, SearchPapersOutput, error) {
	return nil, SearchPapersOutput{
		Papers: []Paper{
			{
				ID:        "arxiv:2301.01234",
				Title:     "Attention Is All You Need: A Comprehensive Survey",
				Authors:   []string{"Smith, J.", "Chen, L.", "Kumar, A."},
				Year:      2024,
				Abstract:  "We present a comprehensive survey of transformer architectures and their applications across natural language processing, computer vision, and scientific computing.",
				Citations: 342,
			},
			{
				ID:        "arxiv:2302.05678",
				Title:     "Scaling Laws for Neural Language Models",
				Authors:   []string{"Johnson, R.", "Williams, K."},
				Year:      2024,
				Abstract:  "This paper investigates empirical scaling laws governing the performance of language models as a function of model size, dataset size, and compute budget.",
				Citations: 189,
			},
		},
		TotalFound: 1247,
	}, nil
}

type GetPaperInput struct {
	ID string `json:"id" jsonschema:"paper identifier (e.g. arxiv:2301.01234, doi:10.1234/5678)"`
}

type PaperDetail struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Authors    []string `json:"authors"`
	Year       int      `json:"year"`
	Abstract   string   `json:"abstract"`
	Citations  int      `json:"citations"`
	References []string `json:"references"`
	Keywords   []string `json:"keywords"`
	Venue      string   `json:"venue"`
}

type GetPaperOutput struct {
	Paper PaperDetail `json:"paper"`
}

func getPaper(_ context.Context, _ *mcp.CallToolRequest, in GetPaperInput) (*mcp.CallToolResult, GetPaperOutput, error) {
	return nil, GetPaperOutput{
		Paper: PaperDetail{
			ID:         in.ID,
			Title:      "Attention Is All You Need: A Comprehensive Survey",
			Authors:    []string{"Smith, J.", "Chen, L.", "Kumar, A."},
			Year:       2024,
			Abstract:   "We present a comprehensive survey of transformer architectures and their applications across natural language processing, computer vision, and scientific computing.",
			Citations:  342,
			References: []string{"arxiv:1706.03762", "arxiv:1810.04805", "arxiv:2005.14165"},
			Keywords:   []string{"transformers", "attention mechanisms", "deep learning", "survey"},
			Venue:      "NeurIPS 2024",
		},
	}, nil
}

type SummarizeInput struct {
	Text   string `json:"text" jsonschema:"text to summarize"`
	Style  string `json:"style,omitempty" jsonschema:"summary style: abstract, bullet-points, executive (default: abstract)"`
	Length string `json:"length,omitempty" jsonschema:"summary length: short, medium, long (default: medium)"`
}

type SummarizeOutput struct {
	Summary string `json:"summary"`
	Style   string `json:"style"`
}

func summarize(_ context.Context, _ *mcp.CallToolRequest, in SummarizeInput) (*mcp.CallToolResult, SummarizeOutput, error) {
	style := in.Style
	if style == "" {
		style = "abstract"
	}
	return nil, SummarizeOutput{
		Summary: "This paper presents key findings in the field, demonstrating significant improvements over prior work through novel methodology and comprehensive evaluation across multiple benchmarks.",
		Style:   style,
	}, nil
}
