package extract

import (
	"encoding/json"
	"fmt"
	"strings"
)

// QueryJSON filters a JSON object (map of objects) by searching for a value
// in any field. Returns matching entries as pretty-printed JSON.
func QueryJSON(data []byte, query string) (string, error) {
	var records map[string]json.RawMessage
	if err := json.Unmarshal(data, &records); err != nil {
		return "", fmt.Errorf("parse json: %w", err)
	}

	q := strings.ToLower(query)
	matches := make(map[string]json.RawMessage)

	for key, raw := range records {
		if strings.Contains(strings.ToLower(key), q) ||
			strings.Contains(strings.ToLower(string(raw)), q) {
			matches[key] = raw
		}
	}

	if len(matches) == 0 {
		return "no matches found", nil
	}

	out, err := json.MarshalIndent(matches, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal results: %w", err)
	}

	return fmt.Sprintf("found %d matches:\n%s", len(matches), string(out)), nil
}
