package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jians1/cf-tunnel/internal/cftunnel/credentials"
)

const DefaultQuickService = "https://api.trycloudflare.com"

type Client struct {
	baseURL       string
	userAgent     string
	httpClient    *http.Client
	retryBackoffs []time.Duration
}

type ClientOptions struct {
	Timeout       time.Duration
	RetryBackoffs []time.Duration
}

type QuickTunnelResponse struct {
	Success bool               `json:"success"`
	Result  QuickTunnel        `json:"result"`
	Errors  []QuickTunnelError `json:"errors"`
}

type QuickTunnelError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type QuickTunnel struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Hostname   string `json:"hostname"`
	AccountTag string `json:"account_tag"`
	Secret     []byte `json:"secret"`
}

type QuickTunnelReservation struct {
	Credentials credentials.Credentials
	Hostname    string
	URL         string
	Name        string
}

type QuickTunnelRateLimitedError struct {
	err error
}

func (e *QuickTunnelRateLimitedError) Error() string {
	return e.err.Error()
}

func (e *QuickTunnelRateLimitedError) Unwrap() error {
	return e.err
}

func NewClient(baseURL, userAgent string) *Client {
	return NewClientWithOptions(baseURL, userAgent, ClientOptions{})
}

func NewClientWithOptions(baseURL, userAgent string, opts ClientOptions) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = DefaultQuickService
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	backoffs := opts.RetryBackoffs
	if backoffs == nil {
		backoffs = []time.Duration{500 * time.Millisecond, 1500 * time.Millisecond}
	}
	return &Client{
		baseURL:       baseURL,
		userAgent:     userAgent,
		retryBackoffs: append([]time.Duration(nil), backoffs...),
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSHandshakeTimeout:   timeout,
				ResponseHeaderTimeout: timeout,
			},
			Timeout: timeout,
		},
	}
}

func (c *Client) CreateQuickTunnel(ctx context.Context) (*QuickTunnelReservation, error) {
	var lastErr error
	backoffs := c.retryBackoffs

	for attempt := 0; attempt <= len(backoffs); attempt++ {
		reservation, err := c.createQuickTunnelOnce(ctx)
		if err == nil {
			return reservation, nil
		}
		lastErr = err

		var rlErr *QuickTunnelRateLimitedError
		if !errors.As(err, &rlErr) || attempt == len(backoffs) {
			return nil, lastErr
		}

		timer := time.NewTimer(backoffs[attempt])
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("quick tunnel retry canceled: %w", ctx.Err())
		case <-timer.C:
		}
	}

	return nil, lastErr
}

func (c *Client) createQuickTunnelOnce(ctx context.Context) (*QuickTunnelReservation, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/tunnel", nil)
	if err != nil {
		return nil, fmt.Errorf("build quick tunnel request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request quick tunnel: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read quick tunnel response: %w", err)
	}
	if isRateLimited(resp.StatusCode, body) {
		return nil, &QuickTunnelRateLimitedError{
			err: fmt.Errorf(
				"quick tunnel request rate limited (status=%s content-type=%q body=%q)",
				resp.Status,
				resp.Header.Get("Content-Type"),
				snippet(body),
			),
		}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf(
			"quick tunnel request failed with HTTP status %s (content-type=%q body=%q)",
			resp.Status,
			resp.Header.Get("Content-Type"),
			snippet(body),
		)
	}

	var parsed QuickTunnelResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf(
			"decode quick tunnel response: %w (status=%s content-type=%q body=%q)",
			err,
			resp.Status,
			resp.Header.Get("Content-Type"),
			snippet(body),
		)
	}
	if !parsed.Success {
		return nil, responseError(resp.Status, resp.Header.Get("Content-Type"), body, parsed.Errors)
	}
	if parsed.Result.ID == "" {
		return nil, fmt.Errorf("missing quick tunnel id")
	}
	if parsed.Result.Hostname == "" {
		return nil, fmt.Errorf("missing quick tunnel hostname")
	}
	if parsed.Result.AccountTag == "" {
		return nil, fmt.Errorf("missing quick tunnel account tag")
	}
	if len(parsed.Result.Secret) == 0 {
		return nil, fmt.Errorf("missing quick tunnel secret")
	}

	tunnelID, err := uuid.Parse(parsed.Result.ID)
	if err != nil {
		return nil, fmt.Errorf("parse tunnel id: %w", err)
	}

	hostname := parsed.Result.Hostname
	url := hostname
	if !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	return &QuickTunnelReservation{
		Credentials: credentials.Credentials{
			AccountTag:   parsed.Result.AccountTag,
			TunnelSecret: parsed.Result.Secret,
			TunnelID:     tunnelID,
		},
		Hostname: hostname,
		URL:      url,
		Name:     parsed.Result.Name,
	}, nil
}

func IsRateLimitedError(err error) bool {
	var rlErr *QuickTunnelRateLimitedError
	return errors.As(err, &rlErr)
}

func isRateLimited(statusCode int, body []byte) bool {
	if statusCode == http.StatusTooManyRequests {
		return true
	}
	text := strings.ToLower(string(body))
	return strings.Contains(text, "error code: 1015")
}

func responseError(status, contentType string, body []byte, items []QuickTunnelError) error {
	if len(items) == 0 {
		return fmt.Errorf(
			"quick tunnel request failed without error details (status=%s content-type=%q body=%q)",
			status,
			contentType,
			snippet(body),
		)
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%d:%s", item.Code, item.Message))
	}
	return fmt.Errorf(
		"quick tunnel request failed: %s (status=%s content-type=%q body=%q)",
		strings.Join(parts, ", "),
		status,
		contentType,
		snippet(body),
	)
}

func snippet(body []byte) string {
	const max = 200
	text := strings.TrimSpace(string(body))
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}
