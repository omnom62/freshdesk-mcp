package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/whalebone/freshdesk-mcp/internal/extract"
	"github.com/whalebone/freshdesk-mcp/internal/freshdesk"
	"golang.org/x/sync/errgroup"
)

type GetTicketInput struct {
	TicketID int64 `json:"ticket_id"`
}

type GetTicketSummaryInput struct {
	TicketID int64 `json:"ticket_id"`
}

type GetTicketSummaryOutput struct {
	Ticket        GetTicketOutput      `json:"ticket"`
	Conversations []ConversationOutput `json:"conversations"`
	Attachments   []AttachmentInfo     `json:"attachments"`
}

type GetTicketOutput struct {
	ID              int64          `json:"id"`
	Subject         string         `json:"subject"`
	Status          int            `json:"status"`
	Type            string         `json:"type"`
	Priority        int            `json:"priority"`
	DueBy           string         `json:"due_by"`
	IsEscalated     bool           `json:"is_escalated"`
	CreatedAt       string         `json:"created_at"`
	Tags            []string       `json:"tags"`
	CustomFields    map[string]any `json:"custom_fields"`
	DescriptionText string         `json:"description_text"`
}

type SearchTicketsInput struct {
	Query         string `json:"query,omitempty"`
	Status        int    `json:"status,omitempty"`
	Priority      int    `json:"priority,omitempty"`
	Overdue       *bool  `json:"overdue,omitempty"`
	Type          string `json:"type,omitempty"`
	IsEscalated   *bool  `json:"is_escalated,omitempty"`
	CreatedAfter  string `json:"created_after,omitempty"`
	CreatedBefore string `json:"created_before,omitempty"`
	UpdatedSince  string `json:"updated_since,omitempty"`
	RequesterID   int64  `json:"requester_id,omitempty"`
	CompanyID     int64  `json:"company_id,omitempty"`
	GroupID       int64  `json:"group_id,omitempty"`
}

type SearchTicketsOutput struct {
	Total   int               `json:"total"`
	Results []GetTicketOutput `json:"results"`
}

type GetConversationsInput struct {
	TicketID int64 `json:"ticket_id"`
}

type ConversationOutput struct {
	ID        int64  `json:"id"`
	BodyText  string `json:"body_text"`
	Incoming  bool   `json:"incoming"`
	Private   bool   `json:"private"`
	CreatedAt string `json:"created_at"`
}

type GetConversationsOutput struct {
	TicketID      int64                `json:"ticket_id"`
	Conversations []ConversationOutput `json:"conversations"`
}

type GetAttachmentTextInput struct {
	TicketID     int64 `json:"ticket_id"`
	AttachmentID int64 `json:"attachment_id"`
}

type GetAttachmentTextOutput struct {
	AttachmentID int64  `json:"attachment_id"`
	Name         string `json:"name"`
	Text         string `json:"text"`
}

type ListAttachmentsInput struct {
	TicketID int64 `json:"ticket_id"`
}

type AttachmentInfo struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

type ListAttachmentsOutput struct {
	TicketID    int64            `json:"ticket_id"`
	Attachments []AttachmentInfo `json:"attachments"`
}

type QueryAttachmentInput struct {
	TicketID     int64  `json:"ticket_id"`
	AttachmentID int64  `json:"attachment_id"`
	Query        string `json:"query"`
}

type QueryAttachmentOutput struct {
	AttachmentID int64  `json:"attachment_id"`
	Name         string `json:"name"`
	Query        string `json:"query"`
	Result       string `json:"result"`
}

type FindImageAttachmentsInput struct {
	TicketIDs []int64 `json:"ticket_ids"`
}

type ImageAttachmentInfo struct {
	TicketID     int64  `json:"ticket_id"`
	AttachmentID int64  `json:"attachment_id"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
}

type FindImageAttachmentsOutput struct {
	Total   int                   `json:"total"`
	Results []ImageAttachmentInfo `json:"results"`
}

type FindRequesterInput struct {
	Query string `json:"query"`
}

type RequesterOutput struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type FindRequesterOutput struct {
	Total   int               `json:"total"`
	Results []RequesterOutput `json:"results"`
}

type FindCompanyInput struct {
	Query string `json:"query"`
}

type CompanyOutput struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
}

type FindCompanyOutput struct {
	Total   int             `json:"total"`
	Results []CompanyOutput `json:"results"`
}

type ListTicketsInput struct {
	Status        int    `json:"status,omitempty"`
	Priority      int    `json:"priority,omitempty"`
	Type          string `json:"type,omitempty"`
	CreatedAfter  string `json:"created_after,omitempty"`
	CreatedBefore string `json:"created_before,omitempty"`
	UpdatedSince  string `json:"updated_since,omitempty"`
}

type ListTicketsOutput struct {
	Total   int               `json:"total"`
	Results []GetTicketOutput `json:"results"`
}

type GetDescriptionImagesInput struct {
	TicketID int64 `json:"ticket_id"`
}

type InlineImageResult struct {
	URL  string `json:"url"`
	Text string `json:"text"`
}

type GetDescriptionImagesOutput struct {
	TicketID int64               `json:"ticket_id"`
	Total    int                 `json:"total"`
	Images   []InlineImageResult `json:"images"`
}

type ListGroupsOutput struct {
	Total   int               `json:"total"`
	Results []freshdesk.Group `json:"results"`
}


func buildServer(client *freshdesk.Client, gcpVisionProject string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "freshdesk-mcp",
		Version: "v0.1.0",
	}, nil)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "get_ticket",
			Description: "Retrieve a single Freshdesk support ticket by its numeric ID. Returns full details including id, subject, status, type, priority, due_by, is_escalated, created_at, tags and custom_fields (cf_impact, cf_urgency, cf_category, cf_subcategory, cf_domain).",
		},
		func(ctx context.Context, req *mcp.CallToolRequest, input GetTicketInput) (*mcp.CallToolResult, GetTicketOutput, error) {
			ticket, err := client.GetTicket(ctx, input.TicketID)
			if err != nil {
				return nil, GetTicketOutput{}, fmt.Errorf("get_ticket: %w", err)
			}
			return nil, ticketToOutput(ticket), nil
		},
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name: "search_tickets",
			Description: `Search and filter Freshdesk tickets. All fields are optional.
							Filters:
							- query: text matched against subject or type
							- status: 2=open, 3=pending, 4=resolved, 5=closed
							- priority: 1=low, 2=medium, 3=high, 4=urgent
							- type: "False Positive", "False Negative", "Service Request", "Incident"
							- overdue: true = tickets past due_by that are still open or pending
							- is_escalated: true = escalated tickets only
							- created_after / created_before: ISO8601 date e.g. "2026-06-01T00:00:00Z"
							- requester_id: from find_requester tool
							- company_id: from find_company tool
							Examples:
							Overdue tickets: {"overdue": true}
							Open false positives: {"type": "False Positive", "status": 2}
							High priority open: {"status": 2, "priority": 3}
							All tickets from Defence: first call find_company query="Defence", then use company_id`,
		},
		func(ctx context.Context, req *mcp.CallToolRequest, input SearchTicketsInput) (*mcp.CallToolResult, SearchTicketsOutput, error) {
			tickets, err := client.SearchTickets(ctx, freshdesk.TicketFilter{
				Query:         input.Query,
				Status:        input.Status,
				Priority:      input.Priority,
				Overdue:       input.Overdue != nil && *input.Overdue,
				Type:          input.Type,
				IsEscalated:   input.IsEscalated != nil && *input.IsEscalated,
				CreatedAfter:  input.CreatedAfter,
				CreatedBefore: input.CreatedBefore,
			UpdatedSince:  input.UpdatedSince,
				RequesterID:   input.RequesterID,
				CompanyID:     input.CompanyID,
				GroupID:       input.GroupID,
			})
			if err != nil {
				return nil, SearchTicketsOutput{}, fmt.Errorf("search_tickets: %w", err)
			}
			results := make([]GetTicketOutput, len(tickets))
			for i, t := range tickets {
				results[i] = ticketToOutput(&t)
			}
			return nil, SearchTicketsOutput{Total: len(results), Results: results}, nil
		},
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "get_conversations",
			Description: "Get all replies, notes and email threads for a Freshdesk ticket. Returns body_text (plain text, HTML stripped), incoming=true means customer sent it, incoming=false means agent sent it. Use this to understand the full investigation history, analyst notes, and customer communications.",
		},
		func(ctx context.Context, req *mcp.CallToolRequest, input GetConversationsInput) (*mcp.CallToolResult, GetConversationsOutput, error) {
			convs, err := client.GetConversations(ctx, input.TicketID)
			if err != nil {
				return nil, GetConversationsOutput{}, fmt.Errorf("get_conversations: %w", err)
			}
			results := make([]ConversationOutput, len(convs))
			for i, c := range convs {
				results[i] = ConversationOutput{
					ID:        c.ID,
					BodyText:  c.BodyText,
					Incoming:  c.Incoming,
					Private:   c.Private,
					CreatedAt: c.CreatedAt,
				}
			}
			return nil, GetConversationsOutput{
				TicketID:      input.TicketID,
				Conversations: results,
			}, nil
		},
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name: "get_attachment_text",
			Description: `Extract text from a Freshdesk ticket attachment. Supported formats:
							- .docx: Word documents → plain text
							- .xlsx: Excel spreadsheets → tab-separated rows per sheet
							- .json: JSON files → pretty printed
							- .txt: plain text
							- .csv: CSV files → plain text
							- .png .jpg .jpeg: screenshots and images → OCR via Google Vision API
							Use list_attachments first to get the attachment_id. Ideal for reading investigation reports, domain lists, DNS screenshots and phishing page captures.`,
		},
		func(ctx context.Context, req *mcp.CallToolRequest, input GetAttachmentTextInput) (*mcp.CallToolResult, GetAttachmentTextOutput, error) {
			attachments, err := client.GetAllAttachments(ctx, input.TicketID)
			if err != nil {
				return nil, GetAttachmentTextOutput{}, fmt.Errorf("get attachments: %w", err)
			}

			var att *freshdesk.Attachment
			for i := range attachments {
				if attachments[i].ID == input.AttachmentID {
					att = &attachments[i]
					break
				}
			}
			if att == nil {
				return nil, GetAttachmentTextOutput{}, fmt.Errorf("attachment %d not found on ticket %d", input.AttachmentID, input.TicketID)
			}

			data, err := client.DownloadAttachment(ctx, att.URL)
			if err != nil {
				return nil, GetAttachmentTextOutput{}, fmt.Errorf("download: %w", err)
			}

			text, err := extract.FromAttachment(ctx, att.Name, att.ContentType, data, gcpVisionProject)
			if err != nil {
				return nil, GetAttachmentTextOutput{}, fmt.Errorf("extract: %w", err)
			}

			return nil, GetAttachmentTextOutput{
				AttachmentID: att.ID,
				Name:         att.Name,
				Text:         text,
			}, nil
		},
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "list_attachments",
			Description: "List all attachments on a Freshdesk ticket including files in conversation replies. Returns attachment id, name, content_type and size in bytes. Always call this before get_attachment_text, query_attachment or find_image_attachments to discover attachment IDs.",
		},
		func(ctx context.Context, req *mcp.CallToolRequest, input ListAttachmentsInput) (*mcp.CallToolResult, ListAttachmentsOutput, error) {
			attachments, err := client.GetAllAttachments(ctx, input.TicketID)
			if err != nil {
				return nil, ListAttachmentsOutput{}, fmt.Errorf("list_attachments: %w", err)
			}

			results := make([]AttachmentInfo, len(attachments))
			for i, a := range attachments {
				results[i] = AttachmentInfo{
					ID:          a.ID,
					Name:        a.Name,
					ContentType: a.ContentType,
					Size:        a.Size,
				}
			}

			return nil, ListAttachmentsOutput{
				TicketID:    input.TicketID,
				Attachments: results,
			}, nil
		},
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "query_attachment",
			Description: "Search within a JSON attachment on a Freshdesk ticket. Useful for finding specific domains or threat feeds in large JSON files.",
		},
		func(ctx context.Context, req *mcp.CallToolRequest, input QueryAttachmentInput) (*mcp.CallToolResult, QueryAttachmentOutput, error) {
			attachments, err := client.GetAllAttachments(ctx, input.TicketID)
			if err != nil {
				return nil, QueryAttachmentOutput{}, fmt.Errorf("get attachments: %w", err)
			}

			var att *freshdesk.Attachment
			for i := range attachments {
				if attachments[i].ID == input.AttachmentID {
					att = &attachments[i]
					break
				}
			}
			if att == nil {
				return nil, QueryAttachmentOutput{}, fmt.Errorf("attachment %d not found", input.AttachmentID)
			}

			data, err := client.DownloadAttachment(ctx, att.URL)
			if err != nil {
				return nil, QueryAttachmentOutput{}, fmt.Errorf("download: %w", err)
			}

			result, err := extract.QueryJSON(data, input.Query)
			if err != nil {
				return nil, QueryAttachmentOutput{}, fmt.Errorf("query: %w", err)
			}

			return nil, QueryAttachmentOutput{
				AttachmentID: att.ID,
				Name:         att.Name,
				Query:        input.Query,
				Result:       result,
			}, nil
		},
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name: "find_image_attachments",
			Description: `Scan multiple tickets at once and return all image attachments (png, jpg, jpeg).
							Typical workflow:
							1. search_tickets → get list of ticket IDs
							2. find_image_attachments with those IDs → find screenshots
							3. get_attachment_text on attachment_id → OCR the image`,
		},
		func(ctx context.Context, req *mcp.CallToolRequest, input FindImageAttachmentsInput) (*mcp.CallToolResult, FindImageAttachmentsOutput, error) {
			var results []ImageAttachmentInfo

			for _, ticketID := range input.TicketIDs {
				attachments, err := client.GetAllAttachments(ctx, ticketID)
				if err != nil {
					continue
				}

				for _, a := range attachments {
					if isImage(a.Name, a.ContentType) {
						results = append(results, ImageAttachmentInfo{
							TicketID:     ticketID,
							AttachmentID: a.ID,
							Name:         a.Name,
							Size:         a.Size,
						})
					}
				}
			}

			return nil, FindImageAttachmentsOutput{
				Total:   len(results),
				Results: results,
			}, nil
		},
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "find_requester",
			Description: "Search Freshdesk contacts by name or email. Returns requester_id which can then be passed to search_tickets to find all tickets submitted by that person.",
		},
		func(ctx context.Context, req *mcp.CallToolRequest, input FindRequesterInput) (*mcp.CallToolResult, FindRequesterOutput, error) {
			contacts, err := client.SearchContacts(ctx, input.Query)
			if err != nil {
				return nil, FindRequesterOutput{}, fmt.Errorf("find_requester: %w", err)
			}
			results := make([]RequesterOutput, len(contacts))
			for i, c := range contacts {
				results[i] = RequesterOutput{
					ID:    c.ID,
					Name:  c.Name,
					Email: c.Email,
					Phone: c.Phone,
				}
			}
			return nil, FindRequesterOutput{Total: len(results), Results: results}, nil
		},
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "find_company",
			Description: "Search Freshdesk companies by name. Returns company_id which can be used in search_tickets to find all tickets from a specific organisation. Example: find_company query='Defence' then search_tickets company_id=<id>.",
		},
		func(ctx context.Context, req *mcp.CallToolRequest, input FindCompanyInput) (*mcp.CallToolResult, FindCompanyOutput, error) {
			companies, err := client.SearchCompanies(ctx, input.Query)
			if err != nil {
				return nil, FindCompanyOutput{}, fmt.Errorf("find_company: %w", err)
			}
			results := make([]CompanyOutput, len(companies))
			for i, c := range companies {
				results[i] = CompanyOutput{
					ID:      c.ID,
					Name:    c.Name,
					Domains: c.Domains,
				}
			}
			return nil, FindCompanyOutput{Total: len(results), Results: results}, nil
		},
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name: "get_ticket_summary",
			Description: `Retrieve a complete summary of a Freshdesk ticket in one call: ticket details, all conversation replies and notes, and list of attachments.
	Use this as the first tool when investigating a specific ticket — it gives everything needed to understand the full context without multiple round trips.
	Returns:
	- ticket: id, subject, status, type, priority, due_by, is_escalated, custom_fields
	- conversations: all replies and notes with body_text and direction (incoming=customer, outgoing=agent)
	- attachments: list of files with id, name, content_type, size (use get_attachment_text to read them)`,
		},
		func(ctx context.Context, req *mcp.CallToolRequest, input GetTicketSummaryInput) (*mcp.CallToolResult, GetTicketSummaryOutput, error) {
			// fetch all three in parallel
			var (
				ticket      *freshdesk.Ticket
				convs       []freshdesk.Conversation
				attachments []freshdesk.Attachment
			)

			g, gctx := errgroup.WithContext(ctx)

			g.Go(func() error {
				t, err := client.GetTicket(gctx, input.TicketID)
				if err != nil {
					return fmt.Errorf("get ticket: %w", err)
				}
				ticket = t
				return nil
			})

			g.Go(func() error {
				c, err := client.GetConversations(gctx, input.TicketID)
				if err != nil {
					return fmt.Errorf("get conversations: %w", err)
				}
				convs = c
				return nil
			})

			g.Go(func() error {
				a, err := client.GetAllAttachments(gctx, input.TicketID)
				if err != nil {
					return fmt.Errorf("get attachments: %w", err)
				}
				attachments = a
				return nil
			})

			if err := g.Wait(); err != nil {
				return nil, GetTicketSummaryOutput{}, fmt.Errorf("get_ticket_summary: %w", err)
			}

			convResults := make([]ConversationOutput, len(convs))
			for i, c := range convs {
				convResults[i] = ConversationOutput{
					ID:        c.ID,
					BodyText:  c.BodyText,
					Incoming:  c.Incoming,
					Private:   c.Private,
					CreatedAt: c.CreatedAt,
				}
			}

			attResults := make([]AttachmentInfo, len(attachments))
			for i, a := range attachments {
				attResults[i] = AttachmentInfo{
					ID:          a.ID,
					Name:        a.Name,
					ContentType: a.ContentType,
					Size:        a.Size,
				}
			}

			return nil, GetTicketSummaryOutput{
				Ticket:        ticketToOutput(ticket),
				Conversations: convResults,
				Attachments:   attResults,
			}, nil
		},
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name: "list_tickets",
			Description: `Return all Freshdesk tickets with full fields for reporting and analysis. All filters are optional.
	Filters:
	- status: 2=open, 3=pending, 4=resolved, 5=closed
	- priority: 1=low, 2=medium, 3=high, 4=urgent
	- type: "False Positive", "False Negative", "Service Request", "Incident"
	- created_after / created_before: ISO8601 e.g. "2026-06-01T00:00:00Z"
	Use this for bulk analysis - e.g. all false positives this month, all high priority open tickets.
	For company or requester filtering use search_tickets instead.`,
		},
		func(ctx context.Context, req *mcp.CallToolRequest, input ListTicketsInput) (*mcp.CallToolResult, ListTicketsOutput, error) {
			tickets, err := client.SearchTickets(ctx, freshdesk.TicketFilter{
				Status:        input.Status,
				Priority:      input.Priority,
				Type:          input.Type,
				CreatedAfter:  input.CreatedAfter,
				CreatedBefore: input.CreatedBefore,
			UpdatedSince:  input.UpdatedSince,
			})
			if err != nil {
				return nil, ListTicketsOutput{}, fmt.Errorf("list_tickets: %w", err)
			}

			results := make([]GetTicketOutput, len(tickets))
			for i, t := range tickets {
				results[i] = ticketToOutput(&t)
			}

			return nil, ListTicketsOutput{Total: len(results), Results: results}, nil
		},
	)
	mcp.AddTool(server,
		&mcp.Tool{
			Name: "get_description_images",
			Description: `Extract and OCR inline images embedded in a ticket's description HTML body. 
	Some tickets contain screenshots pasted directly into the description rather than uploaded as file attachments — these are not visible via list_attachments.
	Use this tool when get_ticket_summary shows a non-empty description but list_attachments finds no images, or when the description mentions a screenshot/table/log.
	Returns OCR text from each inline image found.`,
		},
		func(ctx context.Context, req *mcp.CallToolRequest, input GetDescriptionImagesInput) (*mcp.CallToolResult, GetDescriptionImagesOutput, error) {
			ticket, err := client.GetTicket(ctx, input.TicketID)
			if err != nil {
				return nil, GetDescriptionImagesOutput{}, fmt.Errorf("get ticket: %w", err)
			}

			urls := freshdesk.ExtractInlineImageURLs(ticket.Description)
			if len(urls) == 0 {
				return nil, GetDescriptionImagesOutput{TicketID: input.TicketID, Total: 0}, nil
			}

			var results []InlineImageResult
			for _, imgURL := range urls {
				data, err := client.DownloadInlineAttachment(ctx, imgURL)
				if err != nil {
					results = append(results, InlineImageResult{URL: imgURL, Text: fmt.Sprintf("error: %v", err)})
					continue
				}

				text, err := extract.ImageOCR(ctx, data, gcpVisionProject)
				if err != nil {
					results = append(results, InlineImageResult{URL: imgURL, Text: fmt.Sprintf("ocr error: %v", err)})
					continue
				}

				results = append(results, InlineImageResult{URL: imgURL, Text: text})
			}

			return nil, GetDescriptionImagesOutput{
				TicketID: input.TicketID,
				Total:    len(results),
				Images:   results,
			}, nil
		},
	)
	mcp.AddTool(server,
		&mcp.Tool{
			Name: "list_groups",
			Description: `List all Freshdesk agent groups. Returns group id and name.
							Use this to find the group_id for filtering tickets by team.
							Example workflow: list_groups → find "Threat Intelligence" id → search_tickets group_id=<id>
							Known groups: "AU staff" (general), "Threat Intelligence" (phishing/false negatives), "EU Staff"`,
		},
		func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, ListGroupsOutput, error) {
			groups, err := client.ListGroups(ctx)
			if err != nil {
				return nil, ListGroupsOutput{}, fmt.Errorf("list_groups: %w", err)
			}
			results := make([]freshdesk.Group, len(groups))
			for i, g := range groups {
				results[i] = freshdesk.Group{ID: g.ID, Name: g.Name}
			}
			return nil, ListGroupsOutput{Total: len(results), Results: results}, nil
		},
	)
	return server
}

func main() {
	// GOMAXPROCS — Cloud Run allocates fractional CPUs, default GOMAXPROCS
	// may be set to host CPU count. Cap it to what's actually available.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// structured JSON logging
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	client := freshdesk.NewClient(
		"https://"+os.Getenv("FRESHDESK_DOMAIN")+".freshdesk.com",
		os.Getenv("FRESHDESK_API_KEY"),
	)

	gcpVisionProject := os.Getenv("GCP_VISION_PROJECT")
	if gcpVisionProject == "" {
		slog.Error("GCP_VISION_PROJECT environment variable is required")
		os.Exit(1)
	}

	server := buildServer(client, gcpVisionProject)

	switch os.Getenv("MCP_TRANSPORT") {
	case "http":
		addr := ":" + port()
		token := os.Getenv("MCP_TOKEN")
		if token == "" {
			slog.Error("MCP_TOKEN environment variable is required in HTTP mode")
			os.Exit(1)
		}

		slog.Info("freshdesk-mcp HTTP server starting", "addr", addr)

		handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return server
		}, nil)

		mux := http.NewServeMux()
		mux.Handle("/mcp", authMiddleware(token, handler))
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		})

		httpServer := &http.Server{
			Addr:    addr,
			Handler: mux,
		}

		// graceful shutdown on SIGTERM/SIGINT
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
		defer stop()

		go func() {
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("http server error", "err", err)
				os.Exit(1)
			}
		}()

		slog.Info("freshdesk-mcp ready")
		<-ctx.Done()

		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown error", "err", err)
		}
		slog.Info("shutdown complete")

	default:
		slog.Info("freshdesk-mcp server starting on stdio")
		if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}
}

func port() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "8080"
}

func authMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isImage(name, contentType string) bool {
	imageTypes := []string{"image/png", "image/jpeg", "image/jpg", "image/gif", "image/webp"}
	for _, t := range imageTypes {
		if contentType == t {
			return true
		}
	}
	imageSuffixes := []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}
	for _, s := range imageSuffixes {
		if strings.HasSuffix(strings.ToLower(name), s) {
			return true
		}
	}
	return false
}

func ticketToOutput(t *freshdesk.Ticket) GetTicketOutput {
	return GetTicketOutput{
		ID:              t.ID,
		Subject:         t.Subject,
		Status:          t.Status,
		Type:            t.Type,
		Priority:        t.Priority,
		DueBy:           t.DueBy,
		IsEscalated:     t.IsEscalated,
		CreatedAt:       t.CreatedAt,
		Tags:            t.Tags,
		CustomFields:    t.CustomFields,
		DescriptionText: t.DescriptionText,
	}
}
