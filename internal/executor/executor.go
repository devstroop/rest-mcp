package executor

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/devstroop/rest-mcp/internal/cache"
	"github.com/devstroop/rest-mcp/internal/config"
	"github.com/devstroop/rest-mcp/internal/logger"
	"github.com/devstroop/rest-mcp/internal/model"
)

// Result holds the outcome of an HTTP request execution.
type Result struct {
	StatusCode int
	Body       string
	IsError    bool
}

// Executor performs HTTP requests for tool calls.
type Executor struct {
	client          *http.Client
	baseURL         string
	headers         map[string]string
	maxResponseSize int
	auth            config.Auth
	retry           config.Retry
	respCache       *cache.Cache

	// OAuth2 token cache
	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

// New creates an Executor with the given configuration.
func New(baseURL string, headers map[string]string, timeout time.Duration, maxResponseSize int, auth config.Auth, retry config.Retry) *Executor {
	return &Executor{
		client: &http.Client{
			Timeout: timeout,
		},
		baseURL:         strings.TrimRight(baseURL, "/"),
		headers:         headers,
		maxResponseSize: maxResponseSize,
		auth:            auth,
		retry:           retry,
		respCache:       cache.New(1000),
	}
}

// UpdateConfig updates the executor's configuration for hot-reload (M4-05).
// Thread-safe: called from the SIGHUP handler goroutine.
func (e *Executor) UpdateConfig(baseURL string, headers map[string]string, timeout time.Duration, maxResponseSize int, auth config.Auth, retry config.Retry) {
	e.tokenMu.Lock()
	defer e.tokenMu.Unlock()

	e.baseURL = strings.TrimRight(baseURL, "/")
	e.headers = headers
	e.client.Timeout = timeout
	e.maxResponseSize = maxResponseSize
	e.auth = auth
	e.retry = retry
	e.cachedToken = ""
	e.tokenExpiry = time.Time{}
	e.respCache.Clear()
}

// Execute performs the HTTP request described by the operation using the provided arguments.
func (e *Executor) Execute(ctx context.Context, op model.Operation, args map[string]interface{}) (*Result, error) {
	// Build URL path with path parameter substitution
	path := op.Path
	for _, p := range op.PathParams {
		if val, ok := args[p.Name]; ok {
			path = strings.ReplaceAll(path, "{"+p.Name+"}", fmt.Sprintf("%v", val))
		}
	}

	fullURL := e.baseURL + path

	// Build query parameters
	queryParams := url.Values{}
	for _, p := range op.QueryParams {
		if val, ok := args[p.Name]; ok {
			queryParams.Set(p.Name, fmt.Sprintf("%v", val))
		} else if p.Default != nil {
			queryParams.Set(p.Name, fmt.Sprintf("%v", p.Default))
		}
	}

	// Inject API key as query parameter if configured
	if e.auth.Type == "apikey_query" && e.auth.Key != "" {
		queryParams.Set(e.auth.Key, e.auth.Value)
	}

	if len(queryParams) > 0 {
		fullURL += "?" + queryParams.Encode()
	}

	// Build request body
	var bodyReader io.Reader
	if len(op.BodyParams) > 0 {
		bodyMap := make(map[string]interface{})
		for _, p := range op.BodyParams {
			if val, ok := args[p.Name]; ok {
				bodyMap[p.Name] = val
			}
		}
		if len(bodyMap) > 0 {
			bodyJSON, err := json.Marshal(bodyMap)
			if err != nil {
				return nil, fmt.Errorf("marshal request body: %w", err)
			}
			bodyReader = strings.NewReader(string(bodyJSON))
		}
	}

	// Also check for a raw "body" argument (pass-through)
	if bodyReader == nil {
		if rawBody, ok := args["body"]; ok {
			switch v := rawBody.(type) {
			case string:
				bodyReader = strings.NewReader(v)
			case map[string]interface{}:
				b, _ := json.Marshal(v)
				bodyReader = strings.NewReader(string(b))
			}
		}
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, op.Method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Inject static headers
	for k, v := range e.headers {
		req.Header.Set(k, v)
	}

	// Inject per-endpoint header overrides (M4-07)
	for k, v := range op.Headers {
		req.Header.Set(k, v)
	}

	// Inject auth credentials
	if err := e.applyAuth(ctx, req); err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	// Set Content-Type for requests with body
	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Set conditional request headers from cache (M4-09)
	cacheKey := cache.Key(op.Method, fullURL)
	if op.Method == "GET" || op.Method == "HEAD" {
		if cached := e.respCache.Get(cacheKey); cached != nil {
			if cached.ETag != "" {
				req.Header.Set("If-None-Match", cached.ETag)
			}
			if cached.LastModified != "" {
				req.Header.Set("If-Modified-Since", cached.LastModified)
			}
		}
	}

	// Log outbound request
	start := time.Now()
	logger.Debug("outbound request", map[string]interface{}{
		"method": op.Method,
		"url":    fullURL,
	})

	// Execute with retry
	maxAttempts := 1
	if e.retry.MaxAttempts > 0 {
		maxAttempts = e.retry.MaxAttempts
	}
	initialWait := parseDurationOrDefault(e.retry.InitialWait, 500*time.Millisecond)
	maxWait := parseDurationOrDefault(e.retry.MaxWait, 30*time.Second)

	var resp *http.Response
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, lastErr = e.client.Do(req)
		if lastErr == nil {
			// Success or non-retryable HTTP status
			if resp.StatusCode < 500 || attempt == maxAttempts {
				break
			}
			// Server error — close body and retry
			resp.Body.Close()
			logger.Warn("retrying request", map[string]interface{}{
				"attempt": attempt,
				"status":  resp.StatusCode,
				"method":  op.Method,
				"url":     fullURL,
			})
		} else {
			logger.Warn("request error, retrying", map[string]interface{}{
				"attempt": attempt,
				"error":   lastErr.Error(),
			})
		}

		if attempt < maxAttempts {
			wait := initialWait * time.Duration(1<<uint(attempt-1)) // exponential
			if wait > maxWait {
				wait = maxWait
			}
			select {
			case <-ctx.Done():
				if lastErr != nil {
					return nil, classifyError(lastErr)
				}
				return nil, ctx.Err()
			case <-time.After(wait):
			}

			// Rebuild request for retry (body may have been consumed)
			req, err = http.NewRequestWithContext(ctx, op.Method, fullURL, rebuildBody(op, args))
			if err != nil {
				return nil, fmt.Errorf("create retry request: %w", err)
			}
			for k, v := range e.headers {
				req.Header.Set(k, v)
			}
			for k, v := range op.Headers {
				req.Header.Set(k, v)
			}
			if err := e.applyAuth(ctx, req); err != nil {
				return nil, fmt.Errorf("auth: %w", err)
			}
			if req.Header.Get("Content-Type") == "" && req.Body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
		}
	}

	if lastErr != nil {
		return nil, classifyError(lastErr)
	}
	defer resp.Body.Close()

	latency := time.Since(start)

	// Handle 304 Not Modified — return cached response (M4-09)
	if resp.StatusCode == http.StatusNotModified {
		if cached := e.respCache.Get(cacheKey); cached != nil {
			logger.Debug("cache hit (304 Not Modified)", map[string]interface{}{
				"method": op.Method,
				"url":    fullURL,
			})
			result := &Result{
				StatusCode: cached.StatusCode,
				Body:       cached.Body,
				IsError:    cached.StatusCode >= 400,
			}
			if op.ResponsePath != "" && !result.IsError {
				if extracted, err := extractJSONPath(cached.Body, op.ResponsePath); err == nil {
					result.Body = extracted
				}
			}
			return result, nil
		}
	}

	// Read response body with size limit
	limitedReader := io.LimitReader(resp.Body, int64(e.maxResponseSize)+1)
	respBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	bodyStr := string(respBody)

	// Truncate if over limit
	truncated := false
	if len(respBody) > e.maxResponseSize {
		bodyStr = string(respBody[:e.maxResponseSize])
		truncated = true
	}

	logger.Debug("response received", map[string]interface{}{
		"method":    op.Method,
		"url":       fullURL,
		"status":    resp.StatusCode,
		"latency":   latency.String(),
		"size":      len(respBody),
		"truncated": truncated,
	})

	isError := resp.StatusCode >= 400

	result := &Result{
		StatusCode: resp.StatusCode,
		Body:       bodyStr,
		IsError:    isError,
	}

	// Apply response path extraction (M4-04)
	if op.ResponsePath != "" && !isError {
		if extracted, err := extractJSONPath(bodyStr, op.ResponsePath); err == nil {
			result.Body = extracted
		} else {
			logger.Debug("response_path extraction failed, returning full body", map[string]interface{}{
				"path":  op.ResponsePath,
				"error": err.Error(),
			})
		}
	}

	// Cache response if ETag or Last-Modified present (M4-09)
	if (op.Method == "GET" || op.Method == "HEAD") && !isError {
		etag := resp.Header.Get("ETag")
		lastMod := resp.Header.Get("Last-Modified")
		if etag != "" || lastMod != "" {
			e.respCache.Set(cacheKey, &cache.Entry{
				Body:         bodyStr,
				StatusCode:   resp.StatusCode,
				ETag:         etag,
				LastModified: lastMod,
				CachedAt:     time.Now(),
			})
			logger.Debug("response cached", map[string]interface{}{
				"method":        op.Method,
				"url":           fullURL,
				"etag":          etag,
				"last_modified": lastMod,
			})
		}
	}

	return result, nil
}

// applyAuth injects authentication credentials into the HTTP request
// based on the configured auth type.
func (e *Executor) applyAuth(ctx context.Context, req *http.Request) error {
	switch e.auth.Type {
	case "":
		// No auth configured
		return nil

	case "bearer":
		req.Header.Set("Authorization", "Bearer "+e.auth.Value)

	case "apikey_header":
		if e.auth.Key == "" {
			return fmt.Errorf("apikey_header requires auth.key to be set")
		}
		req.Header.Set(e.auth.Key, e.auth.Value)

	case "apikey_query":
		// Already handled in query parameter building
		return nil

	case "basic":
		if e.auth.Key == "" || e.auth.Value == "" {
			return fmt.Errorf("basic auth requires auth.key (username) and auth.value (password)")
		}
		creds := base64.StdEncoding.EncodeToString([]byte(e.auth.Key + ":" + e.auth.Value))
		req.Header.Set("Authorization", "Basic "+creds)

	case "oauth2_cc":
		token, err := e.getOAuth2Token(ctx)
		if err != nil {
			return fmt.Errorf("oauth2 client-credentials: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

	default:
		return fmt.Errorf("unsupported auth type %q — supported: bearer, apikey_header, apikey_query, basic, oauth2_cc", e.auth.Type)
	}

	return nil
}

// getOAuth2Token retrieves a cached OAuth2 token or fetches a new one using
// the client-credentials grant. Tokens are cached until expiry with a 30s buffer.
func (e *Executor) getOAuth2Token(ctx context.Context) (string, error) {
	e.tokenMu.Lock()
	defer e.tokenMu.Unlock()

	// Return cached token if still valid (with 30s buffer)
	if e.cachedToken != "" && time.Now().Add(30*time.Second).Before(e.tokenExpiry) {
		return e.cachedToken, nil
	}

	if e.auth.TokenURL == "" {
		return "", fmt.Errorf("oauth2_cc requires auth.token_url")
	}
	if e.auth.ClientID == "" || e.auth.ClientSecret == "" {
		return "", fmt.Errorf("oauth2_cc requires auth.client_id and auth.client_secret")
	}

	// Build token request
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {e.auth.ClientID},
		"client_secret": {e.auth.ClientSecret},
	}
	if len(e.auth.Scopes) > 0 {
		data.Set("scope", strings.Join(e.auth.Scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.auth.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	logger.Debug("fetching OAuth2 token", map[string]interface{}{
		"token_url": e.auth.TokenURL,
		"client_id": e.auth.ClientID,
	})

	resp, err := e.client.Do(req)
	if err != nil {
		return "", classifyError(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("token response missing access_token field")
	}

	// Cache the token
	e.cachedToken = tokenResp.AccessToken
	if tokenResp.ExpiresIn > 0 {
		e.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	} else {
		e.tokenExpiry = time.Now().Add(1 * time.Hour) // default 1h if not specified
	}

	logger.Info("OAuth2 token obtained", map[string]interface{}{
		"expires_in": tokenResp.ExpiresIn,
	})

	return e.cachedToken, nil
}

// rebuildBody reconstructs the request body for retry attempts.
func rebuildBody(op model.Operation, args map[string]interface{}) io.Reader {
	if len(op.BodyParams) > 0 {
		bodyMap := make(map[string]interface{})
		for _, p := range op.BodyParams {
			if val, ok := args[p.Name]; ok {
				bodyMap[p.Name] = val
			}
		}
		if len(bodyMap) > 0 {
			bodyJSON, _ := json.Marshal(bodyMap)
			return strings.NewReader(string(bodyJSON))
		}
	}
	if rawBody, ok := args["body"]; ok {
		switch v := rawBody.(type) {
		case string:
			return strings.NewReader(v)
		case map[string]interface{}:
			b, _ := json.Marshal(v)
			return strings.NewReader(string(b))
		}
	}
	return nil
}

// parseDurationOrDefault parses a duration string and returns a default if invalid/empty.
func parseDurationOrDefault(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}

// extractJSONPath extracts a value from a JSON string using dot-notation path.
// Supports paths like "data", "data.items", "results.0.name".
func extractJSONPath(jsonStr string, path string) (string, error) {
	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}

	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			val, ok := v[part]
			if !ok {
				return "", fmt.Errorf("key %q not found in object", part)
			}
			current = val
		case []interface{}:
			// Try to parse as array index
			var idx int
			if _, err := fmt.Sscanf(part, "%d", &idx); err != nil {
				return "", fmt.Errorf("expected array index, got %q", part)
			}
			if idx < 0 || idx >= len(v) {
				return "", fmt.Errorf("array index %d out of bounds (length %d)", idx, len(v))
			}
			current = v[idx]
		default:
			return "", fmt.Errorf("cannot traverse into %T at %q", current, part)
		}
	}

	// Marshal the result back to JSON
	result, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal extracted value: %w", err)
	}
	return string(result), nil
}

// classifyError inspects an HTTP request error and returns a user-friendly
// error message that identifies the root cause (DNS, TLS, timeout, etc.).
func classifyError(err error) error {
	if err == nil {
		return nil
	}

	// Context cancellation or deadline exceeded
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("request cancelled by client")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("request timed out — increase REQUEST_TIMEOUT if the API is slow")
	}

	// Unwrap *url.Error to get at the underlying cause
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		cause := urlErr.Err

		// Timeout (wraps context.DeadlineExceeded or net.Error with Timeout())
		if errors.Is(cause, context.DeadlineExceeded) {
			return fmt.Errorf("request timed out — increase REQUEST_TIMEOUT if the API is slow")
		}
		var netErr net.Error
		if errors.As(cause, &netErr) && netErr.Timeout() {
			return fmt.Errorf("request timed out — increase REQUEST_TIMEOUT if the API is slow")
		}

		// DNS resolution failure
		var dnsErr *net.DNSError
		if errors.As(cause, &dnsErr) {
			return fmt.Errorf("DNS lookup failed for %q — check that BASE_URL hostname is correct", dnsErr.Name)
		}

		// Connection refused
		var opErr *net.OpError
		if errors.As(cause, &opErr) {
			if opErr.Op == "dial" {
				return fmt.Errorf("connection refused — is the API server running at %s?", urlErr.URL)
			}
			return fmt.Errorf("network error (%s): %s", opErr.Op, opErr.Err)
		}

		// TLS errors
		var tlsRecordErr *tls.RecordHeaderError
		if errors.As(cause, &tlsRecordErr) {
			return fmt.Errorf("TLS error — the server may not support HTTPS, try using http:// in BASE_URL")
		}
		// Check for generic TLS errors via string matching as a fallback
		if strings.Contains(cause.Error(), "tls:") || strings.Contains(cause.Error(), "x509:") || strings.Contains(cause.Error(), "certificate") {
			return fmt.Errorf("TLS/certificate error: %s — check that the API uses a valid SSL certificate", cause.Error())
		}

		return fmt.Errorf("HTTP request failed (%s %s): %s", urlErr.Op, urlErr.URL, cause)
	}

	return fmt.Errorf("HTTP request failed: %w", err)
}
