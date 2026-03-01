// Example: Minimal variant-aware MCP server over stdio. Registers a single
// variant with one tool to demonstrate the simplest possible setup.
//
// Run over stdio:
//
//	go run ./examples/server/variants-stdio
package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/modelcontextprotocol/experimental-ext-variants/go/sdk/variants"
)

type GreetInput struct {
	Name string `json:"name" jsonschema:"name to greet"`
}

type GreetOutput struct {
	Message string `json:"message"`
}

func greet(_ context.Context, _ *mcp.CallToolRequest, in GreetInput) (*mcp.CallToolResult, GreetOutput, error) {
	return nil, GreetOutput{Message: "Hello, " + in.Name + "!"}, nil
}

func main() {
	inner := mcp.NewServer(&mcp.Implementation{Name: "greeter", Version: "v1.0.0"}, nil)
	mcp.AddTool(inner, &mcp.Tool{
		Name:        "greet",
		Description: "Greet someone by name",
	}, greet)

	vs := variants.NewServer(&mcp.Implementation{Name: "greeter", Version: "v1.0.0"}).
		WithVariant(variants.ServerVariant{
			ID:          "default",
			Description: "Default greeting variant.",
			Status:      variants.Stable,
		}, inner, 0)

	if err := vs.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
