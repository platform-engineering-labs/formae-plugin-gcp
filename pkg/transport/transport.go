// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/config"
	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"google.golang.org/api/googleapi"
	htransport "google.golang.org/api/transport/http"
)

const (
	DefaultRequestTimeout = 5 * time.Minute
	DefaultUserAgent      = "formae-gcp-plugin/1.0"
)

// Client wraps HTTP client for GCP REST API calls
type Client struct {
	httpClient *http.Client
	config     *config.Config
	userAgent  string
}

// NewClient creates a new transport client
func NewClient(ctx context.Context, cfg *config.Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Create OAuth2 HTTP client
	opts, err := cfg.ToClientOptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create client options: %w", err)
	}

	// Get authenticated HTTP client using htransport
	httpClient, _, err := htransport.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	return &Client{
		httpClient: httpClient,
		config:     cfg,
		userAgent:  DefaultUserAgent,
	}, nil
}

// RequestOptions configures a REST API request
type RequestOptions struct {
	Method  string
	URL     string
	Body    map[string]interface{}
	Headers http.Header
	Timeout time.Duration

	// RawBody sends bytes verbatim instead of marshalling Body as JSON, for the
	// APIs that take content rather than a resource description - a Cloud
	// Storage object upload is the only one so far. Set ContentType alongside
	// it; without one the request would claim to be JSON and the upload would
	// be stored with the wrong type. Body is ignored when RawBody is set.
	RawBody     []byte
	ContentType string
}

// Response represents a REST API response
type Response struct {
	StatusCode int
	Body       map[string]interface{}
	Headers    http.Header
}

// SendRequest makes a REST API call
// Note: Retries are handled by metastructure's PluginOperator, not at the transport layer.
// This ensures consistent retry behavior across all plugins and proper error code propagation.
func (c *Client) SendRequest(ctx context.Context, opts RequestOptions) (*Response, error) {
	if opts.Timeout == 0 {
		opts.Timeout = DefaultRequestTimeout
	}

	// Set default headers
	headers := opts.Headers
	if headers == nil {
		headers = make(http.Header)
	}
	headers.Set("User-Agent", c.userAgent)
	if opts.ContentType != "" {
		headers.Set("Content-Type", opts.ContentType)
	} else {
		headers.Set("Content-Type", "application/json")
	}

	// Add project header for billing
	if c.config.Project != "" {
		headers.Set("X-Goog-User-Project", c.config.Project)
	}

	// Encode body
	var bodyReader io.Reader
	if opts.RawBody != nil {
		bodyReader = bytes.NewReader(opts.RawBody)
	} else if opts.Body != nil {
		bodyBytes, err := json.Marshal(opts.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Add alt=json query param
	requestURL, err := AddQueryParam(opts.URL, "alt", "json")
	if err != nil {
		return nil, fmt.Errorf("failed to add query param: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, opts.Method, requestURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header = headers

	// Execute request
	response, err := c.httpClient.Do(req)
	if err != nil {
		if isReauthError(err) {
			plugin.LoggerFromContext(ctx).Warn(
				"GCP reauth required: Workspace org reauth policy returned `invalid_rapt`. "+
					"Re-authenticate with `gcloud auth application-default login`, or switch to a "+
					"service account by setting GOOGLE_APPLICATION_CREDENTIALS — service accounts "+
					"are not subject to reauth policies",
				"err", err.Error(),
				"url", requestURL,
				"method", opts.Method,
			)
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Check for HTTP errors
	if err := googleapi.CheckResponse(response); err != nil {
		googleapi.CloseBody(response)
		return nil, err
	}

	if response == nil {
		return nil, fmt.Errorf("no response received from API")
	}

	defer googleapi.CloseBody(response)

	// Handle 204 No Content
	if response.StatusCode == 204 {
		return &Response{
			StatusCode: response.StatusCode,
			Headers:    response.Header,
		}, nil
	}

	// Decode response body
	var responseBody map[string]interface{}
	if err := json.NewDecoder(response.Body).Decode(&responseBody); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &Response{
		StatusCode: response.StatusCode,
		Body:       responseBody,
		Headers:    response.Header,
	}, nil
}

// SendRaw makes a request and returns the response body verbatim, for the
// endpoints that answer with content rather than JSON - reading a Cloud Storage
// object's bytes with alt=media is the only one so far. SendRequest would try
// to decode those bytes as JSON and fail on anything that is not.
func (c *Client) SendRaw(ctx context.Context, opts RequestOptions) ([]byte, error) {
	if opts.Timeout == 0 {
		opts.Timeout = DefaultRequestTimeout
	}
	headers := opts.Headers
	if headers == nil {
		headers = make(http.Header)
	}
	headers.Set("User-Agent", c.userAgent)
	if c.config.Project != "" {
		headers.Set("X-Goog-User-Project", c.config.Project)
	}

	req, err := http.NewRequestWithContext(ctx, opts.Method, opts.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header = headers

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if err := googleapi.CheckResponse(response); err != nil {
		googleapi.CloseBody(response)
		return nil, err
	}
	defer googleapi.CloseBody(response)

	return io.ReadAll(response.Body)
}

// isReauthError detects the `invalid_rapt` / reauth-required OAuth response
// that Google Workspace organizations issue when their reauth policy expires
// an ADC session. Mirrors the detection in formae-plugin-k8s's GKE auth
// provider — both plugins hit the same failure mode independently.
func isReauthError(err error) bool {
	if err == nil {
		return false
	}
	m := err.Error()
	return strings.Contains(m, "invalid_rapt") ||
		strings.Contains(m, "reauth related error")
}

// AddQueryParam adds a query parameter to a URL
func AddQueryParam(rawURL, key, value string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// AddQueryParams adds multiple query parameters to a URL
func AddQueryParams(rawURL string, params map[string]string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
