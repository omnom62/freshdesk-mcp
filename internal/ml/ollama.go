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

var ErrOllamaError = errors.New("ollama error")

// OllamaProvider classifies tickets using a local Ollama instance.
type OllamaProvider struct {
	baseURL      string
	model        string
	client       *http.Client
	systemPrompt string
}

// NewOllamaProvider creates a new OllamaProvider.
// baseURL defaults to http://localhost:11434 if empty.
// model is the Ollama model name e.g. "llama3", "mistral", "qwen2".
func NewOllamaProvider(baseURL, model, systemPrompt string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}
	return &OllamaProvider{
		baseURL:      baseURL,
		model:        model,
		client:       &http.Client{},
		systemPrompt: systemPrompt,
	}
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	Format string `json:"format"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}

// Classify implements Provider using a local Ollama model.
func (o *OllamaProvider) Classify(ctx context.Context, input ClassifyInput) (*Classification, error) {
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
		o.systemPrompt,
		strings.Join(input.ValidTypes, ", "),
		input.Subject,
		input.Description,
	)

	reqBody, err := json.Marshal(ollamaRequest{
		Model:  o.model,
		Prompt: prompt,
		Stream: false,
		Format: "json",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.baseURL+"/api/generate", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call ollama: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w %d: %s", ErrOllamaError, resp.StatusCode, string(body))
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var result Classification
	if err := json.Unmarshal([]byte(ollamaResp.Response), &result); err != nil {
		return nil, fmt.Errorf("decode classification: %w", err)
	}

	return &result, nil
}

// SuggestResolution implements Provider using a local Ollama model.
func (o *OllamaProvider) SuggestResolution(ctx context.Context, input ResolutionInput) (*Resolution, error) {
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
		o.systemPrompt,
		input.TicketType,
		input.Subject,
		input.DescriptionText,
		conversations,
	)

	reqBody, err := json.Marshal(ollamaRequest{
		Model:  o.model,
		Prompt: prompt,
		Stream: false,
		Format: "json",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.baseURL+"/api/generate", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call ollama: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w %d: %s", ErrOllamaError, resp.StatusCode, string(body))
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var result Resolution
	if err := json.Unmarshal([]byte(ollamaResp.Response), &result); err != nil {
		return nil, fmt.Errorf("decode resolution: %w", err)
	}

	return &result, nil
}
