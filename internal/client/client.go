// Package client is a small, typed client for the listmonk REST API
// (https://listmonk.app/docs/apis/apis/). It covers only what the Terraform
// provider manages; it is not a general-purpose SDK.
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// Config holds the connection settings for a Client.
type Config struct {
	// BaseURL is the root URL of the listmonk instance, e.g.
	// https://listmonk.example.com. The /api prefix is added by the client.
	BaseURL string
	// Username and Token are the credentials of a listmonk API user
	// (Admin -> Users -> type "API").
	Username string
	Token    string
	// Timeout per HTTP request. Zero means 30 seconds.
	Timeout time.Duration
	// InsecureSkipVerify disables TLS certificate verification.
	InsecureSkipVerify bool
	// MaxRetries is the number of retries for transient failures (connection
	// errors, 429, 502, 503, 504). listmonk restarts itself after a settings
	// change, so a request right after one can briefly fail. Negative means
	// no retries; zero means the default (5).
	MaxRetries int
	// UserAgent is sent with every request.
	UserAgent string
}

// Client talks to one listmonk instance.
type Client struct {
	baseURL    *url.URL
	authHeader string
	http       *http.Client
	maxRetries int
	userAgent  string
	// retryWait is the base delay between retries; tests shorten it.
	retryWait time.Duration
	// noSettingsByKey is set once the server turned out not to have
	// PUT /api/settings/{key} (listmonk < v6).
	noSettingsByKey atomic.Bool
}

// New validates cfg and returns a Client.
func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("listmonk URL is required")
	}
	u, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid listmonk URL %q: %w", cfg.BaseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("invalid listmonk URL %q: scheme must be http or https", cfg.BaseURL)
	}
	if cfg.Username == "" || cfg.Token == "" {
		return nil, errors.New("listmonk API username and token are required")
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	retries := cfg.MaxRetries
	switch {
	case retries == 0:
		retries = 5
	case retries < 0:
		retries = 0
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.InsecureSkipVerify {
		// Explicit opt-in through insecure_skip_verify.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 //nolint:gosec
	}

	ua := cfg.UserAgent
	if ua == "" {
		ua = "terraform-provider-listmonk"
	}

	return &Client{
		baseURL:    u,
		authHeader: "token " + cfg.Username + ":" + cfg.Token,
		http:       &http.Client{Timeout: timeout, Transport: transport},
		maxRetries: retries,
		userAgent:  ua,
		retryWait:  time.Second,
	}, nil
}

// APIError is a non-2xx response from listmonk.
type APIError struct {
	Method     string
	Path       string
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("listmonk API %s %s: HTTP %d: %s", e.Method, e.Path, e.StatusCode, e.Message)
}

// IsNotFound reports whether err means the object does not exist.
//
// listmonk is inconsistent here: users return 404, but lists, templates and
// subscribers return 400 with a (localised) "not found" message. Callers
// only ask this about GET-by-ID requests with a well-formed ID, where 400
// has no other cause, so both codes count.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusBadRequest
}

// envelope is listmonk's response wrapper: {"data": ...} on success,
// {"message": "..."} on failure.
type envelope struct {
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
}

// do sends a request with an optional JSON body and decodes the "data" field
// of the response into out (when out is non-nil).
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if raw, ok := body.(json.RawMessage); ok {
			payload = raw
		} else if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
	}

	u := *c.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + "/api" + path
	if query != nil {
		u.RawQuery = query.Encode()
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			wait := c.retryWait * time.Duration(1<<(attempt-1))
			if wait > 10*c.retryWait {
				wait = 10 * c.retryWait
			}
			select {
			case <-ctx.Done():
				return errors.Join(ctx.Err(), lastErr)
			case <-time.After(wait):
			}
		}

		var reader io.Reader
		if payload != nil {
			reader = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", c.authHeader)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			if isTransientNetErr(err) && ctx.Err() == nil {
				lastErr = err
				continue
			}
			return fmt.Errorf("listmonk API %s %s: %w", method, path, err)
		}

		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out == nil {
				return nil
			}
			var env envelope
			if err := json.Unmarshal(respBody, &env); err != nil {
				return fmt.Errorf("listmonk API %s %s: decoding response: %w", method, path, err)
			}
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("listmonk API %s %s: decoding data: %w", method, path, err)
			}
			return nil
		}

		apiErr := &APIError{Method: method, Path: path, StatusCode: resp.StatusCode}
		var env envelope
		if json.Unmarshal(respBody, &env) == nil && env.Message != "" {
			apiErr.Message = env.Message
		} else {
			apiErr.Message = strings.TrimSpace(string(respBody))
		}

		switch resp.StatusCode {
		case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			lastErr = apiErr
			continue
		}
		return apiErr
	}
	return lastErr
}

func isTransientNetErr(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	// Connection refused/reset while listmonk restarts after a settings change.
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "connection reset")
}

// Health calls GET /api/health. It doubles as a credentials check because
// the endpoint requires authentication.
func (c *Client) Health(ctx context.Context) error {
	var ok bool
	return c.do(ctx, http.MethodGet, "/health", nil, nil, &ok)
}
