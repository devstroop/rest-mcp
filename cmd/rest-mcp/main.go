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
		"version":  version,
		"base_url": cfg.BaseURL,
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
	)

	// Build MCP server
	mcpServer := server.NewMCPServer(
		"rest-mcp",
		version,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
		server.WithLogging(),
	)

	// Register tools
	for _, entry := range entries {
		op := entry.Operation // capture for closure
		mcpServer.AddTool(entry.Tool, makeHandler(exec, op))
	}

	logger.Info("mcp server ready", map[string]interface{}{
		"tools": len(entries),
	})

	// Start stdio transport
	if err := server.ServeStdio(mcpServer); err != nil {
		logger.Error("server error", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
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
