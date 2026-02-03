package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// GetFluxAPI 发送 POST 请求到 flux-panel API 并返回响应体
// url: API endpoint URL
// token: Authorization header 的值
func GetFluxAPI(ctx context.Context, url string, token string) ([]byte, error) {
	client := &http.Client{
		Timeout: DefaultTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status %d (expected 200)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}
