// Package uploader posts normalized QSOs to the logger's bridge ingest
// endpoint (POST /api/logsheets/{id}/qso), authenticated with the
// logsheet's ingest token.
package uploader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yc2utc/zs-logger-bridge/internal/qso"
)

// Result describes the outcome of a successful POST.
type Result struct {
	Duplicate bool
	LogID     int
}

// Client posts QSOs to one logsheet on the logger.
type Client struct {
	// ServerURL is the logger base URL, e.g. "https://logger.amatir.id"
	// (no trailing slash required).
	ServerURL string
	// LogsheetID is the numeric logsheet id this bridge feeds.
	LogsheetID string
	// Token is the logsheet's ingest_token.
	Token string

	HTTP *http.Client
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (c *Client) url() string {
	base := strings.TrimRight(c.ServerURL, "/")
	return fmt.Sprintf("%s/api/logsheets/%s/qso", base, c.LogsheetID)
}

// Send posts one QSO, retrying transient failures (network errors, 5xx)
// with exponential backoff. It does not retry 4xx responses -- those mean
// the request itself is wrong (bad token, failed validation) and won't
// succeed on retry.
func (c *Client) Send(ctx context.Context, q qso.QSO) (*Result, error) {
	body, err := json.Marshal(q.ToPayload())
	if err != nil {
		return nil, fmt.Errorf("uploader: encode payload: %w", err)
	}

	const maxAttempts = 3
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, retryable, err := c.attempt(ctx, body)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retryable || attempt == maxAttempts {
			break
		}

		backoff := time.Duration(attempt) * time.Second
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}

	return nil, lastErr
}

func (c *Client) attempt(ctx context.Context, body []byte) (*Result, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(), bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("uploader: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		// Network-level failure: retryable.
		return nil, true, fmt.Errorf("uploader: request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("uploader: server error %d: %s", resp.StatusCode, string(respBody))
	}
	if resp.StatusCode >= 400 {
		return nil, false, fmt.Errorf("uploader: rejected (%d): %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Success   bool `json:"success"`
		Duplicate bool `json:"duplicate"`
		Log       struct {
			ID int `json:"id"`
		} `json:"log"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		// 2xx but unparsable body -- treat as success without details rather
		// than retrying (retrying would risk creating a duplicate QSO).
		return &Result{}, false, nil
	}

	return &Result{Duplicate: parsed.Duplicate, LogID: parsed.Log.ID}, false, nil
}
