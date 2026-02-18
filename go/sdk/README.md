# MCP Variants - Go Implementation

Go implementation of [SEP-2053: Server Variants](https://github.com/modelcontextprotocol/modelcontextprotocol/pull/2053) — a server-level variant mechanism for MCP that lets servers expose multiple tool/resource/prompt sets simultaneously, selectable per-request via `_meta`.

## Quick Start

```go
package main

import (
    "log"
    "net/http"

    "github.com/modelcontextprotocol/go-sdk/mcp"
    "github.com/modelcontextprotocol/experimental-ext-variants/go/sdk/variants"
)

func main() {
    // Code review variant: PR-focused tools
    codeReviewServer := mcp.NewServer(&mcp.Implementation{Name: "devplatform", Version: "v1.0.0"}, nil)
    mcp.AddTool(codeReviewServer, &mcp.Tool{
        Name:        "list_pull_requests",
        Description: "List open pull requests, optionally filtered by author",
    }, listPullRequests)
    mcp.AddTool(codeReviewServer, &mcp.Tool{
        Name:        "get_diff",
        Description: "Get the diff for a pull request",
    }, getDiff)

    // Project management variant: issue-focused tools
    pmServer := mcp.NewServer(&mcp.Implementation{Name: "devplatform", Version: "v1.0.0"}, nil)
    mcp.AddTool(pmServer, &mcp.Tool{
        Name:        "list_issues",
        Description: "List issues, optionally filtered by state and labels",
    }, listIssues)
    mcp.AddTool(pmServer, &mcp.Tool{
        Name:        "create_issue",
        Description: "Create a new issue with title, body, and optional labels",
    }, createIssue)

    vs := variants.NewServer(&mcp.Implementation{Name: "devplatform", Version: "v1.0.0"}).
        WithVariant(variants.ServerVariant{
            ID:          "code-review",
            Description: "Pull request and code review operations. Includes diff viewing, review comments, approval workflows, and merge controls.",
            Hints:       map[string]string{"domain": "code-review", "accessLevel": "read-write"},
            Status:      variants.Stable,
        }, codeReviewServer, 0).
        WithVariant(variants.ServerVariant{
            ID:          "project-management",
            Description: "Issue and project tracking operations. Includes issue CRUD, labels, milestones, assignments, and project boards.",
            Hints:       map[string]string{"domain": "project-management", "accessLevel": "read-write"},
            Status:      variants.Stable,
        }, pmServer, 1)

    // Serve over HTTP (or use vs.Run(ctx, &mcp.StdioTransport{}) for stdio)
    handler := variants.NewStreamableHTTPHandler(vs, nil)
    log.Fatal(http.ListenAndServe(":8080", handler))
}
```

Clients select a variant per-request via `_meta`:

```json
{
  "method": "tools/list",
  "params": {
    "_meta": {
      "io.modelcontextprotocol/server-variant": "code-review"
    }
  }
}
```

Clients that don't know about variants get the default (first-ranked) variant automatically.

## How It Works

During `initialize`, the server responds with ranked `availableVariants`:

```json
{
  "capabilities": {
    "experimental": {
      "io.modelcontextprotocol/server-variants": {
        "availableVariants": [
          {
            "id": "code-review",
            "description": "Pull request and code review operations...",
            "hints": { "domain": "code-review", "accessLevel": "read-write" },
            "status": "stable"
          },
          {
            "id": "project-management",
            "description": "Issue and project tracking operations...",
            "hints": { "domain": "project-management", "accessLevel": "read-write" },
            "status": "stable"
          }
        ],
        "moreVariantsAvailable": false
      }
    }
  }
}
```

Each subsequent request can target a specific variant via `_meta`. The server routes the request to the appropriate backing `mcp.Server`:

```
Client ── transport ──▸ variants.Server ──▸ sessionMiddleware
                                                 │
                                    ┌────────────┼────────────┐
                                    │            │            │
                              initialize    route by     pass through
                              (create        _meta       (ping, etc.)
                               per-session   variant
                               connections)     │
                                                ▼
                                           dispatcher
                                                │
                          ┌────────────┬────────┴────────┬────────────┐
                          ▼            ▼                 ▼            ▼
                    code-review  project-mgmt   security-readonly  ci-automation
                    (mcp.Server) (mcp.Server)   (mcp.Server)       (mcp.Server)
```

In **stateful mode** (default, stdio and HTTP), per-session inner connections are created during `initialize` and scoped to the client session's lifetime. In **stateless mode** (via `NewStreamableHTTPHandler` with `Stateless: true`), a single set of shared connections is created at construction and reused across all requests.

## Features

- **Variant isolation**: each variant is a full `mcp.Server` with its own tools, resources, and prompts
- **Per-request selection**: variant chosen via `_meta` field, no session state needed
- **Default fallback**: clients without variant support get the first-ranked variant
- **Custom ranking**: provide a `RankingFunc` to rank variants based on client hints
- **Cursor scoping**: pagination cursors are variant-scoped and cannot be reused across variants (per SEP-2053)
- **Namespace scoping**: tool names, prompt names, and resource URIs resolve within the active variant's namespace; errors include `activeVariant` in error data
- **Notification forwarding**: progress and logging notifications from inner servers are forwarded to the front client with variant metadata injected
- **HTTP and stdio**: works with both `StdioTransport` and `StreamableHTTPHandler`

## Examples

See [`examples/server/`](examples/server/) for runnable examples:

- [`variants/`](examples/server/variants/) — stdio transport (single client)
- [`variants-http/`](examples/server/variants-http/) — HTTP transport (multiple concurrent clients)

## API

### Server

#### `variants.NewServer(impl *mcp.Implementation) *Server`

Creates a new variant-aware server with no registered variants. `impl` must not be nil.

#### `(*Server).WithVariant(v ServerVariant, mcpServer *mcp.Server, priority int) *Server`

Registers a variant backed by an in-memory `mcp.Server`. `priority` determines the default ordering when no `RankingFunc` is set — lower values rank higher (0 = highest priority). Panics on duplicate variant IDs. Returns the receiver for chaining.

#### `(*Server).WithRanking(fn RankingFunc) *Server`

Sets a custom ranking function used to order variants based on client hints during initialization. If nil, variants are ordered by priority value.

#### `(*Server).Variants() []ServerVariant`

Returns a copy of all registered variants in registration order.

#### `(*Server).RankedVariants(ctx context.Context, hints VariantHints) []ServerVariant`

Returns registered variants ranked by the configured `RankingFunc` (or default priority-based ranking).

#### `(*Server).Run(ctx context.Context, t mcp.Transport) error`

Starts the server on the given transport (e.g., `&mcp.StdioTransport{}`). For multi-client HTTP support, use `NewStreamableHTTPHandler` instead.

#### `(*Server).Close() error`

Releases resources held by all registered backends.

#### `variants.NewStreamableHTTPHandler(vs *Server, opts *mcp.StreamableHTTPOptions) *mcp.StreamableHTTPHandler`

Returns an `http.Handler` for serving multiple concurrent clients over HTTP. Pass `&mcp.StreamableHTTPOptions{Stateless: true}` for stateless mode.

### Types

#### `ServerVariant`

Describes a selectable variant:

```go
type ServerVariant struct {
    ID              string            `json:"id"`
    Description     string            `json:"description"`
    Hints           map[string]string `json:"hints,omitempty"`
    Status          VariantStatus     `json:"status,omitempty"`
    DeprecationInfo *DeprecationInfo  `json:"deprecationInfo,omitempty"`
}
```

`Priority() int` returns the priority value set during registration.

#### `VariantStatus`

```go
const (
    Stable       VariantStatus = "stable"
    Experimental VariantStatus = "experimental"
    Deprecated   VariantStatus = "deprecated"
)
```

#### `DeprecationInfo`

Migration guidance for deprecated variants:

```go
type DeprecationInfo struct {
    Message     string `json:"message"`
    Replacement string `json:"replacement,omitempty"`
    RemovalDate string `json:"removalDate,omitempty"`
}
```

#### `VariantHints`

Client-provided hints for variant selection, sent during `initialize`:

```go
type VariantHints struct {
    Description string         `json:"description,omitempty"`
    Hints       map[string]any `json:"hints,omitempty"`
}
```

#### `HintValue[T any](h VariantHints, key string) (T, bool)`

Generic helper to extract a typed value from a `VariantHints` map.

#### `RankingFunc`

```go
type RankingFunc func(ctx context.Context, hints VariantHints, variants []ServerVariant) []ServerVariant
```

Called during initialization to rank variants based on client hints. Must return variants sorted by relevance, most appropriate first.

#### Well-known hint keys

| Constant | Key | Example values |
|---|---|---|
| `HintModelFamily` | `"modelFamily"` | `"anthropic"`, `"openai"`, `"local"`, `"any"` |
| `HintUseCase` | `"useCase"` | `"autonomous-agent"`, `"ide"`, `"chat"` |
| `HintContextSize` | `"contextSize"` | `"compact"`, `"standard"`, `"verbose"` |
| `HintRenderingCapabilities` | `"renderingCapabilities"` | `"rich"`, `"markdown"`, `"text-only"` |
| `HintLanguageOptimization` | `"languageOptimization"` | `"en"`, `"multilingual"`, `"code-focused"` |

## Known Limitations

- **Default variant resolution**: When a client omits `_meta` variant selection, the server re-ranks variants with empty hints to determine the default. This may differ from the ranking returned during `initialize` (where client hints were used). Per SEP-2053, the default should be the first variant from the `initialize` response. To fix this, the per-session ranked order needs to be stored during `initialize` and reused for subsequent requests.
- **List-changed notifications**: Dynamic capability changes from inner servers (tool/resource/prompt list changes) are not forwarded to front clients. The Go MCP SDK does not expose generic notification sending on `ServerSession`. In practice this is acceptable because inner servers are typically statically configured.
- **HTTP and remote backends**: `WithHTTPVariant` and `WithRemoteVariant` are not yet implemented.
