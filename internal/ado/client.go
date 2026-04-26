package ado

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client is a minimal Azure DevOps REST client.
type Client struct {
	BaseURL    string // e.g. https://dev.azure.com/<org>
	HTTP       *http.Client
	Tokens     TokenProvider
	UserAgent  string
	MaxRetries int
}

func NewClient(org string, tokens TokenProvider) *Client {
	return &Client{
		BaseURL:    "https://dev.azure.com/" + url.PathEscape(org),
		HTTP:       &http.Client{Timeout: 30 * time.Second},
		Tokens:     tokens,
		UserAgent:  "adotop/0.1",
		MaxRetries: 3,
	}
}

// APIError is returned for non-2xx responses.
type APIError struct {
	Status int
	URL    string
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("ado: %s -> %d: %s", e.URL, e.Status, truncate(e.Body, 300))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// GetJSON GETs path (relative to BaseURL) and decodes JSON into out.
// Adds api-version automatically if not present in path's query string.
func (c *Client) GetJSON(ctx context.Context, path string, out any) error {
	u, err := c.resolve(path)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodGet, u, nil, out)
}

func (c *Client) resolve(path string) (string, error) {
	if path == "" || path[0] != '/' {
		path = "/" + path
	}
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return "", err
	}
	q := u.Query()
	if q.Get("api-version") == "" {
		q.Set("api-version", APIVersion)
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

func (c *Client) do(ctx context.Context, method, fullURL string, body io.Reader, out any) error {
	var bodyBytes []byte
	if body != nil {
		b, err := io.ReadAll(body)
		if err != nil {
			return err
		}
		bodyBytes = b
	}
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return err
		}
		token, err := c.Tokens.Token(ctx)
		if err != nil {
			return fmt.Errorf("get token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.UserAgent)
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			if !shouldRetry(err) || attempt == c.MaxRetries {
				return err
			}
			sleepBackoff(ctx, attempt, 0)
			continue
		}

		// 401 -> drop cached token and retry once.
		if resp.StatusCode == http.StatusUnauthorized && attempt < c.MaxRetries {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			c.Tokens.Invalidate()
			slog.Debug("ado: 401, refreshing token", "url", fullURL)
			continue
		}

		// 429 / 503 with Retry-After
		if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable) && attempt < c.MaxRetries {
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			slog.Debug("ado: rate-limited, retrying", "url", fullURL, "after", retryAfter)
			sleepBackoff(ctx, attempt, retryAfter)
			continue
		}

		if resp.StatusCode >= 500 && attempt < c.MaxRetries {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			sleepBackoff(ctx, attempt, 0)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return &APIError{Status: resp.StatusCode, URL: fullURL, Body: string(respBody)}
		}
		if out == nil || len(respBody) == 0 {
			return nil
		}
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode %s: %w", fullURL, err)
		}
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("ado: exhausted retries")
	}
	return lastErr
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

func parseRetryAfter(h string) time.Duration {
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}

func sleepBackoff(ctx context.Context, attempt int, hint time.Duration) {
	d := hint
	if d <= 0 {
		d = time.Duration(1<<attempt) * 250 * time.Millisecond
		if d > 5*time.Second {
			d = 5 * time.Second
		}
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
