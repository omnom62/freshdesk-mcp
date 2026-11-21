package extract

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// JSON pretty-prints raw JSON bytes as a string.
func JSON(data []byte) (string, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return "", fmt.Errorf("indent json: %w", err)
	}
	return buf.String(), nil
}
