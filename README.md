# MCP Variants (Experimental Extension)

> #### **Status:** Experimental. This work is for prototyping and feedback only, and is not an accepted or official MCP extension.

This repository provides a multi-language reference implementation of the variants proposal for the Model Context Protocol (MCP), as described in [SEP-2053](https://github.com/modelcontextprotocol/modelcontextprotocol/pull/2053).

## Implementations

| Language | Directory | Package | Status |
|----------|-----------|---------|--------|
| Go | `go/sdk/` | `github.com/modelcontextprotocol/experimental-ext-variants/go/sdk` | Planned |
| Python | `python/sdk/` | `mcp-ext-variants` | Planned |
| TypeScript | `typescript/sdk/` | `@ext-modelcontextprotocol/variants` | Planned |


## Examples (Go)

The Go implementation includes runnable example servers under [`go/sdk/examples/server/`](go/sdk/examples/server/):

| Example | Description |
|---|---|
| [`model-optimized/`](go/sdk/examples/server/model-optimized/) | Same tools, different descriptions per LLM family |
| [`github/`](go/sdk/examples/server/github/) | Different tool sets per variant with custom ranking ([demo](go/sdk/examples/server/github/mcp-inspector-variants-demo.mp4)) |
| [`research/`](go/sdk/examples/server/research/) | Context budget management via description verbosity |
| [`trading/`](go/sdk/examples/server/trading/) | API versioning, lifecycle statuses, and deprecation info |
| [`variants-stdio/`](go/sdk/examples/server/variants-stdio/) | Minimal single-variant setup |

## CI/CD

This monorepo uses **path-based CI workflows** to efficiently test only what changes:

### How It Works

1. **Language-specific workflows** (`python.yml`, `go.yml`, `typescript.yml`)
   - Only trigger when their language directory or workflow file changes
   - Run all tests, linting, and checks for that language

2. **Status check workflow** (`status-check.yml`)
   - Runs on every PR to verify required checks passed
   - Determines what needs to pass based on which files changed
   - This is the only required check in branch protection

### Examples

- Change `python/sdk/file.py` → Only Python CI runs → PR requires Python checks to pass
- Change both Go and TypeScript files → Both CIs run → PR requires both to pass
- Change only `README.md` → No language CIs run → PR can merge immediately

### Forcing All Checks

To run all language checks regardless of changed files:
- **In a PR**: Comment `/test all` (only works for repo owners/members/collaborators)
- **Manually**: Use GitHub Actions UI or CLI to trigger individual workflows

### Adding New Required Checks

1. **Add your check** to the appropriate language workflow (e.g., `python.yml`):
   ```yaml
   python-security-scan:
     name: "Security Scan"
     runs-on: ubuntu-latest
     steps:
       - name: Run security checks
         run: # your commands here
   ```

2. **Update the status check** in `.github/workflows/status-check.yml`:
   ```javascript
   const requiredChecks = {
     python: [
       'Python CI / Linting',
       'Python CI / Unit Tests (3.10)',
       // ... existing checks ...
       'Python CI / Security Scan'  // ← Add your new check
     ],
   ```

3. **Submit PR** - Your new check is now required for all relevant changes!

## License

Apache License 2.0 - See LICENSE file for details

## Resources

- [Variants Proposal Specification (SEP-2053)](https://github.com/modelcontextprotocol/modelcontextprotocol/pull/2053) - Full specification and design details
- [Model Context Protocol](https://modelcontextprotocol.io/specification)
