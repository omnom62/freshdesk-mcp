package extract

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrUnsupportedType = errors.New("unsupported attachment type")

// FromAttachment routes to the correct extractor based on content type and filename.
//
//nolint:cyclop
func FromAttachment(ctx context.Context, name, contentType string, data []byte, gcpProject string) (string, error) {
	switch {
	case contentType == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" ||
		strings.HasSuffix(name, ".docx"):
		return Docx(data)
	case contentType == "application/json" ||
		strings.HasSuffix(name, ".json"):
		return JSON(data)
	case contentType == "text/plain" ||
		strings.HasSuffix(name, ".txt"):
		return string(data), nil
	case contentType == "text/csv" ||
		contentType == "application/csv" ||
		strings.HasSuffix(name, ".csv"):
		return string(data), nil
	case contentType == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
		strings.HasSuffix(name, ".xlsx"):
		return Xlsx(data)
	case contentType == "image/png" ||
		contentType == "image/jpeg" ||
		contentType == "image/jpg" ||
		strings.HasSuffix(name, ".png") ||
		strings.HasSuffix(name, ".jpg") ||
		strings.HasSuffix(name, ".jpeg"):
		return ImageOCR(ctx, data, gcpProject)
	default:
		return "", fmt.Errorf("%w: %s (%s)", ErrUnsupportedType, name, contentType)
	}
}
