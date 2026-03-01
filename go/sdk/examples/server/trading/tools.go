package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// -- V2 tools -----------------------------------------------------------------

type PlaceOrderInput struct {
	Symbol   string  `json:"symbol" jsonschema:"ticker symbol (e.g. AAPL)"`
	Side     string  `json:"side" jsonschema:"order side: buy, sell"`
	Quantity int     `json:"quantity" jsonschema:"number of shares"`
	Type     string  `json:"type,omitempty" jsonschema:"order type: market, limit (default: market)"`
	Price    float64 `json:"price,omitempty" jsonschema:"limit price (required for limit orders)"`
}

type PlaceOrderOutput struct {
	OrderID string  `json:"orderId"`
	Symbol  string  `json:"symbol"`
	Side    string  `json:"side"`
	Status  string  `json:"status"`
	Price   float64 `json:"filledPrice"`
}

func placeOrder(_ context.Context, _ *mcp.CallToolRequest, in PlaceOrderInput) (*mcp.CallToolResult, PlaceOrderOutput, error) {
	price := in.Price
	if price == 0 {
		price = 185.50
	}
	return nil, PlaceOrderOutput{
		OrderID: "ORD-20250115-001",
		Symbol:  in.Symbol,
		Side:    in.Side,
		Status:  "filled",
		Price:   price,
	}, nil
}

type GetQuotesInput struct {
	Symbols []string `json:"symbols" jsonschema:"list of ticker symbols"`
}

type Quote struct {
	Symbol string  `json:"symbol"`
	Bid    float64 `json:"bid"`
	Ask    float64 `json:"ask"`
	Last   float64 `json:"last"`
	Volume int     `json:"volume"`
}

type GetQuotesOutput struct {
	Quotes []Quote `json:"quotes"`
}

func getQuotes(_ context.Context, _ *mcp.CallToolRequest, in GetQuotesInput) (*mcp.CallToolResult, GetQuotesOutput, error) {
	quotes := make([]Quote, len(in.Symbols))
	for i, sym := range in.Symbols {
		quotes[i] = Quote{
			Symbol: sym,
			Bid:    185.40 + float64(i),
			Ask:    185.60 + float64(i),
			Last:   185.50 + float64(i),
			Volume: 1500000 + i*100000,
		}
	}
	return nil, GetQuotesOutput{Quotes: quotes}, nil
}

type GetPortfolioInput struct {
	AccountID string `json:"accountId,omitempty" jsonschema:"account identifier (default: primary account)"`
}

type Position struct {
	Symbol   string  `json:"symbol"`
	Quantity int     `json:"quantity"`
	AvgCost  float64 `json:"avgCost"`
	Current  float64 `json:"currentPrice"`
	PnL      float64 `json:"unrealizedPnL"`
}

type GetPortfolioOutput struct {
	AccountID  string     `json:"accountId"`
	Positions  []Position `json:"positions"`
	CashBal    float64    `json:"cashBalance"`
	TotalValue float64    `json:"totalValue"`
}

func getPortfolio(_ context.Context, _ *mcp.CallToolRequest, in GetPortfolioInput) (*mcp.CallToolResult, GetPortfolioOutput, error) {
	return nil, GetPortfolioOutput{
		AccountID: "ACCT-001",
		Positions: []Position{
			{Symbol: "AAPL", Quantity: 100, AvgCost: 175.00, Current: 185.50, PnL: 1050.00},
			{Symbol: "GOOGL", Quantity: 50, AvgCost: 140.00, Current: 178.25, PnL: 1912.50},
		},
		CashBal:    25000.00,
		TotalValue: 52337.50,
	}, nil
}

type CancelOrderInput struct {
	OrderID string `json:"orderId" jsonschema:"order identifier to cancel"`
}

type CancelOrderOutput struct {
	OrderID string `json:"orderId"`
	Status  string `json:"status"`
}

func cancelOrder(_ context.Context, _ *mcp.CallToolRequest, in CancelOrderInput) (*mcp.CallToolResult, CancelOrderOutput, error) {
	return nil, CancelOrderOutput{
		OrderID: in.OrderID,
		Status:  "cancelled",
	}, nil
}

// -- V3 tools -----------------------------------------------------------------

type PlaceOrderV3Input struct {
	Symbol      string            `json:"symbol" jsonschema:"ticker symbol"`
	Side        string            `json:"side" jsonschema:"order side: buy, sell, short"`
	Quantity    int               `json:"quantity" jsonschema:"number of shares"`
	Type        string            `json:"type,omitempty" jsonschema:"order type: market, limit, stop, stop_limit (default: market)"`
	Price       float64           `json:"price,omitempty" jsonschema:"limit price"`
	StopPrice   float64           `json:"stopPrice,omitempty" jsonschema:"stop trigger price"`
	TimeInForce string            `json:"timeInForce,omitempty" jsonschema:"GTC, DAY, IOC, FOK (default: DAY)"`
	Tags        map[string]string `json:"tags,omitempty" jsonschema:"custom order tags for tracking"`
}

type PlaceOrderV3Output struct {
	OrderID     string  `json:"orderId"`
	Symbol      string  `json:"symbol"`
	Side        string  `json:"side"`
	Status      string  `json:"status"`
	Price       float64 `json:"filledPrice"`
	TimeInForce string  `json:"timeInForce"`
}

func placeOrderV3(_ context.Context, _ *mcp.CallToolRequest, in PlaceOrderV3Input) (*mcp.CallToolResult, PlaceOrderV3Output, error) {
	price := in.Price
	if price == 0 {
		price = 185.50
	}
	tif := in.TimeInForce
	if tif == "" {
		tif = "DAY"
	}
	return nil, PlaceOrderV3Output{
		OrderID:     "ORD-V3-20250115-001",
		Symbol:      in.Symbol,
		Side:        in.Side,
		Status:      "filled",
		Price:       price,
		TimeInForce: tif,
	}, nil
}

type StreamQuotesInput struct {
	Symbols  []string `json:"symbols" jsonschema:"list of ticker symbols to stream"`
	Interval int      `json:"interval,omitempty" jsonschema:"update interval in milliseconds (default: 1000)"`
}

type StreamQuotesOutput struct {
	Snapshot []Quote `json:"snapshot"`
	StreamID string  `json:"streamId"`
}

func streamQuotes(_ context.Context, _ *mcp.CallToolRequest, in StreamQuotesInput) (*mcp.CallToolResult, StreamQuotesOutput, error) {
	quotes := make([]Quote, len(in.Symbols))
	for i, sym := range in.Symbols {
		quotes[i] = Quote{
			Symbol: sym,
			Bid:    185.40 + float64(i),
			Ask:    185.60 + float64(i),
			Last:   185.50 + float64(i),
			Volume: 1500000 + i*100000,
		}
	}
	return nil, StreamQuotesOutput{
		Snapshot: quotes,
		StreamID: "stream-001",
	}, nil
}

type GetPortfolioV3Input struct {
	AccountID    string `json:"accountId,omitempty" jsonschema:"account identifier (default: primary account)"`
	IncludeGreek bool   `json:"includeGreeks,omitempty" jsonschema:"include options greeks in positions"`
}

type GetPortfolioV3Output struct {
	AccountID  string     `json:"accountId"`
	Positions  []Position `json:"positions"`
	CashBal    float64    `json:"cashBalance"`
	TotalValue float64    `json:"totalValue"`
	MarginUsed float64    `json:"marginUsed"`
}

func getPortfolioV3(_ context.Context, _ *mcp.CallToolRequest, in GetPortfolioV3Input) (*mcp.CallToolResult, GetPortfolioV3Output, error) {
	return nil, GetPortfolioV3Output{
		AccountID: "ACCT-001",
		Positions: []Position{
			{Symbol: "AAPL", Quantity: 100, AvgCost: 175.00, Current: 185.50, PnL: 1050.00},
			{Symbol: "GOOGL", Quantity: 50, AvgCost: 140.00, Current: 178.25, PnL: 1912.50},
		},
		CashBal:    25000.00,
		TotalValue: 52337.50,
		MarginUsed: 5000.00,
	}, nil
}

// -- V1 legacy tools ----------------------------------------------------------

type TradeInput struct {
	Ticker string `json:"ticker" jsonschema:"stock ticker"`
	Action string `json:"action" jsonschema:"buy or sell"`
	Shares int    `json:"shares" jsonschema:"number of shares"`
}

type TradeOutput struct {
	TradeID string `json:"tradeId"`
	Status  string `json:"status"`
}

func trade(_ context.Context, _ *mcp.CallToolRequest, in TradeInput) (*mcp.CallToolResult, TradeOutput, error) {
	return nil, TradeOutput{
		TradeID: fmt.Sprintf("T-%s-%d", in.Ticker, in.Shares),
		Status:  "executed",
	}, nil
}

type QuoteInput struct {
	Ticker string `json:"ticker" jsonschema:"stock ticker"`
}

type QuoteOutput struct {
	Ticker string  `json:"ticker"`
	Price  float64 `json:"price"`
}

func quote(_ context.Context, _ *mcp.CallToolRequest, in QuoteInput) (*mcp.CallToolResult, QuoteOutput, error) {
	return nil, QuoteOutput{
		Ticker: in.Ticker,
		Price:  185.50,
	}, nil
}

type BalanceInput struct {
	Account string `json:"account,omitempty" jsonschema:"account name"`
}

type BalanceOutput struct {
	Cash  float64 `json:"cash"`
	Total float64 `json:"total"`
}

func balance(_ context.Context, _ *mcp.CallToolRequest, in BalanceInput) (*mcp.CallToolResult, BalanceOutput, error) {
	return nil, BalanceOutput{
		Cash:  25000.00,
		Total: 52337.50,
	}, nil
}

// -- Analysis tools -----------------------------------------------------------

type GetHistoricalDataInput struct {
	Symbol   string `json:"symbol" jsonschema:"ticker symbol"`
	Period   string `json:"period,omitempty" jsonschema:"time period: 1d, 5d, 1mo, 3mo, 1y (default: 1mo)"`
	Interval string `json:"interval,omitempty" jsonschema:"data interval: 1m, 5m, 1h, 1d (default: 1d)"`
}

type DataPoint struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume int     `json:"volume"`
}

type GetHistoricalDataOutput struct {
	Symbol string      `json:"symbol"`
	Data   []DataPoint `json:"data"`
}

func getHistoricalData(_ context.Context, _ *mcp.CallToolRequest, in GetHistoricalDataInput) (*mcp.CallToolResult, GetHistoricalDataOutput, error) {
	return nil, GetHistoricalDataOutput{
		Symbol: in.Symbol,
		Data: []DataPoint{
			{Date: "2025-01-13", Open: 183.00, High: 185.00, Low: 182.50, Close: 184.75, Volume: 1200000},
			{Date: "2025-01-14", Open: 184.75, High: 186.50, Low: 184.00, Close: 185.50, Volume: 1350000},
			{Date: "2025-01-15", Open: 185.50, High: 187.25, Low: 185.00, Close: 186.00, Volume: 1100000},
		},
	}, nil
}
