// Example: Model-Optimized Weather Service — the same tools with different
// descriptions and schemas optimized for different LLM families. This
// demonstrates the SEP-2053 primary use case where tool descriptions are
// tailored per model family for optimal tool-calling performance.
//
// Capability demonstrated: Same tools, different descriptions per variant.
//
// Variants:
//   - claude-optimized: Detailed, structured descriptions with explicit guidance
//   - gpt-optimized: Concise function-style descriptions with JSON Schema emphasis
//   - compact: Minimal descriptions for context-constrained environments
//
// Run:
//
//	go run ./examples/server/model-optimized
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
	// Claude-optimized variant: verbose, structured descriptions with
	// explicit guidance on when and how to use each tool.
	claudeServer := mcp.NewServer(&mcp.Implementation{Name: "weather-service", Version: "v1.0.0"}, nil)
	mcp.AddTool(claudeServer, &mcp.Tool{
		Name: "get_weather",
		Description: "Get the current weather conditions for a specific location. " +
			"Use this tool when the user asks about current weather, temperature, or conditions. " +
			"Accepts city names (e.g., 'San Francisco, CA'), country names, or lat/long coordinates. " +
			"Returns temperature, humidity, wind speed, and a human-readable condition description. " +
			"Prefer this over get_forecast when the user only needs current conditions.",
	}, getWeather)
	mcp.AddTool(claudeServer, &mcp.Tool{
		Name: "get_forecast",
		Description: "Get a multi-day weather forecast for a specific location. " +
			"Use this tool when the user asks about future weather, upcoming conditions, or wants to plan ahead. " +
			"Returns daily high/low temperatures and conditions for 1-7 days. " +
			"Defaults to 3 days if not specified. Use get_weather instead if only current conditions are needed.",
	}, getForecast)

	// GPT-optimized variant: concise function-style descriptions with
	// JSON Schema emphasis for OpenAI-family models.
	gptServer := mcp.NewServer(&mcp.Implementation{Name: "weather-service", Version: "v1.0.0"}, nil)
	mcp.AddTool(gptServer, &mcp.Tool{
		Name:        "get_weather",
		Description: "Returns current weather data (temperature, humidity, wind, conditions) for a given location.",
	}, getWeather)
	mcp.AddTool(gptServer, &mcp.Tool{
		Name:        "get_forecast",
		Description: "Returns multi-day forecast (high/low temps, conditions) for a location. Days: 1-7, default 3.",
	}, getForecast)

	// Compact variant: minimal descriptions for context-constrained
	// environments where token budget is limited.
	compactServer := mcp.NewServer(&mcp.Implementation{Name: "weather-service", Version: "v1.0.0"}, nil)
	mcp.AddTool(compactServer, &mcp.Tool{
		Name:        "get_weather",
		Description: "Current weather for a location.",
	}, getWeather)
	mcp.AddTool(compactServer, &mcp.Tool{
		Name:        "get_forecast",
		Description: "Multi-day forecast for a location.",
	}, getForecast)

	vs := variants.NewServer(&mcp.Implementation{Name: "weather-service", Version: "v1.0.0"}).
		WithVariant(variants.ServerVariant{
			ID:          "claude-optimized",
			Description: "Detailed, structured tool descriptions with explicit usage guidance. Optimized for Anthropic Claude models that benefit from rich context and clear instructions.",
			Hints:       map[string]string{"modelFamily": "anthropic", "contextSize": "verbose"},
			Status:      variants.Stable,
		}, claudeServer, 0).
		WithVariant(variants.ServerVariant{
			ID:          "gpt-optimized",
			Description: "Concise function-style descriptions with JSON Schema emphasis. Optimized for OpenAI GPT models that work well with terse, specification-like tool definitions.",
			Hints:       map[string]string{"modelFamily": "openai", "contextSize": "standard"},
			Status:      variants.Stable,
		}, gptServer, 1).
		WithVariant(variants.ServerVariant{
			ID:          "compact",
			Description: "Minimal tool descriptions for context-constrained environments. Use when token budget is severely limited or tools are self-explanatory from their schemas.",
			Hints:       map[string]string{"modelFamily": "any", "contextSize": "compact"},
			Status:      variants.Stable,
		}, compactServer, 2).
		// Custom ranking: match by modelFamily hint, fall back to priority.
		WithRanking(func(_ context.Context, hints variants.VariantHints, vs []variants.ServerVariant) []variants.ServerVariant {
			requested, _ := variants.HintValue[string](hints, "modelFamily")
			slices.SortStableFunc(vs, func(a, b variants.ServerVariant) int {
				aMatch := strings.EqualFold(a.Hints["modelFamily"], requested)
				bMatch := strings.EqualFold(b.Hints["modelFamily"], requested)
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
