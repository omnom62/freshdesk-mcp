// Package ocr defines the OCRProvider interface for extracting text from images.
package ocr

import "context"

// Provider extracts text from image bytes.
// Implementations: GCPVision, Noop.
type Provider interface {
	ExtractText(ctx context.Context, data []byte) (string, error)
}
