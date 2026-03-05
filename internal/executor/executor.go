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

	// OAuth2 token cache
	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

// New creates an Executor with the given configuration.
func New(baseURL string, headers map[string]string, timeout time.Duration, maxResponseSize int, auth config.Auth) *Executor {
	return &Executor{
		client: &http.Client{
			Timeout: timeout,
		},
		baseURL:         strings.TrimRight(baseURL, "/"),
		headers:         headers,
		maxResponseSize: maxResponseSize,
		auth:            auth,
	}
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

	// Inject auth credentials
	if err := e.applyAuth(ctx, req); err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	// Set Content-Type for requests with body
	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Log outbound request
	start := time.Now()
	logger.Debug("outbound request", map[string]interface{}{
		"method": op.Method,
		"url":    fullURL,
	})

	// Execute
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, classifyError(err)
	}
	defer resp.Body.Close()

	latency := time.Since(start)

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

	return &Result{
		StatusCode: resp.StatusCode,
		Body:       bodyStr,
		IsError:    isError,
	}, nil
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
