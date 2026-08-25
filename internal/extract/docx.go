package extract


import (
	"errors"
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

var ErrDocxMissingDocument = errors.New("word/document.xml not found in docx")

// Docx extracts plain text from a .docx file bytes.
func Docx(data []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("open docx zip: %w", err)
	}

	for _, f := range r.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("open document.xml: %w", err)
		}
		defer rc.Close()

		return extractXMLText(rc)
	}

	return "", ErrDocxMissingDocument
}

func extractXMLText(r io.Reader) (string, error) {
	var sb strings.Builder
	dec := xml.NewDecoder(r)

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("decode xml: %w", err)
		}

		if cd, ok := tok.(xml.CharData); ok {
			text := strings.TrimSpace(string(cd))
			if text != "" {
				sb.WriteString(text)
				sb.WriteString(" ")
			}
		}
	}

	return strings.TrimSpace(sb.String()), nil
}
