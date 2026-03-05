package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/devstroop/rest-mcp/internal/config"
	"github.com/devstroop/rest-mcp/internal/executor"
	"github.com/devstroop/rest-mcp/internal/logger"
	"github.com/devstroop/rest-mcp/internal/model"
	"github.com/devstroop/rest-mcp/internal/spec"
	"github.com/devstroop/rest-mcp/internal/tool"
)

var version = "dev"

func main() {
	// Parse CLI flags
	flags := parseFlags()

	if flags.Version {
		fmt.Println("rest-mcp " + version)
		os.Exit(0)
	}

	// Load config (TOML + env + CLI merge)
	cfg, err := config.LoadConfig(flags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	// Configure logger
	logger.SetLevel(logger.ParseLevel(cfg.LogLevel))
	logger.Info("starting rest-mcp", map[string]interface{}{
		"version":   version,
		"base_url":  cfg.BaseURL,
		"transport": cfg.Transport,
	})

	// Parse endpoints into operations
	ops, err := loadOperations(cfg)
	if err != nil {
		logger.Error("failed to parse operations", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}

	if len(ops) == 0 {
		logger.Error("no operations found — provide endpoints in config or an OpenAPI spec")
		os.Exit(1)
	}

	logger.Info("operations loaded", map[string]interface{}{
		"count": len(ops),
	})

	// Generate MCP tools
	entries := tool.Generate(ops)

	// Dry-run mode: print tools and exit
	if cfg.DryRun {
		dryRun(entries)
		return
	}

	// Create HTTP executor
	exec := executor.New(
		cfg.BaseURL,
		cfg.Headers,
		cfg.Timeout(),
		cfg.MaxResponseSize,
		cfg.Auth,
		cfg.Retry,
	)

	// Build MCP server
	mcpServer := server.NewMCPServer(
		"rest-mcp",
		version,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
		server.WithLogging(),
		server.WithResourceCapabilities(false, false),
	)

	// Register tools
	for _, entry := range entries {
		op := entry.Operation // capture for closure
		mcpServer.AddTool(entry.Tool, makeHandler(exec, op))
	}

	// Register spec summary resource
	registerSpecResource(mcpServer, ops)

	// Set up config reload on SIGHUP (M4-05)
	watchReload(func() {
		reloadConfig(flags, mcpServer, exec)
	})

	logger.Info("mcp server ready", map[string]interface{}{
		"tools":     len(entries),
		"transport": cfg.Transport,
	})

	// Start the appropriate transport
	switch cfg.Transport {
	case "sse":
		sseServer := server.NewSSEServer(mcpServer)
		logger.Info("starting SSE transport", map[string]interface{}{
			"addr": cfg.ListenAddr,
		})
		if err := sseServer.Start(cfg.ListenAddr); err != nil {
			logger.Error("SSE server error", map[string]interface{}{"error": err.Error()})
			os.Exit(1)
		}

	case "streamable-http":
		httpServer := server.NewStreamableHTTPServer(mcpServer)
		logger.Info("starting Streamable HTTP transport", map[string]interface{}{
			"addr": cfg.ListenAddr,
		})
		if err := httpServer.Start(cfg.ListenAddr); err != nil {
			logger.Error("Streamable HTTP server error", map[string]interface{}{"error": err.Error()})
			os.Exit(1)
		}

	default: // "stdio"
		if err := server.ServeStdio(mcpServer); err != nil {
			logger.Error("server error", map[string]interface{}{"error": err.Error()})
			os.Exit(1)
		}
	}
}

// parseFlags parses command-line flags.
func parseFlags() config.CLIFlags {
	var flags config.CLIFlags
	flag.StringVar(&flags.ConfigFile, "config", "", "Path to config file (default: rest-mcp.toml)")
	flag.StringVar(&flags.BaseURL, "base-url", "", "Target API base URL")
	flag.StringVar(&flags.Spec, "spec", "", "Path or URL to OpenAPI spec")
	flag.BoolVar(&flags.DryRun, "dry-run", false, "Print generated tools as JSON and exit")
	flag.StringVar(&flags.LogLevel, "log-level", "", "Log level: debug, info, warn, error")
	flag.StringVar(&flags.Transport, "transport", "", "Transport mode: stdio, sse, streamable-http (default: stdio)")
	flag.StringVar(&flags.ListenAddr, "listen-addr", "", "Listen address for SSE/HTTP transport (default: :8080)")
	flag.BoolVar(&flags.Version, "version", false, "Print version and exit")
	flag.Parse()
	return flags
}

// loadOperations loads operations from manual config or OpenAPI spec.
func loadOperations(cfg *config.Config) ([]model.Operation, error) {
	// If manual endpoints defined, use those
	if len(cfg.Endpoints) > 0 {
		logger.Info("using manual endpoint config", map[string]interface{}{
			"count": len(cfg.Endpoints),
		})
		return spec.ParseManualEndpoints(cfg.Endpoints)
	}

	// OpenAPI spec parsing (M1)
	if cfg.OpenAPISpec != "" {
		logger.Info("using OpenAPI spec", map[string]interface{}{
			"spec": cfg.OpenAPISpec,
		})
		return spec.ParseOpenAPISpec(cfg.OpenAPISpec, cfg.Filters)
	}

	return nil, fmt.Errorf("no endpoints configured — add [[endpoints]] to config or set OPENAPI_SPEC")
}

// makeHandler creates an MCP tool handler function for the given operation.
func makeHandler(exec *executor.Executor, op model.Operation) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract arguments
		args := make(map[string]interface{})
		if request.Params.Arguments != nil {
			if m, ok := request.Params.Arguments.(map[string]interface{}); ok {
				args = m
			}
		}

		logger.Debug("tool called", map[string]interface{}{
			"tool": op.Name,
			"args": args,
		})

		// Execute HTTP request
		result, err := exec.Execute(ctx, op, args)
		if err != nil {
			logger.Error("request failed", map[string]interface{}{
				"tool":  op.Name,
				"error": err.Error(),
			})
			return mcp.NewToolResultError(fmt.Sprintf("HTTP request failed: %s", err.Error())), nil
		}

		// Format response
		response := formatResponse(result)

		if result.IsError {
			return mcp.NewToolResultError(response), nil
		}

		return mcp.NewToolResultText(response), nil
	}
}

// formatResponse creates a structured response string from the executor result.
func formatResponse(r *executor.Result) string {
	// Try to pretty-print JSON response
	var prettyJSON interface{}
	if err := json.Unmarshal([]byte(r.Body), &prettyJSON); err == nil {
		formatted, err := json.MarshalIndent(prettyJSON, "", "  ")
		if err == nil {
			return fmt.Sprintf("HTTP %d\n\n%s", r.StatusCode, string(formatted))
		}
	}

	// Fall back to raw body
	return fmt.Sprintf("HTTP %d\n\n%s", r.StatusCode, r.Body)
}

// registerSpecResource registers an MCP resource that exposes a summary of the API spec.
func registerSpecResource(mcpServer *server.MCPServer, ops []model.Operation) {
	if len(ops) == 0 {
		return
	}

	resource := mcp.Resource{
		URI:         "rest-mcp://spec/summary",
		Name:        "API Spec Summary",
		Description: "Summary of all available API operations exposed as MCP tools",
		MIMEType:    "application/json",
	}

	mcpServer.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		type opSummary struct {
			Name        string   `json:"name"`
			Method      string   `json:"method"`
			Path        string   `json:"path"`
			Description string   `json:"description,omitempty"`
			Tags        []string `json:"tags,omitempty"`
			PathParams  []string `json:"path_params,omitempty"`
			QueryParams []string `json:"query_params,omitempty"`
			BodyParams  []string `json:"body_params,omitempty"`
		}

		summaries := make([]opSummary, 0, len(ops))
		for _, op := range ops {
			s := opSummary{
				Name:        op.Name,
				Method:      op.Method,
				Path:        op.Path,
				Description: op.Description,
				Tags:        op.Tags,
			}
			for _, p := range op.PathParams {
				s.PathParams = append(s.PathParams, p.Name)
			}
			for _, p := range op.QueryParams {
				s.QueryParams = append(s.QueryParams, p.Name)
			}
			for _, p := range op.BodyParams {
				s.BodyParams = append(s.BodyParams, p.Name)
			}
			summaries = append(summaries, s)
		}

		data, _ := json.MarshalIndent(map[string]interface{}{
			"total_operations": len(summaries),
			"operations":       summaries,
		}, "", "  ")

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "rest-mcp://spec/summary",
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

// dryRun prints the generated tools as JSON and exits.
func dryRun(entries []tool.ToolEntry) {
	tools := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		t := map[string]interface{}{
			"name":        e.Tool.Name,
			"description": e.Tool.Description,
		}
		// Include the raw input schema if available
		if e.Tool.RawInputSchema != nil {
			var schema interface{}
			if err := json.Unmarshal(e.Tool.RawInputSchema, &schema); err == nil {
				t["inputSchema"] = schema
			}
		}
		tools = append(tools, t)
	}

	output, _ := json.MarshalIndent(tools, "", "  ")
	fmt.Println(string(output))
}

// reloadConfig reloads the config file, re-parses operations, and atomically
// replaces tools and resources on the running MCP server. Called on SIGHUP.
func reloadConfig(flags config.CLIFlags, mcpServer *server.MCPServer, exec *executor.Executor) {
	logger.Info("reloading configuration")

	cfg, err := config.LoadConfig(flags)
	if err != nil {
		logger.Error("reload failed: config error", map[string]interface{}{"error": err.Error()})
		return
	}

	// Update logger level
	logger.SetLevel(logger.ParseLevel(cfg.LogLevel))

	// Re-parse operations
	ops, err := loadOperations(cfg)
	if err != nil {
		logger.Error("reload failed: operation parse error", map[string]interface{}{"error": err.Error()})
		return
	}

	if len(ops) == 0 {
		logger.Error("reload failed: no operations found")
		return
	}

	// Regenerate tools
	entries := tool.Generate(ops)

	// Update executor config
	exec.UpdateConfig(
		cfg.BaseURL,
		cfg.Headers,
		cfg.Timeout(),
		cfg.MaxResponseSize,
		cfg.Auth,
		cfg.Retry,
	)

	// Atomically replace all tools
	serverTools := make([]server.ServerTool, 0, len(entries))
	for _, entry := range entries {
		op := entry.Operation
		serverTools = append(serverTools, server.ServerTool{
			Tool:    entry.Tool,
			Handler: makeHandler(exec, op),
		})
	}
	mcpServer.SetTools(serverTools...)

	// Re-register spec resource
	mcpServer.DeleteResources("rest-mcp://spec/summary")
	registerSpecResource(mcpServer, ops)

	logger.Info("configuration reloaded", map[string]interface{}{
		"tools": len(entries),
		"ops":   len(ops),
	})
}
