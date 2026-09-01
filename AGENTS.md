# CLAUDE.md

This file guides AI agents working in this repository.

## What this is

A self-hosted MCP (Model Context Protocol) server that exposes Freshdesk support data to AI clients. Written in Go, deployed as a stateless HTTP server on Cloud Run (or any container platform).

## Build and run

```bash
# Build
go build -o freshdesk-mcp .

# Run locally
MCP_TRANSPORT=http \
MCP_TOKEN=dev-token \
FRESHDESK_DOMAIN=yourcompany \
FRESHDESK_API_KEY=your-key \
GCP_VISION_PROJECT=your-project \
./freshdesk-mcp

# Test
go test ./...

# Lint
golangci-lint run ./...
```

## Project structure

```
main.go                        — MCP server, tool definitions, HTTP handler
internal/
  freshdesk/
    client.go                  — Freshdesk API client (HTTP, rate limiting, pagination)
    filter.go                  — In-memory ticket filtering
  extract/
    extract.go                 — Routes attachment extraction by type
    docx.go                    — Word document text extraction
    xlsx.go                    — Excel spreadsheet text extraction
    image.go                   — Image OCR via GCP Vision API
    json.go                    — JSON pretty printing
    query.go                   — JSON search
  cache/
    cache.go                   — Generic TTL cache Cache[K, V]
```

## Code conventions

**Errors:**
- Always wrap errors with context: `fmt.Errorf("operation: %w", err)`
- Define sentinel errors as package-level vars: `var ErrFoo = errors.New("foo")`
- Use `errors.Is` for comparison, never `==`
- Never swallow errors silently — log or return them

**Goroutines:**
- Use `errgroup.WithContext` for parallel work, never raw goroutines
- Always pass `ctx` as the first argument to every function that does I/O
- Cancel context on error — `errgroup` handles this automatically

**MCP tools:**
- Each tool has a `Name`, `Description`, input struct, output struct, and handler func
- Tool descriptions are read by the AI to decide when to call the tool — make them precise and include examples
- Input/output structs use `json` tags with `omitempty` for optional fields
- Handler functions follow: `func(ctx, req, input) (*mcp.CallToolResult, Output, error)`

**Logging:**
- Use `slog` with typed attributes: `slog.String("key", val)`, `slog.Any("err", err)`
- Never use key-value string pairs: `slog.Info("msg", "key", val)` — use `slog.Info("msg", slog.String("key", val))`

**Formatting:**
- Max line length: 120 chars (enforced by golines)
- Import order: stdlib, then external packages (enforced by gci)
- Run `gci write <file>` and `golines -w --max-len=120 <file>` after editing imports or long lines

## Linting

The project uses `golangci-lint` with a strict config in `.golangci.yml`. Before committing:

```bash
golangci-lint run ./...
```

Key rules enforced:
- `err113` — no dynamic errors, use sentinel errors with `%w` wrapping
- `errcheck` — all error return values must be checked
- `errorlint` — use `errors.Is` not `==` for error comparison
- `sloglint` — use typed slog attributes, not key-value pairs
- `gocognit` — cognitive complexity limit (use `//nolint:gocognit` with reason for large functions like `buildServer`)

## What NOT to do

- Do not add corp-specific config (wb-sandbox-1, aupdns-support, whalebone.io) — keep it generic
- Do not commit binaries, `.bak` files, `deploy.sh`, `.mcp.json`, or `freshdesk-mcp-guide.md` — all gitignored
- Do not add the ticket cache back — the current approach fetches fresh data per request
- Do not use `log.Printf` or `fmt.Println` for logging — use `slog`
- Do not compare errors with `==` — use `errors.Is`
- Do not create goroutines without a context and error handling

## Adding a new MCP tool

1. Define input and output structs with `json` tags
2. Add the tool with `mcp.AddTool(server, &mcp.Tool{Name: ..., Description: ...}, handler)`
3. Keep the description precise — the AI reads it to decide when to call the tool
4. Add the tool to the tools table in `README.md`

## Adding a new attachment extractor

1. Add a new file in `internal/extract/`
2. Export a function `func Foo(data []byte) (string, error)`
3. Add the file extension and content type to the `switch` in `extract.go`
4. Add a test case in `extract_test.go`

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `FRESHDESK_DOMAIN` | yes | Subdomain only (e.g. `mycompany`) |
| `FRESHDESK_API_KEY` | yes | Freshdesk API key |
| `MCP_TOKEN` | yes | Bearer token for MCP client auth |
| `GCP_VISION_PROJECT` | no | GCP project for Vision OCR |
| `MCP_TRANSPORT` | no | `http` (default in Docker) or unset for stdio |
| `PORT` | no | HTTP port (default 8080) |
