package frigate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a Frigate REST client. Timeout policy lives on the caller:
// every method here uses http.NewRequestWithContext, so the per-call context
// deadline is the only bound. Pass a custom *http.Client to share connection
// pools or impose a client-level timeout (the probe path does this); pass nil
// to get a fresh &http.Client{} with no Timeout.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
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

// OpenVOD issues a Frigate recording VOD request for the supplied camera
// and [start, end] window, returning the raw upstream *http.Response so
// the caller can stream the body through. start and end are integer
// unix-seconds STRINGS passed verbatim (the handler validates them as
// digit-only before calling); rest is the already-validated relative
// playlist or segment filename (master.m3u8, index-*.m3u8, init-*.mp4,
// or seg-*.m4s). method is the caller's HTTP method (GET or HEAD).
// When rangeHeader is non-empty it is forwarded as the upstream Range
// header so byte-range playback flows end-to-end. The caller owns the
// returned response body and is responsible for closing it. Any status
// code is returned to the caller; non-2xx is not treated as an error
// here (same contract as OpenPreview).
func (c *Client) OpenVOD(ctx context.Context, method, cam, start, end, rest, rangeHeader string) (*http.Response, error) {
	reqURL := c.baseURL + "/vod/" + url.PathEscape(cam) +
		"/start/" + start +
		"/end/" + end +
		"/" + rest
	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("frigate vod: %w", err)
	}
	return resp, nil
}

// GetRecordingsSummary fetches the per-day recordings summary for a
// camera from Frigate's /api/{cam}/recordings/summary endpoint. The
// optional timezone query parameter is forwarded only when non-empty
// (Frigate uses it to bucket entries by the caller's local day).
// Returns the raw upstream *http.Response so the BFF handler can copy
// the Content-Type and stream the JSON body verbatim — the response
// shape is owned upstream and not yet typed here. Any status code is
// returned to the caller; non-2xx is not an error.
func (c *Client) GetRecordingsSummary(ctx context.Context, cam, timezone string) (*http.Response, error) {
	reqURL := c.baseURL + "/api/" + url.PathEscape(cam) + "/recordings/summary"
	if timezone != "" {
		q := url.Values{}
		q.Set("timezone", timezone)
		reqURL = reqURL + "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("frigate recordings summary: %w", err)
	}
	return resp, nil
}
