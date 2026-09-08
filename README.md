# freshdesk-mcp

A self-hosted [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) server that exposes Freshdesk support data to MCP-compatible AI clients (Claude, Cursor, VS Code Copilot, etc.).

## What it does

Connects your AI assistant directly to Freshdesk — search tickets, read conversations, extract text from attachments (including OCR for screenshots), and enrich results with status names, group names, and company IDs.

```
AI client → HTTPS POST /mcp → freshdesk-mcp → Freshdesk API
                                             → GCP Vision OCR (image attachments)
```

## Available Tools

| Tool | Description |
|------|-------------|
| `get_ticket` | Fetch a single ticket by ID. Returns status name, group name, company ID. |
| `get_ticket_summary` | Ticket + all conversations + attachments in one call. |
| `batch_get_ticket_summaries` | Multiple ticket summaries in parallel. |
| `classify_ticket` | Classify a ticket type using AI (requires `ML_PROVIDER`). |
| `suggest_resolution` | Suggest resolution, draft reply and next steps using AI (requires `ML_PROVIDER`). |
| `search_tickets` | Filter by query, status, priority, type, overdue, escalated, dates, company, requester, agent, group. Full ticket history. |
| `list_tickets` | All tickets with optional filters. |
| `get_conversations` | All replies and notes for a ticket (fully paginated). |
| `list_attachments` | All attachments across ticket and conversations. Filters scanning placeholders. |
| `get_attachment_text` | Extract text from docx, xlsx, json, txt, csv, png, jpg (Vision OCR). |
| `query_attachment` | Search within a JSON attachment. |
| `find_image_attachments` | Scan multiple tickets for image attachments. |
| `get_description_images` | OCR inline images in ticket description HTML and conversation replies. |
| `find_requester` | Search contacts by name/email → requester_id. |
| `find_company` | Search companies by name → company_id. |
| `find_agent` | Search agents by name/email → agent_id. ⚠️ Requires admin API key. |
| `list_groups` | List all agent groups → group_id. ⚠️ Requires admin API key. |

> **Note:** `find_agent` and `list_groups` use Freshdesk admin-only endpoints (`/api/v2/agents`, `/api/v2/groups`). They work if your API key belongs to an admin account. Regular agent API keys will receive a 403.

## Prerequisites

- A Freshdesk account with API access
- Docker
- A GCP project with the [Vision API](https://cloud.google.com/vision) enabled (optional — needed for image OCR only)
- A container runtime — Cloud Run, AWS App Runner, Azure Container Apps, or plain Docker

## Configuration

| Variable | Required | Description |
|----------|----------|-------------|
| `FRESHDESK_DOMAIN` | yes | Freshdesk subdomain only — e.g. `mycompany` (not `mycompany.freshdesk.com`) |
| `FRESHDESK_API_KEY` | yes | Freshdesk API key. Find it at Profile → Profile Settings → API Key. |
| `MCP_TOKEN` | yes (HTTP mode) | Bearer token your MCP client sends in `Authorization: Bearer <token>`. Generate any random secret. |
| `GCP_VISION_PROJECT` | no | GCP project ID for Vision OCR. If unset, image OCR tools return an error. |
| `MCP_TRANSPORT` | no | Set to `http` for HTTP transport (default in the Docker image). |
| `PORT` | no | HTTP port (default `8080`). Set automatically by Cloud Run. |
| `ML_PROVIDER` | no | AI provider for ticket classification: `claude`, `ollama`. Unset disables ML tools. |
| `ANTHROPIC_API_KEY` | no | Anthropic API key — required when `ML_PROVIDER=claude`. |
| `OLLAMA_URL` | no | Ollama base URL (default `http://localhost:11434`) — used when `ML_PROVIDER=ollama`. |
| `OLLAMA_MODEL` | no | Ollama model name e.g. `llama3.2`, `qwen2.5:1.5b` — required when `ML_PROVIDER=ollama`. |
| `ML_SYSTEM_PROMPT` | no | Custom system prompt for ML tools. Defaults to a generic support engineer persona. |

## Deployment

### Docker (any platform)

```bash
# Pull pre-built image
docker pull ghcr.io/omnom62/freshdesk-mcp:latest

# Or build from source
docker build -t freshdesk-mcp .

docker run -p 8080:8080 \
  -e MCP_TRANSPORT=http \
  -e FRESHDESK_DOMAIN=mycompany \
  -e FRESHDESK_API_KEY=your-api-key \
  -e MCP_TOKEN=your-random-secret \
  -e GCP_VISION_PROJECT=my-gcp-project \
  freshdesk-mcp
```

### GCP Cloud Run

Build and push to Artifact Registry, then deploy:

```bash
IMAGE="<region>-docker.pkg.dev/<project>/freshdesk-mcp/freshdesk-mcp"

docker build -t ${IMAGE}:latest .
DIGEST=$(docker push ${IMAGE}:latest | grep "digest:" | awk '{print $3}')

gcloud run deploy freshdesk-mcp \
  --image ${IMAGE}@${DIGEST} \
  --region <region> \
  --platform managed \
  --allow-unauthenticated \
  --set-secrets FRESHDESK_DOMAIN=FRESHDESK_DOMAIN:latest,FRESHDESK_API_KEY=FRESHDESK_API_KEY:latest,MCP_TOKEN=MCP_TOKEN:latest \
  --set-env-vars GCP_VISION_PROJECT=<project> \
  --project <project>
```

Store secrets in GCP Secret Manager and grant the Cloud Run runtime service account `roles/secretmanager.secretAccessor` on each secret.

### AWS App Runner / Azure Container Apps

Deploy the Docker image to your preferred platform. Pass the configuration variables as environment variables or secrets via your platform's secret store.

## Connecting your MCP client

Once deployed, add to your MCP client config:

```json
{
  "mcpServers": {
    "freshdesk": {
      "type": "http",
      "url": "https://your-deployment-url/mcp",
      "headers": {
        "Authorization": "Bearer your-mcp-token"
      }
    }
  }
}
```

## Secret rotation

When rotating the Freshdesk API key:

1. Get the new key from Freshdesk → Profile → Profile Settings → API Key
2. Verify it works: `curl -s -u "<new-key>:X" "https://<domain>.freshdesk.com/api/v2/tickets?per_page=1"`
3. Update your secret store (e.g. GCP Secret Manager: `echo -n "<new-key>" | gcloud secrets versions add FRESHDESK_API_KEY --data-file=- --project <project>`)
4. Redeploy to pick up the new version

> **Tip:** Never include `:X` in the stored secret value — that suffix is only used in curl Basic auth syntax.

## Local development

```bash
export FRESHDESK_DOMAIN=mycompany
export FRESHDESK_API_KEY=your-api-key
export MCP_TOKEN=dev-token
export GCP_VISION_PROJECT=my-gcp-project  # optional

go run .
```

Server starts on port 8080. MCP endpoint: `POST /mcp`.

## License

MIT
