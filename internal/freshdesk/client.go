// internal/freshdesk/client.go
package freshdesk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/omnom62/freshdesk-mcp/internal/cache"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

var (
	ErrRateLimited = errors.New("HTTP 429: rate limited")
	ErrHTTPError   = errors.New("HTTP error")
)

const (
	httpStatusOKMin = 200
	httpStatusOKMax = 300
	pageSize        = 100
	statusResolved  = 4
	methodGET       = "GET"
)

type Client struct {
	BaseURL     string
	APIKey      string
	HTTPClient  *http.Client
	tickets     *cache.Cache[string, []Ticket]
	attachments *cache.Cache[string, []byte]
	limiter     *rate.Limiter
	statusMap   *cache.Cache[string, map[int]string]
}

type Ticket struct {
	ID              int64          `json:"id"`
	Subject         string         `json:"subject"`
	Status          int            `json:"status"`
	Type            string         `json:"type"`
	Priority        int            `json:"priority"`
	RequesterID     int64          `json:"requester_id"`
	CompanyID       int64          `json:"company_id"`
	GroupID         int64          `json:"group_id"`
	ResponderID     int64          `json:"responder_id"`
	DueBy           string         `json:"due_by"`
	FrDueBy         string         `json:"fr_due_by"`
	IsEscalated     bool           `json:"is_escalated"`
	FrEscalated     bool           `json:"fr_escalated"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
	Tags            []string       `json:"tags"`
	CustomFields    map[string]any `json:"custom_fields"`
	Attachments     []Attachment   `json:"attachments"`
	Description     string         `json:"description"`
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
	UpdatedSince  string
	GroupID       int64
	AgentID       int64
}

type Conversation struct {
	ID          int64        `json:"id"`
	Body        string       `json:"body"`
	BodyText    string       `json:"body_text"`
	Incoming    bool         `json:"incoming"`
	Private     bool         `json:"private"`
	CreatedAt   string       `json:"created_at"`
	Attachments []Attachment `json:"attachments"`
}

type Group struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (c *Client) ListGroups(ctx context.Context) ([]Group, error) {
	body, err := c.doRequest(ctx, "/api/v2/groups")
	if err != nil {
		return nil, fmt.Errorf("ListGroups: %w", err)
	}

	var groups []Group
	if err := json.Unmarshal(body, &groups); err != nil {
		return nil, fmt.Errorf("decode groups: %w", err)
	}

	return groups, nil
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:     baseURL,
		APIKey:      apiKey,
		HTTPClient:  &http.Client{},
		tickets:     cache.New[string, []Ticket](5 * time.Minute),
		attachments: cache.New[string, []byte](10 * time.Minute),
		limiter:     rate.NewLimiter(rate.Every(time.Minute/40), 1),
		statusMap:   cache.New[string, map[int]string](60 * time.Minute),
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

type Agent struct {
	ID      int64        `json:"id"`
	Contact AgentContact `json:"contact"`
}

type AgentContact struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

//nolint:cyclop
func (c *Client) doRequest(ctx context.Context, path string) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}
	rawURL := c.BaseURL + path

	const maxRetries = 3
	var lastErr error

	for attempt := range maxRetries {
		req, err := http.NewRequestWithContext(ctx, methodGET, rawURL, nil)
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
			lastErr = ErrRateLimited
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
				continue
			}
		}

		if resp.StatusCode < httpStatusOKMin || resp.StatusCode >= httpStatusOKMax {
			return nil, fmt.Errorf("%w: %d: %s", ErrHTTPError, resp.StatusCode, string(body))
		}

		return body, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (c *Client) GetTicket(ctx context.Context, id int64) (*Ticket, error) {
	body, err := c.doRequest(ctx, fmt.Sprintf("/api/v2/tickets/%d", id))
	if err != nil {
		return nil, fmt.Errorf("GetTicket(%d): %w", id, err)
	}

	var ticket Ticket
	if err := json.Unmarshal(body, &ticket); err != nil {
		return nil, fmt.Errorf("decode ticket: %w\nraw: %s", err, string(body))
	}

	return &ticket, nil
}

func (c *Client) ListTickets(ctx context.Context, updatedSince string) ([]Ticket, error) {
	const cacheKey = "all"

	if updatedSince == "" {
		if cached, ok := c.tickets.Get(cacheKey); ok {
			return cached, nil
		}
	}

	var all []Ticket
	page := 1

	for {
		path := fmt.Sprintf("/api/v2/tickets?per_page=100&page=%d", page)
		since := updatedSince
		if since == "" {
			since = "2000-01-01T00:00:00Z"
		}
		path += "&updated_since=" + url.QueryEscape(since)
		body, err := c.doRequest(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("ListTickets page %d: %w", page, err)
		}

		var tickets []Ticket
		if err := json.Unmarshal(body, &tickets); err != nil {
			return nil, fmt.Errorf("decode tickets page %d: %w\nraw: %s", page, err, string(body))
		}

		all = append(all, tickets...)

		if len(tickets) < pageSize {
			break
		}

		page++
	}
	if updatedSince == "" {
		c.tickets.Set(cacheKey, all)
	}
	return all, nil
}

func (c *Client) SearchTickets(ctx context.Context, filter *TicketFilter) ([]Ticket, error) {
	tickets, err := c.ListTickets(ctx, filter.UpdatedSince)
	if err != nil {
		return nil, fmt.Errorf("SearchTickets: %w", err)
	}

	matches := filterTickets(tickets, filter)

	return matches, nil
}

func (c *Client) GetConversations(ctx context.Context, ticketID int64) ([]Conversation, error) {
	var all []Conversation
	page := 1
	for {
		body, err := c.doRequest(
			ctx,
			fmt.Sprintf("/api/v2/tickets/%d/conversations?per_page=100&page=%d", ticketID, page),
		)
		if err != nil {
			return nil, fmt.Errorf("GetConversations(%d) page %d: %w", ticketID, page, err)
		}
		var convs []Conversation
		if err := json.Unmarshal(body, &convs); err != nil {
			return nil, fmt.Errorf("decode conversations page %d: %w\nraw: %s", page, err, string(body))
		}
		all = append(all, convs...)
		if len(convs) < pageSize {
			break
		}
		page++
	}
	return all, nil
}
func (c *Client) DownloadAttachment(ctx context.Context, url string) ([]byte, error) {
	if cached, ok := c.attachments.Get(url); ok {
		return cached, nil
	}

	req, err := http.NewRequestWithContext(ctx, methodGET, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download attachment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < httpStatusOKMin || resp.StatusCode >= httpStatusOKMax {
		return nil, fmt.Errorf("%w: %d", ErrHTTPError, resp.StatusCode)
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

	var result []Attachment
	for _, a := range append(ticketAtts, convAtts...) {
		if a.ID != 0 && a.URL != "" {
			result = append(result, a)
		}
	}
	return result, nil
}

func (c *Client) SearchCompanies(ctx context.Context, query string) ([]Company, error) {
	var all []Company
	page := 1

	for {
		body, err := c.doRequest(
			ctx,
			fmt.Sprintf("/api/v2/companies?per_page=100&page=%d", page),
		)
		if err != nil {
			return nil, fmt.Errorf("SearchCompanies page %d: %w", page, err)
		}

		var companies []Company
		if err := json.Unmarshal(body, &companies); err != nil {
			return nil, fmt.Errorf("decode companies: %w\nraw: %s", err, string(body))
		}

		all = append(all, companies...)

		if len(companies) < pageSize {
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
		body, err := c.doRequest(
			ctx,
			fmt.Sprintf("/api/v2/contacts?per_page=100&page=%d", page),
		)
		if err != nil {
			return nil, fmt.Errorf("SearchContacts page %d: %w", page, err)
		}

		var contacts []Contact
		if err := json.Unmarshal(body, &contacts); err != nil {
			return nil, fmt.Errorf("decode contacts: %w\nraw: %s", err, string(body))
		}

		all = append(all, contacts...)

		if len(contacts) < pageSize {
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

func (c *Client) SearchAgents(ctx context.Context, query string) ([]Agent, error) {
	var all []Agent
	page := 1

	for {
		body, err := c.doRequest(
			ctx,
			fmt.Sprintf("/api/v2/agents?per_page=100&page=%d", page),
		)
		if err != nil {
			return nil, fmt.Errorf("SearchAgents page %d: %w", page, err)
		}

		var agents []Agent
		if err := json.Unmarshal(body, &agents); err != nil {
			return nil, fmt.Errorf("decode agents: %w\nraw: %s", err, string(body))
		}

		all = append(all, agents...)

		if len(agents) < pageSize {
			break
		}
		page++
	}

	// client-side filter by name or email
	q := strings.ToLower(query)
	var matches []Agent
	for _, a := range all {
		if strings.Contains(strings.ToLower(a.Contact.Name), q) ||
			strings.Contains(strings.ToLower(a.Contact.Email), q) {
			matches = append(matches, a)
		}
	}

	return matches, nil
}

// ExtractInlineImageURLs parses HTML and returns all inline attachment URLs.
func ExtractInlineImageURLs(html string) []string {
	var urls []string
	// Match <img src="..."> tags
	re := regexp.MustCompile(`<img[^>]+src="([^"]+)"`)
	matches := re.FindAllStringSubmatch(html, -1)
	for _, m := range matches {
		if len(m) > 1 {
			urls = append(urls, m[1])
		}
	}
	return urls
}

// DownloadInlineAttachment downloads an inline Freshdesk attachment URL
// using API key auth and following redirects.
func (c *Client) DownloadInlineAttachment(ctx context.Context, url string) ([]byte, error) {
	if cached, ok := c.attachments.Get(url); ok {
		return cached, nil
	}

	req, err := http.NewRequestWithContext(ctx, methodGET, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(c.APIKey, "X")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download inline attachment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < httpStatusOKMin || resp.StatusCode >= httpStatusOKMax {
		return nil, fmt.Errorf("%w: %d", ErrHTTPError, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	c.attachments.Set(url, data)
	return data, nil
}

// GetStatusMap returns a map of status ID to status name fetched from Freshdesk ticket fields.
// Result is cached for 60 minutes.
func (c *Client) GetStatusMap(ctx context.Context) (map[int]string, error) {
	const cacheKey = "status"
	if cached, ok := c.statusMap.Get(cacheKey); ok {
		return cached, nil
	}

	body, err := c.doRequest(ctx, "/api/v2/ticket_fields")
	if err != nil {
		return nil, fmt.Errorf("GetStatusMap: %w", err)
	}

	var fields []struct {
		Name    string          `json:"name"`
		Choices json.RawMessage `json:"choices"`
	}
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, fmt.Errorf("decode ticket fields: %w\nraw: %s", err, string(body))
	}

	statusMap := make(map[int]string)
	for _, f := range fields {
		if f.Name != "status" {
			continue
		}
		var choices map[string][]string
		if err := json.Unmarshal(f.Choices, &choices); err != nil {
			continue
		}
		for k, v := range choices {
			var id int
			if _, err := fmt.Sscanf(k, "%d", &id); err != nil {
				continue
			}
			if len(v) > 0 {
				statusMap[id] = v[0]
			}
		}
	}

	c.statusMap.Set(cacheKey, statusMap)
	return statusMap, nil
}
