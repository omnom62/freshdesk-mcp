// Package ml defines the MLProvider interface for AI-powered ticket assistance.
package ml

import "context"

// ClassifyInput holds the ticket data and valid classification values.
type ClassifyInput struct {
	Subject     string   `json:"subject"`
	Description string   `json:"description"`
	ValidTypes  []string `json:"valid_types"`
}

// Classification is the result of ticket type classification.
type Classification struct {
	Type       string   `json:"type"`
	Confidence float64  `json:"confidence"`
	Reasoning  string   `json:"reasoning"`
	Tags       []string `json:"tags,omitempty"`
}

// ResolutionInput holds the full ticket context for resolution suggestion.
type ResolutionInput struct {
	Subject         string   `json:"subject"`
	DescriptionText string   `json:"description_text"`
	Conversations   []string `json:"conversations"`
	TicketType      string   `json:"ticket_type"`
}

// Resolution is the suggested resolution for a ticket.
type Resolution struct {
	SuggestedAction string   `json:"suggested_action"`
	DraftReply      string   `json:"draft_reply"`
	NextSteps       []string `json:"next_steps"`
	Confidence      float64  `json:"confidence"`
}

// DefaultSystemPrompt is used when no custom prompt is configured.
const DefaultSystemPrompt = "You are an expert support engineer. Classify tickets and suggest resolutions based on ticket content. Always respond with valid JSON only."

// Provider classifies and suggests resolutions for Freshdesk tickets.
type Provider interface {
	Classify(ctx context.Context, input ClassifyInput) (*Classification, error)
	SuggestResolution(ctx context.Context, input ResolutionInput) (*Resolution, error)
}
