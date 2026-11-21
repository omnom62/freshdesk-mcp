package extract

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Xlsx extracts text content from an .xlsx file bytes.
// Returns all sheets with their data as readable text.
func Xlsx(data []byte) (string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	var sb strings.Builder

	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			return "", fmt.Errorf("get rows from sheet %q: %w", sheet, err)
		}

		if len(rows) == 0 {
			continue
		}

		fmt.Fprintf(&sb, "=== Sheet: %s ===\n", sheet)

		for _, row := range rows {
			sb.WriteString(strings.Join(row, "\t"))
			sb.WriteString("\n")
		}

		sb.WriteString("\n")
	}

	return strings.TrimSpace(sb.String()), nil
}
