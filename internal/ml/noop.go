package ml

import "context"

// Noop is an ML provider that always returns empty results.
// Use when ML assistance is disabled or no provider is configured.
type Noop struct{}

// Classify implements Provider. Returns empty classification with no error.
func (Noop) Classify(_ context.Context, _ ClassifyInput) (*Classification, error) {
	return &Classification{}, nil
}

// SuggestResolution implements Provider. Returns empty resolution with no error.
func (Noop) SuggestResolution(_ context.Context, _ ResolutionInput) (*Resolution, error) {
	return &Resolution{}, nil
}
