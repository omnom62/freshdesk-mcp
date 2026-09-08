package ml

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var (
	ErrClaudeAPIError = errors.New("claude api error")
	ErrClaudeEmpty    = errors.New("empty response from claude")
)

// ClaudeProvider classifies tickets using the Anthropic Claude API.
type ClaudeProvider struct {
	apiKey       string
	client       *http.Client
	systemPrompt string
}

// NewClaudeProvider creates a new ClaudeProvider with the given API key.
func NewClaudeProvider(apiKey, systemPrompt string) *ClaudeProvider {
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}
	return &ClaudeProvider{
		apiKey:       apiKey,
		client:       &http.Client{},
		systemPrompt: systemPrompt,
	}
}

type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []claudeMessage `json:"messages"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

// Classify implements Provider using Claude to classify ticket type.
func (c *ClaudeProvider) Classify(ctx context.Context, input ClassifyInput) (*Classification, error) {
	prompt := fmt.Sprintf(
		`%s
Classify the following ticket into one of these types: %s

Ticket subject: %s
Ticket description: %s

Respond with JSON only, no other text:
{
  "type": "<one of the valid types>",
  "confidence": <0.0 to 1.0>,
  "reasoning": "<one sentence explaining why>",
  "tags": ["<keyword1>", "<keyword2>"]
}`,
		c.systemPrompt,
		strings.Join(input.ValidTypes, ", "),
		input.Subject,
		input.Description,
	)

	reqBody, err := json.Marshal(claudeRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 256,
		Messages: []claudeMessage{
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call claude api: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w %d: %s", ErrClaudeAPIError, resp.StatusCode, string(body))
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(claudeResp.Content) == 0 {
		return nil, ErrClaudeEmpty
	}

	var result Classification
	if err := json.Unmarshal([]byte(claudeResp.Content[0].Text), &result); err != nil {
		return nil, fmt.Errorf("decode classification: %w", err)
	}

	return &result, nil
}

// SuggestResolution implements Provider using Claude to suggest ticket resolution.
func (c *ClaudeProvider) SuggestResolution(ctx context.Context, input ResolutionInput) (*Resolution, error) {
	conversations := strings.Join(input.Conversations, "\n---\n")

	prompt := fmt.Sprintf(
		`%s
Suggest a formal resolution for the following support ticket for the following support ticket.

Ticket type: %s
Subject: %s
Description: %s
Conversation history:
%s

Respond with JSON only, no other text:
{
  "suggested_action": "<concise action to resolve this ticket>",
  "draft_reply": "<formal reply to the customer, professional government tone>",
  "next_steps": ["<step 1>", "<step 2>"],
  "confidence": <0.0 to 1.0>
}`,
		c.systemPrompt,
		input.TicketType,
		input.Subject,
		input.DescriptionText,
		conversations,
	)

	reqBody, err := json.Marshal(claudeRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 1024,
		Messages: []claudeMessage{
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call claude api: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w %d: %s", ErrClaudeAPIError, resp.StatusCode, string(body))
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(claudeResp.Content) == 0 {
		return nil, ErrClaudeEmpty
	}

	var result Resolution
	if err := json.Unmarshal([]byte(claudeResp.Content[0].Text), &result); err != nil {
		return nil, fmt.Errorf("decode resolution: %w", err)
	}

	return &result, nil
}
