package extract

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/omnom62/freshdesk-mcp/internal/ocr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

func makeDocx(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("word/document.xml")
	require.NoError(t, err)
	_, err = f.Write(
		[]byte(
			`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + content + `</w:t></w:r></w:p></w:body></w:document>`,
		),
	)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func makeEmptyZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, zip.NewWriter(&buf).Close())
	return buf.Bytes()
}

// --- JSON ---

func TestJSON_Valid(t *testing.T) {
	result, err := JSON([]byte(`{"key":"value"}`))
	require.NoError(t, err)
	assert.Contains(t, result, `"key"`)
	assert.Contains(t, result, `"value"`)
}

func TestJSON_Invalid(t *testing.T) {
	_, err := JSON([]byte(`not json`))
	assert.Error(t, err)
}

func TestJSON_Empty(t *testing.T) {
	_, err := JSON([]byte(``))
	assert.Error(t, err)
}

// --- QueryJSON ---

func TestQueryJSON_MatchByValue(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"ticket1": map[string]string{"subject": "network issue"},
		"ticket2": map[string]string{"subject": "billing question"},
	})
	result, err := QueryJSON(data, "network")
	require.NoError(t, err)
	assert.Contains(t, result, "ticket1")
	assert.NotContains(t, result, "ticket2")
}

func TestQueryJSON_MatchByKey(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"network_ticket": map[string]string{"subject": "foo"},
		"billing_ticket": map[string]string{"subject": "bar"},
	})
	result, err := QueryJSON(data, "network")
	require.NoError(t, err)
	assert.Contains(t, result, "network_ticket")
}

func TestQueryJSON_NoMatch(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"ticket1": map[string]string{"subject": "billing"},
	})
	result, err := QueryJSON(data, "network")
	require.NoError(t, err)
	assert.Equal(t, "no matches found", result)
}

func TestQueryJSON_CaseInsensitive(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"ticket1": map[string]string{"subject": "NETWORK issue"},
	})
	result, err := QueryJSON(data, "network")
	require.NoError(t, err)
	assert.Contains(t, result, "ticket1")
}

func TestQueryJSON_InvalidJSON(t *testing.T) {
	_, err := QueryJSON([]byte(`not json`), "query")
	assert.Error(t, err)
}

// --- Docx ---

func TestDocx_Valid(t *testing.T) {
	data := makeDocx(t, "Hello World")
	result, err := Docx(data)
	require.NoError(t, err)
	assert.Contains(t, result, "Hello World")
}

func TestDocx_MissingDocumentXML(t *testing.T) {
	data := makeEmptyZip(t)
	_, err := Docx(data)
	assert.ErrorIs(t, err, ErrDocxMissingDocument)
}

func TestDocx_InvalidZip(t *testing.T) {
	_, err := Docx([]byte("not a zip"))
	assert.Error(t, err)
}

// --- FromAttachment routing ---

func TestFromAttachment_JSON_byContentType(t *testing.T) {
	data := []byte(`{"key":"value"}`)
	result, err := FromAttachment(context.Background(), "file.json", "application/json", data, ocr.Noop{})
	require.NoError(t, err)
	assert.Contains(t, result, "key")
}

func TestFromAttachment_JSON_byExtension(t *testing.T) {
	data := []byte(`{"key":"value"}`)
	result, err := FromAttachment(context.Background(), "file.json", "application/octet-stream", data, ocr.Noop{})
	require.NoError(t, err)
	assert.Contains(t, result, "key")
}

func TestFromAttachment_TXT(t *testing.T) {
	data := []byte("plain text content")
	result, err := FromAttachment(context.Background(), "file.txt", "text/plain", data, ocr.Noop{})
	require.NoError(t, err)
	assert.Equal(t, "plain text content", result)
}

func TestFromAttachment_CSV(t *testing.T) {
	data := []byte("a,b,c\n1,2,3")
	result, err := FromAttachment(context.Background(), "file.csv", "text/csv", data, ocr.Noop{})
	require.NoError(t, err)
	assert.Contains(t, result, "a,b,c")
}

func TestFromAttachment_Docx(t *testing.T) {
	data := makeDocx(t, "docx content")
	result, err := FromAttachment(
		context.Background(),
		"file.docx",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		data,
		ocr.Noop{},
	)
	require.NoError(t, err)
	assert.Contains(t, result, "docx content")
}

func TestFromAttachment_Unsupported(t *testing.T) {
	_, err := FromAttachment(context.Background(), "file.pdf", "application/pdf", []byte("data"), ocr.Noop{})
	assert.ErrorIs(t, err, ErrUnsupportedType)
}

func TestFromAttachment_Docx_byExtension(t *testing.T) {
	data := makeDocx(t, "docx by ext")
	result, err := FromAttachment(context.Background(), "file.docx", "application/octet-stream", data, ocr.Noop{})
	require.NoError(t, err)
	assert.Contains(t, result, "docx by ext")
}
