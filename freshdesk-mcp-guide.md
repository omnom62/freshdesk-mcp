# Freshdesk MCP Server — Project Guide

A practical guide to what was built, how it works, and the Go and MCP concepts used.

---

## 1. What is this project?

A **Go HTTP server** that exposes Freshdesk support ticket data to AI agents (VS Code Copilot) via the **Model Context Protocol (MCP)**.

```
VS Code Copilot Agent
        ↓  HTTPS POST /mcp
Cloud Run (Go server)
        ↓  REST API
Freshdesk + GCP Vision OCR
```

Instead of querying Freshdesk manually, an AI agent can ask natural language questions like:
- "What are all the updates since yesterday?"
- "How many False Negative tickets did TID submit this month?"
- "OCR the screenshot in ticket 3202"

---

## 2. Key Concepts

### MCP — Model Context Protocol
A protocol (from Anthropic) that lets AI agents call external "tools" — functions that return structured data. Think of it like a REST API but designed specifically for AI agents.

- The agent sends a JSON-RPC request: `{"method": "tools/call", "params": {"name": "search_tickets", "arguments": {...}}}`
- The server executes the tool and returns structured JSON
- The agent uses the result to answer the user's question

### JSON-RPC
A lightweight remote procedure call protocol using JSON. Every MCP message is a JSON-RPC request or response:
```json
{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {...}}
```

### SSE — Server-Sent Events
The HTTP transport used by MCP. The server streams responses back as `data: {...}` lines. This allows long-running tool calls to stream results progressively.

### Bearer Token Auth
The server requires `Authorization: Bearer <token>` on every request. The token is stored in GCP Secret Manager and injected at runtime.

### Cloud Run
A GCP serverless container platform. You push a Docker image, Cloud Run runs it and scales automatically. No servers to manage. Handles HTTPS, load balancing, and graceful shutdown via SIGTERM.

### Secret Manager
GCP service for storing secrets (API keys, tokens). Cloud Run injects them as environment variables at startup — secrets never appear in code or Docker images.

---

## 3. Project Structure

```
freshdesk-mcp/
├── main.go                      # MCP server, all tool definitions, HTTP/stdio server
├── internal/
│   ├── freshdesk/
│   │   └── client.go            # Freshdesk REST API client
│   ├── cache/
│   │   └── cache.go             # Generic TTL cache (Go generics)
│   └── extract/
│       ├── extract.go           # Router: picks extractor by file type
│       ├── docx.go              # Word document text extraction
│       ├── xlsx.go              # Excel spreadsheet extraction
│       ├── json.go              # JSON pretty-print
│       ├── query.go             # JSON client-side filtering
│       └── image.go             # GCP Vision OCR
├── Dockerfile                   # Multi-stage build: golang:alpine → alpine
├── go.mod                       # Module definition and dependencies
└── go.sum                       # Dependency checksums (security)
```

---

## 4. Go Concepts Used

### Packages and Modules
Go code is organised into **packages**. The `internal/` directory contains packages that can only be imported by this module — Go enforces this at compile time. `go.mod` defines the module name and Go version.

### Structs and JSON Tags
```go
type Ticket struct {
    ID      int64  `json:"id"`
    Subject string `json:"subject"`
    Status  int    `json:"status"`
}
```
The backtick annotations (`json:"id"`) tell the `encoding/json` package how to serialise/deserialise. `omitempty` skips the field if it's a zero value.

### Interfaces
`http.Handler` is an interface — any type with a `ServeHTTP(w, r)` method satisfies it. This is how middleware works:
```go
func authMiddleware(token string, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // check token, then call next
        next.ServeHTTP(w, r)
    })
}
```

### Context
`context.Context` is passed through every function call. It carries:
- **Cancellation** — if the HTTP request is cancelled, all downstream work stops
- **Deadlines** — time limits for operations
- **Values** — rarely used, but possible

Always the first parameter by convention: `func (c *Client) GetTicket(ctx context.Context, id int64)`.

### Goroutines and errgroup
A **goroutine** is a lightweight concurrent function: `go func() { ... }()`.

`errgroup` (from `golang.org/x/sync`) manages a group of goroutines and collects the first error:
```go
g, ctx := errgroup.WithContext(ctx)

g.Go(func() error {
    ticket, err = client.GetTicket(ctx, id)
    return err
})

g.Go(func() error {
    convs, err = client.GetConversations(ctx, id)
    return err
})

if err := g.Wait(); err != nil {  // blocks until all goroutines finish
    return err
}
```
`get_ticket_summary` uses this to fetch ticket + conversations + attachments in parallel.

### Generics
The cache uses Go generics (1.18+) to work with any key/value type:
```go
type Cache[K comparable, V any] struct { ... }

cache.New[string, []Ticket](5 * time.Minute)
cache.New[string, []byte](10 * time.Minute)
```
The `[K comparable, V any]` syntax defines type parameters.

### Pointers
`*bool` (pointer to bool) is used for optional boolean fields in tool inputs. A `bool` has two states (true/false); a `*bool` has three (true/false/nil — not provided). This matters for JSON schema generation.

### Error Wrapping
```go
return nil, fmt.Errorf("get_ticket: %w", err)
```
`%w` wraps the error, preserving the original for `errors.Is()` / `errors.As()` checks while adding context.

### Rate Limiting
`golang.org/x/time/rate` implements a token bucket limiter:
```go
limiter := rate.NewLimiter(rate.Every(time.Minute/40), 1)
limiter.Wait(ctx)  // blocks if over 40 req/min
```

### Graceful Shutdown
```go
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
<-ctx.Done()  // blocks until SIGTERM arrives (Cloud Run sends this before killing the container)
httpServer.Shutdown(shutdownCtx)  // finish in-flight requests
```

### Structured Logging
`slog` (standard library since Go 1.21) outputs JSON logs:
```go
slog.Info("freshdesk-mcp ready")
slog.Error("shutdown error", "err", err)
```
JSON logs are readable by GCP Cloud Logging.

---

## 5. The MCP SDK

Uses `github.com/modelcontextprotocol/go-sdk/mcp`. Key patterns:

```go
// Create server
server := mcp.NewServer(&mcp.Implementation{Name: "freshdesk-mcp"}, nil)

// Register a tool
mcp.AddTool(server,
    &mcp.Tool{
        Name:        "search_tickets",
        Description: "...",
    },
    func(ctx context.Context, req *mcp.CallToolRequest, input SearchTicketsInput) (*mcp.CallToolResult, SearchTicketsOutput, error) {
        // input is automatically deserialised from JSON
        // return output is automatically serialised to JSON
    },
)

// Serve over HTTP
handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
    return server
}, nil)
```

The SDK generates JSON Schema from your input structs automatically. `omitempty` on struct tags makes fields optional in the schema.

---

## 6. The Freshdesk Client

`internal/freshdesk/client.go` wraps the Freshdesk REST API:

- **Authentication**: HTTP Basic Auth with API key as username, "X" as password
- **Rate limiting**: 40 req/min outgoing limiter + exponential backoff retry on 429
- **Caching**: tickets cached for 5 minutes, attachments for 10 minutes (skipped when `updated_since` is set)
- **Pagination**: all list endpoints fetch all pages automatically
- **Redirect following**: `http.Client` follows 302 redirects automatically (needed for inline image downloads)

### Key methods:
| Method | Description |
|--------|-------------|
| `GetTicket(ctx, id)` | Single ticket by ID |
| `ListTickets(ctx, updatedSince)` | All tickets, optional updated_since filter |
| `SearchTickets(ctx, filter)` | Client-side filter on top of ListTickets |
| `GetConversations(ctx, ticketID)` | All replies and notes |
| `GetAllAttachments(ctx, ticketID)` | Attachments from ticket + all conversations |
| `DownloadAttachment(ctx, url)` | Download file with caching |
| `DownloadInlineAttachment(ctx, url)` | Download inline image with API key auth |
| `SearchContacts(ctx, query)` | Paginated contact search |
| `SearchCompanies(ctx, query)` | Paginated company search |
| `ListGroups(ctx)` | All agent groups |
| `ExtractInlineImageURLs(html)` | Parse `<img>` tags from HTML (static function) |

---

## 7. The Extract Package

`internal/extract/` routes attachments to the right extractor:

```go
func FromAttachment(ctx, name, contentType, data, gcpProject) (string, error)
```

| Type | Extractor |
|------|-----------|
| `.docx` | `unioffice` library — extracts paragraph text |
| `.xlsx` | `excelize` library — tab-separated rows per sheet |
| `.json` | `encoding/json` — pretty print |
| `.txt`, `.csv` | raw `string(data)` |
| `.png`, `.jpg`, `.jpeg` | GCP Vision API `BatchAnnotateImages` |

### GCP Vision OCR
```go
client.BatchAnnotateImages(ctx, &visionpb.BatchAnnotateImagesRequest{
    Requests: []*visionpb.AnnotateImageRequest{{
        Image:    &visionpb.Image{Content: data},
        Features: []*visionpb.Feature{{Type: visionpb.Feature_TEXT_DETECTION}},
    }},
})
// Returns fullTextAnnotation.Text — the complete extracted text
```

---

## 8. The 14 MCP Tools

| Tool | What it does |
|------|-------------|
| `get_ticket` | Single ticket by ID |
| `get_ticket_summary` | Ticket + conversations + attachments in one parallel call |
| `batch_get_ticket_summaries` | Multiple ticket summaries in one call (parallel server-side) |
| `search_tickets` | Filter by status, priority, type, overdue, escalated, dates, company, requester, group, updated_since |
| `list_tickets` | All tickets with filters for reporting |
| `get_conversations` | All replies and notes for a ticket |
| `list_attachments` | All attachments (ticket + conversations) |
| `get_attachment_text` | Extract text from docx/xlsx/json/txt/csv/png/jpg |
| `query_attachment` | Search within JSON attachment |
| `find_image_attachments` | Scan multiple tickets for image attachments |
| `get_description_images` | OCR inline images embedded in ticket description HTML |
| `find_requester` | Search contacts by name/email → requester_id |
| `find_company` | Search companies by name → company_id |
| `list_groups` | List all agent groups → group_id |

---

## 9. Deployment

### Dockerfile (multi-stage build)
```dockerfile
# Stage 1: Build
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download          # cached layer — only re-runs if go.mod changes
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o freshdesk-mcp .

# Stage 2: Runtime (tiny image, no Go toolchain)
FROM alpine:3.19
RUN apk --no-cache add ca-certificates   # needed for HTTPS calls
WORKDIR /app
COPY --from=builder /app/freshdesk-mcp .
CMD ["./freshdesk-mcp"]
```

`CGO_ENABLED=0` produces a statically linked binary with no C dependencies — runs on minimal Alpine.

### Environment Variables
| Variable | Source | Purpose |
|----------|--------|---------|
| `FRESHDESK_DOMAIN` | Secret Manager | Freshdesk subdomain |
| `FRESHDESK_API_KEY` | Secret Manager | Freshdesk API key |
| `MCP_TOKEN` | Secret Manager | Bearer token for MCP auth |
| `GCP_VISION_PROJECT` | Cloud Run env var | GCP project for Vision OCR |
| `MCP_TRANSPORT` | Cloud Run env var (implicit) | Set to `http` in container |
| `PORT` | Cloud Run (automatic) | HTTP port (default 8080) |

### Deploy Command
```bash
docker build --no-cache -t omnom62/freshdesk-mcp:latest .
docker push omnom62/freshdesk-mcp:latest
DIGEST=$(docker inspect omnom62/freshdesk-mcp:latest --format='{{index .RepoDigests 0}}')
gcloud run deploy freshdesk-mcp \
  --image $DIGEST \
  --region australia-southeast1 \
  --set-secrets FRESHDESK_DOMAIN=FRESHDESK_DOMAIN:latest,...
  --set-env-vars GCP_VISION_PROJECT=wb-sandbox-1
```

Using the image digest (not `:latest` tag) ensures Cloud Run always pulls the exact image you built.

---

## 10. What to Learn Next

To fully understand this codebase, these are the Go topics worth studying in order:

1. **Go tour** — https://go.dev/tour — basics: types, functions, structs, interfaces
2. **Error handling** — Go's explicit error returns vs exceptions
3. **Goroutines and channels** — Go's concurrency model
4. **HTTP in Go** — `net/http` package, handlers, middleware
5. **Context** — cancellation and deadlines
6. **Generics** — type parameters (used in cache)
7. **Testing** — `testing` package, table-driven tests (next milestone for this project)

### MCP Resources
- MCP specification: https://spec.modelcontextprotocol.io
- Go SDK: https://github.com/modelcontextprotocol/go-sdk

### GCP Resources
- Cloud Run: https://cloud.google.com/run/docs
- Secret Manager: https://cloud.google.com/secret-manager/docs
- Vision API: https://cloud.google.com/vision/docs

---

## 11. Architecture Diagram

```
┌─────────────────────────────────────────────────────────┐
│  VS Code + GitHub Copilot Agent                         │
│  .vscode/mcp.json → URL + Bearer token                  │
└────────────────────────┬────────────────────────────────┘
                         │ HTTPS POST /mcp (JSON-RPC)
                         ▼
┌─────────────────────────────────────────────────────────┐
│  Cloud Run (australia-southeast1)                       │
│  ┌─────────────────────────────────────────────────┐    │
│  │  Go HTTP Server (main.go)                       │    │
│  │  ┌─────────────┐  ┌──────────────────────────┐  │    │
│  │  │ Auth        │  │ MCP SDK                  │  │    │
│  │  │ Middleware  │→ │ 14 Tools                 │  │    │
│  │  └─────────────┘  └──────────┬───────────────┘  │    │
│  │                              │                   │    │
│  │  ┌───────────────────────────▼─────────────────┐ │    │
│  │  │ internal/freshdesk/client.go                │ │    │
│  │  │ Rate limiter │ Cache │ Retry │ Pagination   │ │    │
│  │  └───────────────────────────┬─────────────────┘ │    │
│  │                              │                   │    │
│  │  ┌───────────────────────────▼─────────────────┐ │    │
│  │  │ internal/extract/                           │ │    │
│  │  │ docx │ xlsx │ json │ txt │ csv │ image OCR  │ │    │
│  │  └─────────────────────────────────────────────┘ │    │
│  └─────────────────────────────────────────────────┘    │
└──────────────┬──────────────────────────┬───────────────┘
               │                          │
               ▼                          ▼
┌──────────────────────┐    ┌─────────────────────────────┐
│  Freshdesk API       │    │  GCP Vision API             │
│  aupdns-support      │    │  wb-sandbox-1               │
│  .freshdesk.com      │    │  (OCR for images)           │
└──────────────────────┘    └─────────────────────────────┘
               ▲
               │ Secrets injected at startup
┌──────────────────────┐
│  GCP Secret Manager  │
│  FRESHDESK_DOMAIN    │
│  FRESHDESK_API_KEY   │
│  MCP_TOKEN           │
└──────────────────────┘
```
