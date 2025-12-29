package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// DefaultTimeout is the default HTTP request timeout
	DefaultTimeout = 10 * time.Second
)

// GetHTML fetches HTML content from the given URL with a timeout.
// Returns the HTML bytes on success, or an error if the request fails or returns non-200 status.
func GetHTML(ctx context.Context, url string) ([]byte, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: DefaultTimeout,
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set Accept header to request HTML content
	req.Header.Set("Accept", "text/html")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status %d (expected 200)", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}
