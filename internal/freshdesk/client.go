// internal/freshdesk/client.go
package freshdesk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/whalebone/freshdesk-mcp/internal/cache"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

type Client struct {
	BaseURL     string
	APIKey      string
	HTTPClient  *http.Client
	tickets     *cache.Cache[string, []Ticket]
	attachments *cache.Cache[string, []byte]
	limiter     *rate.Limiter
}

type Ticket struct {
	ID              int64          `json:"id"`
	Subject         string         `json:"subject"`
	Status          int            `json:"status"`
	Type            string         `json:"type"`
	Priority        int            `json:"priority"`
	RequesterID     int64          `json:"requester_id"`
	CompanyID       int64          `json:"company_id"`
	DueBy           string         `json:"due_by"`
	FrDueBy         string         `json:"fr_due_by"`
	IsEscalated     bool           `json:"is_escalated"`
	FrEscalated     bool           `json:"fr_escalated"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
	Tags            []string       `json:"tags"`
	CustomFields    map[string]any `json:"custom_fields"`
	Attachments     []Attachment   `json:"attachments"`
	DescriptionText string         `json:"description_text"`
}

type TicketFilter struct {
	Query         string
	Status        int
	Priority      int
	Overdue       bool
	Type          string
	IsEscalated   bool
	CreatedAfter  string
	CreatedBefore string
	RequesterID   int64
	CompanyID     int64
}

type Conversation struct {
	ID          int64        `json:"id"`
	BodyText    string       `json:"body_text"`
	Incoming    bool         `json:"incoming"`
	Private     bool         `json:"private"`
	CreatedAt   string       `json:"created_at"`
	Attachments []Attachment `json:"attachments"`
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:     baseURL,
		APIKey:      apiKey,
		HTTPClient:  &http.Client{},
		tickets:     cache.New[string, []Ticket](5 * time.Minute),
		attachments: cache.New[string, []byte](10 * time.Minute),
		limiter:     rate.NewLimiter(rate.Every(time.Minute/40), 1),
	}
}

type Attachment struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	URL         string `json:"attachment_url"`
}

type Contact struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type Company struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
}

func (c *Client) doRequest(ctx context.Context, method, path string) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}
	rawURL := c.BaseURL + path

	const maxRetries = 3
	var lastErr error

	for attempt := range maxRetries {
		req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}

		req.SetBasicAuth(c.APIKey, "X")
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("do request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			wait := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			lastErr = fmt.Errorf("HTTP 429: rate limited")
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
				continue
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		}

		return body, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (c *Client) GetTicket(ctx context.Context, id int64) (*Ticket, error) {
	body, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v2/tickets/%d", id))
	if err != nil {
		return nil, fmt.Errorf("GetTicket(%d): %w", id, err)
	}

	var ticket Ticket
	if err := json.Unmarshal(body, &ticket); err != nil {
		return nil, fmt.Errorf("decode ticket: %w\nraw: %s", err, string(body))
	}

	return &ticket, nil
}

func (c *Client) ListTickets(ctx context.Context) ([]Ticket, error) {
	const cacheKey = "all"

	if cached, ok := c.tickets.Get(cacheKey); ok {
		return cached, nil
	}

	var all []Ticket
	page := 1

	for {
		body, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v2/tickets?per_page=100&page=%d", page))
		if err != nil {
			return nil, fmt.Errorf("ListTickets page %d: %w", page, err)
		}

		var tickets []Ticket
		if err := json.Unmarshal(body, &tickets); err != nil {
			return nil, fmt.Errorf("decode tickets page %d: %w\nraw: %s", page, err, string(body))
		}

		all = append(all, tickets...)

		if len(tickets) < 100 {
			break
		}

		page++
	}
	c.tickets.Set(cacheKey, all)
	return all, nil
}

func (c *Client) SearchTickets(ctx context.Context, filter TicketFilter) ([]Ticket, error) {
	tickets, err := c.ListTickets(ctx)
	if err != nil {
		return nil, fmt.Errorf("SearchTickets: %w", err)
	}

	now := time.Now().UTC()
	var matches []Ticket

	for _, t := range tickets {
		if filter.Query != "" {
			q := strings.ToLower(filter.Query)
			if !strings.Contains(strings.ToLower(t.Subject), q) &&
				!strings.Contains(strings.ToLower(t.Type), q) {
				continue
			}
		}
		if filter.Status != 0 && t.Status != filter.Status {
			continue
		}
		if filter.Priority != 0 && t.Priority != filter.Priority {
			continue
		}
		if filter.Type != "" && !strings.EqualFold(t.Type, filter.Type) {
			continue
		}
		if filter.IsEscalated && !t.IsEscalated {
			continue
		}
		if filter.RequesterID != 0 && t.RequesterID != filter.RequesterID {
			continue
		}
		if filter.CompanyID != 0 && t.CompanyID != filter.CompanyID {
			continue
		}
		if filter.CreatedAfter != "" {
			after, err := time.Parse(time.RFC3339, filter.CreatedAfter)
			if err == nil {
				created, err := time.Parse(time.RFC3339, t.CreatedAt)
				if err == nil && created.Before(after) {
					continue
				}
			}
		}
		if filter.CreatedBefore != "" {
			before, err := time.Parse(time.RFC3339, filter.CreatedBefore)
			if err == nil {
				created, err := time.Parse(time.RFC3339, t.CreatedAt)
				if err == nil && created.After(before) {
					continue
				}
			}
		}
		if filter.Overdue {
			if t.DueBy == "" || t.Status >= 4 {
				continue
			}
			dueBy, err := time.Parse(time.RFC3339, t.DueBy)
			if err != nil || !now.After(dueBy) {
				continue
			}
		}

		matches = append(matches, t)
	}

	return matches, nil
}

func (c *Client) GetConversations(ctx context.Context, ticketID int64) ([]Conversation, error) {
	body, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v2/tickets/%d/conversations", ticketID))
	if err != nil {
		return nil, fmt.Errorf("GetConversations(%d): %w", ticketID, err)
	}

	var convs []Conversation
	if err := json.Unmarshal(body, &convs); err != nil {
		return nil, fmt.Errorf("decode conversations: %w\nraw: %s", err, string(body))
	}

	return convs, nil
}

func (c *Client) DownloadAttachment(ctx context.Context, url string) ([]byte, error) {
	if cached, ok := c.attachments.Get(url); ok {
		return cached, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download attachment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	c.attachments.Set(url, data)
	return data, nil
}

func (c *Client) GetAllAttachments(ctx context.Context, ticketID int64) ([]Attachment, error) {
	var (
		ticketAtts []Attachment
		convAtts   []Attachment
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		t, err := c.GetTicket(gctx, ticketID)
		if err != nil {
			return fmt.Errorf("fetch ticket: %w", err)
		}
		ticketAtts = t.Attachments
		return nil
	})

	g.Go(func() error {
		conversations, err := c.GetConversations(gctx, ticketID)
		if err != nil {
			return fmt.Errorf("fetch conversations: %w", err)
		}
		for _, conv := range conversations {
			convAtts = append(convAtts, conv.Attachments...)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("GetAllAttachments(%d): %w", ticketID, err)
	}

	return append(ticketAtts, convAtts...), nil
}

func (c *Client) SearchCompanies(ctx context.Context, query string) ([]Company, error) {
	var all []Company
	page := 1

	for {
		body, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v2/companies?per_page=100&page=%d", page))
		if err != nil {
			return nil, fmt.Errorf("SearchCompanies page %d: %w", page, err)
		}

		var companies []Company
		if err := json.Unmarshal(body, &companies); err != nil {
			return nil, fmt.Errorf("decode companies: %w\nraw: %s", err, string(body))
		}

		all = append(all, companies...)

		if len(companies) < 100 {
			break
		}
		page++
	}

	// client-side filter by name
	q := strings.ToLower(query)
	var matches []Company
	for _, co := range all {
		if strings.Contains(strings.ToLower(co.Name), q) {
			matches = append(matches, co)
		}
	}

	return matches, nil
}

func (c *Client) SearchContacts(ctx context.Context, query string) ([]Contact, error) {
	var all []Contact
	page := 1

	for {
		body, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v2/contacts?per_page=100&page=%d", page))
		if err != nil {
			return nil, fmt.Errorf("SearchContacts page %d: %w", page, err)
		}

		var contacts []Contact
		if err := json.Unmarshal(body, &contacts); err != nil {
			return nil, fmt.Errorf("decode contacts: %w\nraw: %s", err, string(body))
		}

		all = append(all, contacts...)

		if len(contacts) < 100 {
			break
		}
		page++
	}

	// client-side filter by name or email
	q := strings.ToLower(query)
	var matches []Contact
	for _, co := range all {
		if strings.Contains(strings.ToLower(co.Name), q) ||
			strings.Contains(strings.ToLower(co.Email), q) {
			matches = append(matches, co)
		}
	}

	return matches, nil
}
