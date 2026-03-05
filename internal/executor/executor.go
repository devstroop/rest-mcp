package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

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
}

// New creates an Executor with the given configuration.
func New(baseURL string, headers map[string]string, timeout time.Duration, maxResponseSize int) *Executor {
	return &Executor{
		client: &http.Client{
			Timeout: timeout,
		},
		baseURL:         strings.TrimRight(baseURL, "/"),
		headers:         headers,
		maxResponseSize: maxResponseSize,
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
		return nil, fmt.Errorf("execute request: %w", err)
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
