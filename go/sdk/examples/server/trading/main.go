// Example: Trading Platform — API versioning and lifecycle management using
// MCP server variants. Demonstrates DeprecationInfo, all three variant
// statuses (stable, experimental, deprecated), and access control patterns
// (read-only subset).
//
// Capability demonstrated: Variant lifecycle, DeprecationInfo, API migration.
//
// Variants:
//   - v2-stable: Production API with order management and portfolio tools
//   - v3-preview: Next-gen experimental API with streaming and enhanced orders
//   - v1-legacy: Deprecated legacy API scheduled for removal
//   - analysis-only: Read-only subset for analytics and monitoring
//
// Run:
//
//	go run ./examples/server/trading
//
// Then connect any MCP client to http://localhost:8080.
package main

import (
	"log"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/modelcontextprotocol/experimental-ext-variants/go/sdk/variants"
)

func main() {
	// V2 stable: production API
	v2Server := mcp.NewServer(&mcp.Implementation{Name: "trading-platform", Version: "v2.0.0"}, nil)
	mcp.AddTool(v2Server, &mcp.Tool{
		Name:        "place_order",
		Description: "Place a market or limit order for a given symbol",
	}, placeOrder)
	mcp.AddTool(v2Server, &mcp.Tool{
		Name:        "get_quotes",
		Description: "Get real-time quotes for one or more symbols",
	}, getQuotes)
	mcp.AddTool(v2Server, &mcp.Tool{
		Name:        "get_portfolio",
		Description: "Get portfolio positions and account balance",
	}, getPortfolio)
	mcp.AddTool(v2Server, &mcp.Tool{
		Name:        "cancel_order",
		Description: "Cancel a pending order by order ID",
	}, cancelOrder)

	// V3 preview: next-gen experimental API
	v3Server := mcp.NewServer(&mcp.Implementation{Name: "trading-platform", Version: "v3.0.0-preview"}, nil)
	mcp.AddTool(v3Server, &mcp.Tool{
		Name:        "place_order_v3",
		Description: "Place an order with advanced options: stop/stop-limit types, time-in-force, and custom tags",
	}, placeOrderV3)
	mcp.AddTool(v3Server, &mcp.Tool{
		Name:        "stream_quotes",
		Description: "Subscribe to real-time quote streaming for one or more symbols",
	}, streamQuotes)
	mcp.AddTool(v3Server, &mcp.Tool{
		Name:        "get_portfolio_v3",
		Description: "Get enhanced portfolio with margin data and optional greeks",
	}, getPortfolioV3)

	// V1 legacy: deprecated API scheduled for removal
	v1Server := mcp.NewServer(&mcp.Implementation{Name: "trading-platform", Version: "v1.0.0"}, nil)
	mcp.AddTool(v1Server, &mcp.Tool{
		Name:        "trade",
		Description: "Execute a trade (deprecated — use place_order in v2-stable)",
	}, trade)
	mcp.AddTool(v1Server, &mcp.Tool{
		Name:        "quote",
		Description: "Get a single stock quote (deprecated — use get_quotes in v2-stable)",
	}, quote)
	mcp.AddTool(v1Server, &mcp.Tool{
		Name:        "balance",
		Description: "Get account balance (deprecated — use get_portfolio in v2-stable)",
	}, balance)

	// Analysis-only: read-only subset for analytics
	analysisServer := mcp.NewServer(&mcp.Implementation{Name: "trading-platform", Version: "v2.0.0"}, nil)
	mcp.AddTool(analysisServer, &mcp.Tool{
		Name:        "get_quotes",
		Description: "Get real-time quotes for one or more symbols",
	}, getQuotes)
	mcp.AddTool(analysisServer, &mcp.Tool{
		Name:        "get_portfolio",
		Description: "Get portfolio positions and account balance (read-only view)",
	}, getPortfolio)
	mcp.AddTool(analysisServer, &mcp.Tool{
		Name:        "get_historical_data",
		Description: "Get historical OHLCV price data for a symbol over a given period",
	}, getHistoricalData)

	vs := variants.NewServer(&mcp.Implementation{Name: "trading-platform", Version: "v2.0.0"}).
		WithVariant(variants.ServerVariant{
			ID:          "v2-stable",
			Description: "Production trading API (v2). Full order management with market/limit orders, real-time quotes, portfolio tracking, and order cancellation.",
			Hints:       map[string]string{"com.example/apiGeneration": "v2", "contextSize": "standard"},
			Status:      variants.Stable,
		}, v2Server, 0).
		WithVariant(variants.ServerVariant{
			ID:          "v3-preview",
			Description: "Next-generation trading API (v3 preview). Adds stop/stop-limit orders, streaming quotes, margin data, and custom order tags. May change without notice.",
			Hints:       map[string]string{"com.example/apiGeneration": "v3", "contextSize": "standard"},
			Status:      variants.Experimental,
		}, v3Server, 1).
		WithVariant(variants.ServerVariant{
			ID:          "v1-legacy",
			Description: "Legacy trading API (v1). Provides basic trade, quote, and balance operations. Scheduled for removal — migrate to v2-stable.",
			Hints:       map[string]string{"com.example/apiGeneration": "v1", "contextSize": "compact"},
			Status:      variants.Deprecated,
			DeprecationInfo: &variants.DeprecationInfo{
				Message:     "v1 API is deprecated. Migrate to v2-stable for improved order types, multi-symbol quotes, and portfolio tracking.",
				Replacement: "v2-stable",
				RemovalDate: "2026-06-30",
			},
		}, v1Server, 2).
		WithVariant(variants.ServerVariant{
			ID:          "analysis-only",
			Description: "Read-only analytics variant. Provides market data, portfolio viewing, and historical data without any order placement or modification capabilities.",
			Hints:       map[string]string{"com.example/apiGeneration": "v2", "useCase": "planning", "contextSize": "standard"},
			Status:      variants.Stable,
		}, analysisServer, 1)

	handler := variants.NewStreamableHTTPHandler(vs, nil)

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}
