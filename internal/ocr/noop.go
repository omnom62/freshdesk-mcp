package ocr

import "context"

// Noop is an OCR provider that always returns an empty string.
// Use when OCR is disabled or no provider is configured.
type Noop struct{}

// ExtractText implements Provider. Returns empty string with no error.
func (Noop) ExtractText(_ context.Context, _ []byte) (string, error) {
	return "", nil
}
