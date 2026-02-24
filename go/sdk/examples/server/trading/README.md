# Trading Platform

A variant-aware MCP server demonstrating API versioning and lifecycle management. Uses all three variant statuses (`stable`, `experimental`, `deprecated`) and `DeprecationInfo` to guide clients through API migrations.

**Pattern demonstrated:** Variant lifecycle, deprecation info, API versioning.

## Variants

| Variant | Status | Tools | Description |
|---|---|---|---|
| `v2-stable` | Stable | `place_order`, `get_quotes`, `get_portfolio`, `cancel_order` | Production API |
| `v3-preview` | Experimental | `place_order_v3`, `stream_quotes`, `get_portfolio_v3` | Next-gen API with streaming |
| `v1-legacy` | Deprecated | `trade`, `quote`, `balance` | Legacy API, removal date 2026-06-30 |
| `analysis-only` | Stable | `get_quotes`, `get_portfolio`, `get_historical_data` | Read-only analytics subset |

## Deprecation Info

The `v1-legacy` variant includes `DeprecationInfo` with a replacement and scheduled removal date, demonstrating how servers communicate migration paths to clients:

```json
{
  "deprecationInfo": {
    "message": "v1 API is deprecated. Migrate to v2-stable for improved order types, multi-symbol quotes, and portfolio tracking.",
    "replacement": "v2-stable",
    "removalDate": "2026-06-30"
  }
}
```

## Run

```bash
go run ./examples/server/trading
```

The server listens on `http://localhost:8080`.
