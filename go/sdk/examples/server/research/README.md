# Research Assistant

A variant-aware MCP server that manages context budget by providing the same research tools (`search_papers`, `get_paper`, `summarize`) at different verbosity levels.

Each tool sends progress notifications and log messages as it works, simulating a deep-research workflow where the client can observe each step (e.g. "Searching arXiv", "Resolving references", "Generating summary").

**Patterns demonstrated:** Context budget management with description verbosity control, notification streaming.

## Variants

| Variant | Verbosity | Status | Use Case |
|---|---|---|---|
| `deep-research` | Multi-paragraph with usage examples | Stable | Literature reviews, large context windows |
| `quick-lookup` | Single-sentence descriptions | Stable | Fast Q&A, limited token budgets |
| `synthesis` | Balanced, moderate detail | Experimental | Report generation workflows |

All three variants expose the same three tools with the same behavior — only the description detail level differs.

## Custom Ranking

Clients send a `"contextSize"` hint (`"verbose"`, `"compact"`, or `"standard"`). The ranking function matches the hint to the appropriate variant, falling back to priority order.

## Run

```bash
go run ./examples/server/research
```

The server listens on `http://localhost:8080`.
