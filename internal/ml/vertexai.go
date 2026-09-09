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
	ErrVertexAIError = errors.New("vertex ai error")
	ErrVertexAIEmpty = errors.New("empty response from vertex ai")
)

// VertexAI is an ML provider backed by Google Cloud Vertex AI Gemini API.
type VertexAI struct {
	project      string
	location     string
	model        string
	systemPrompt string
	client       *http.Client
}

// NewVertexAI creates a new VertexAI provider.
// project: GCP project ID
// location: GCP region e.g. "us-central1"
// model: Gemini model e.g. "gemini-1.5-flash"
func NewVertexAI(project, location, model, systemPrompt string) *VertexAI {
	if location == "" {
		location = "us-central1"
	}
	if model == "" {
		model = "gemini-1.5-flash"
	}
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}
	return &VertexAI{
		project:      project,
		location:     location,
		model:        model,
		systemPrompt: systemPrompt,
		client:       &http.Client{},
	}
}

type vertexRequest struct {
	Contents []vertexContent `json:"contents"`
}

type vertexContent struct {
	Role  string       `json:"role"`
	Parts []vertexPart `json:"parts"`
}

type vertexPart struct {
	Text string `json:"text"`
}

type vertexResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (v *VertexAI) endpoint() string {
	return fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		v.location, v.project, v.location, v.model,
	)
}

func (v *VertexAI) generate(ctx context.Context, prompt string) (string, error) {
	reqBody, err := json.Marshal(vertexRequest{
		Contents: []vertexContent{
			{Role: "user", Parts: []vertexPart{{Text: prompt}}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint(), bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Use Application Default Credentials via metadata server on GCP
	// or GOOGLE_APPLICATION_CREDENTIALS locally
	token, err := getGCPToken(ctx)
	if err != nil {
		return "", fmt.Errorf("get GCP token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := v.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call vertex ai: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w %d: %s", ErrVertexAIError, resp.StatusCode, string(body))
	}

	var vertexResp vertexResponse
	if err := json.Unmarshal(body, &vertexResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(vertexResp.Candidates) == 0 || len(vertexResp.Candidates[0].Content.Parts) == 0 {
		return "", ErrVertexAIEmpty
	}

	return vertexResp.Candidates[0].Content.Parts[0].Text, nil
}

// Classify implements Provider using Vertex AI Gemini.
func (v *VertexAI) Classify(ctx context.Context, input ClassifyInput) (*Classification, error) {
	prompt := fmt.Sprintf(`%s
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
		v.systemPrompt,
		strings.Join(input.ValidTypes, ", "),
		input.Subject,
		input.Description,
	)

	text, err := v.generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("classify: %w", err)
	}

	var result Classification
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("decode classification: %w", err)
	}

	return &result, nil
}

// SuggestResolution implements Provider using Vertex AI Gemini.
func (v *VertexAI) SuggestResolution(ctx context.Context, input ResolutionInput) (*Resolution, error) {
	conversations := strings.Join(input.Conversations, "\n---\n")

	prompt := fmt.Sprintf(`%s
Suggest a formal resolution for the following support ticket.

Ticket type: %s
Subject: %s
Description: %s
Conversation history:
%s

Respond with JSON only, no other text:
{
  "suggested_action": "<concise action to resolve this ticket>",
  "draft_reply": "<formal reply to the customer>",
  "next_steps": ["<step 1>", "<step 2>"],
  "confidence": <0.0 to 1.0>
}`,
		v.systemPrompt,
		input.TicketType,
		input.Subject,
		input.DescriptionText,
		conversations,
	)

	text, err := v.generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("suggest resolution: %w", err)
	}

	var result Resolution
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("decode resolution: %w", err)
	}

	return &result, nil
}
