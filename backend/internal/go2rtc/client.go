// Package go2rtc is a thin HTTP client for the go2rtc REST API. The BFF
// uses it to list configured stream aliases so the stream-override editor
// (sprint B) can validate user input before persisting it.
package go2rtc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
)

// Client is a minimal go2rtc REST client. The HTTP transport is injected
// so the BFF can share its long-lived *http.Client across upstreams.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New builds a Client targeting baseURL (e.g. http://frigate:1984).
// The caller owns httpClient; no internal allocation.
func New(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: baseURL, httpClient: httpClient}
}

// GetStreams fetches /api/streams and returns the sorted alias list.
// go2rtc returns a JSON object keyed by alias; values carry per-stream
// metadata we don't currently need, so they're discarded.
func (c *Client) GetStreams(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf("%s/api/streams", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("go2rtc streams: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("failed to close response body", "error", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("go2rtc streams returned %d", resp.StatusCode)
	}
	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode streams: %w", err)
	}
	names := make([]string, 0, len(raw))
	for name := range raw {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
