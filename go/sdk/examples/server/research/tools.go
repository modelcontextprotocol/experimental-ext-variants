package main

import (
	"context"
	"fmt"
	"time"

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

var databases = []string{"arXiv", "Semantic Scholar", "PubMed"}

func searchPapers(ctx context.Context, req *mcp.CallToolRequest, in SearchPapersInput) (*mcp.CallToolResult, SearchPapersOutput, error) {
	token := req.Params.GetProgressToken()
	total := len(databases)

	for i, db := range databases {
		req.Session.Log(ctx, &mcp.LoggingMessageParams{
			Level:  "info",
			Logger: "search_papers",
			Data:   fmt.Sprintf("Searching %s for %q", db, in.Query),
		})
		req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
			ProgressToken: token,
			Progress:      float64(i + 1),
			Total:         float64(total),
			Message:       fmt.Sprintf("Searching %s (%d/%d)", db, i+1, total),
		})
		time.Sleep(100 * time.Millisecond)
	}

	req.Session.Log(ctx, &mcp.LoggingMessageParams{
		Level:  "info",
		Logger: "search_papers",
		Data:   fmt.Sprintf("Found 1247 results for %q, returning top 2", in.Query),
	})

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

func getPaper(ctx context.Context, req *mcp.CallToolRequest, in GetPaperInput) (*mcp.CallToolResult, GetPaperOutput, error) {
	token := req.Params.GetProgressToken()
	steps := []string{"Fetching metadata", "Resolving references", "Loading citations"}

	for i, step := range steps {
		req.Session.Log(ctx, &mcp.LoggingMessageParams{
			Level:  "info",
			Logger: "get_paper",
			Data:   fmt.Sprintf("%s for %s", step, in.ID),
		})
		req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
			ProgressToken: token,
			Progress:      float64(i + 1),
			Total:         float64(len(steps)),
			Message:       fmt.Sprintf("%s (%d/%d)", step, i+1, len(steps)),
		})
		time.Sleep(80 * time.Millisecond)
	}

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

func summarize(ctx context.Context, req *mcp.CallToolRequest, in SummarizeInput) (*mcp.CallToolResult, SummarizeOutput, error) {
	style := in.Style
	if style == "" {
		style = "abstract"
	}

	token := req.Params.GetProgressToken()
	steps := []string{"Analyzing text", "Generating summary"}

	for i, step := range steps {
		req.Session.Log(ctx, &mcp.LoggingMessageParams{
			Level:  "info",
			Logger: "summarize",
			Data:   fmt.Sprintf("%s (%s style)", step, style),
		})
		req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
			ProgressToken: token,
			Progress:      float64(i + 1),
			Total:         float64(len(steps)),
			Message:       fmt.Sprintf("%s (%d/%d)", step, i+1, len(steps)),
		})
		time.Sleep(80 * time.Millisecond)
	}

	return nil, SummarizeOutput{
		Summary: "This paper presents key findings in the field, demonstrating significant improvements over prior work through novel methodology and comprehensive evaluation across multiple benchmarks.",
		Style:   style,
	}, nil
}
