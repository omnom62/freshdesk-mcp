package ml

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

var ErrEmptyGCPToken = errors.New("empty access token from metadata server")

// getGCPToken fetches a GCP access token from the metadata server.
// On Cloud Run this works automatically via the service account.
// Locally, set GOOGLE_APPLICATION_CREDENTIALS to a service account key file
// and ensure the token endpoint is available.
func getGCPToken(ctx context.Context) (string, error) {
	const metadataURL = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return "", fmt.Errorf("create metadata request: %w", err)
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch metadata token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}

	if result.AccessToken == "" {
		return "", ErrEmptyGCPToken
	}

	return result.AccessToken, nil
}
