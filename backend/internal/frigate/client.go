package frigate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// GetSnapshot fetches the latest snapshot for a camera.
// Caller is responsible for closing the returned body.
func (c *Client) GetSnapshot(ctx context.Context, camID string) (io.ReadCloser, string, error) {
	url := fmt.Sprintf("%s/api/%s/latest.jpg", c.baseURL, camID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("frigate request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("failed to close response body", "error", err)
		}
		return nil, "", fmt.Errorf("frigate returned %d", resp.StatusCode)
	}
	return resp.Body, resp.Header.Get("ETag"), nil
}

// ProbeSnapshot returns true if the camera's snapshot endpoint responds 200.
// Uses GET because Frigate 0.17 returns 405 for HEAD on this endpoint.
func (c *Client) ProbeSnapshot(ctx context.Context, camID string) bool {
	url := fmt.Sprintf("%s/api/%s/latest.jpg", c.baseURL, camID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("failed to close response body", "error", err)
		}
	}()
	return resp.StatusCode == http.StatusOK
}

// GetStats fetches /api/stats from Frigate and returns the parsed response.
func (c *Client) GetStats(ctx context.Context) (*StatsResponse, error) {
	url := fmt.Sprintf("%s/api/stats", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("frigate stats: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("failed to close response body", "error", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("frigate stats returned %d", resp.StatusCode)
	}
	var stats StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("decode stats: %w", err)
	}
	return &stats, nil
}

// GetConfig fetches /api/config from Frigate and returns the parsed response.
// Only the cameras subtree is decoded; the rest of the Frigate config is
// dropped silently.
func (c *Client) GetConfig(ctx context.Context) (*ConfigResponse, error) {
	url := fmt.Sprintf("%s/api/config", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("frigate config: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("failed to close response body", "error", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("frigate config returned %d", resp.StatusCode)
	}
	var cfg ConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	return &cfg, nil
}
