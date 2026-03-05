package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/devstroop/rest-mcp/internal/logger"
)

// Config holds the fully resolved application configuration.
type Config struct {
	BaseURL         string            `toml:"base_url"`
	OpenAPISpec     string            `toml:"openapi_spec"`
	LogLevel        string            `toml:"log_level"`
	RequestTimeout  string            `toml:"request_timeout"`
	MaxResponseSize int               `toml:"max_response_size"`
	DryRun          bool              `toml:"dry_run"`
	Transport       string            `toml:"transport"`    // stdio, sse, streamable-http
	ListenAddr      string            `toml:"listen_addr"` // address for SSE/HTTP transports
	Headers         map[string]string `toml:"headers"`
	Filters         Filters           `toml:"filters"`
	Auth            Auth              `toml:"auth"`
	Retry           Retry             `toml:"retry"`
	Endpoints       []Endpoint        `toml:"endpoints"`
}

// Retry configures automatic request retry with exponential backoff.
type Retry struct {
	MaxAttempts int    `toml:"max_attempts"` // 0 = disabled (default)
	InitialWait string `toml:"initial_wait"` // e.g. "500ms" (default)
	MaxWait     string `toml:"max_wait"`     // e.g. "30s" (default)
}

// Filters configures which operations to include/exclude.
type Filters struct {
	IncludeTags       []string `toml:"include_tags"`
	ExcludePaths      []string `toml:"exclude_paths"`
	IncludeOperations []string `toml:"include_operations"`
	ExcludeOperations []string `toml:"exclude_operations"`
}

// Auth configures API authentication.
type Auth struct {
	Type         string   `toml:"type"` // bearer, apikey_header, apikey_query, basic, oauth2_cc
	Key          string   `toml:"key"`
	Value        string   `toml:"value"`
	TokenURL     string   `toml:"token_url"`
	ClientID     string   `toml:"client_id"`
	ClientSecret string   `toml:"client_secret"`
	Scopes       []string `toml:"scopes"`
}

// Endpoint defines a manual API endpoint.
type Endpoint struct {
	Name         string                     `toml:"name"`
	Method       string                     `toml:"method"`
	Path         string                     `toml:"path"`
	Description  string                     `toml:"description"`
	PathParams   map[string]ParamDef        `toml:"path_params"`
	Query        map[string]ParamDef        `toml:"query"`
	Body         map[string]ParamDef        `toml:"body"`
	Headers      map[string]string          `toml:"headers"`
	ResponsePath string                     `toml:"response_path"`
}

// ParamDef defines a parameter in TOML config.
type ParamDef struct {
	Type        string      `toml:"type"`
	Description string      `toml:"description"`
	Required    bool        `toml:"required"`
	Default     interface{} `toml:"default"`
	Enum        []string    `toml:"enum"`
}

// CLIFlags holds command-line flag values.
type CLIFlags struct {
	ConfigFile string
	BaseURL    string
	Spec       string
	DryRun     bool
	LogLevel   string
	Transport  string
	ListenAddr string
	Version    bool
}

// LoadConfig loads configuration by merging:
//   1. Defaults
//   2. TOML config file
//   3. Environment variables
//   4. CLI flags (highest priority)
func LoadConfig(flags CLIFlags) (*Config, error) {
	cfg := defaults()

	// Determine config file path
	configFile := flags.ConfigFile
	if configFile == "" {
		configFile = os.Getenv("CONFIG_FILE")
	}
	if configFile == "" {
		configFile = "rest-mcp.toml"
	}

	// Load TOML config file (optional — not an error if missing)
	if err := loadTOML(cfg, configFile); err != nil {
		logger.Debug("config file not loaded", map[string]interface{}{
			"path":  configFile,
			"error": err.Error(),
		})
	}

	// Override with environment variables
	applyEnv(cfg)

	// Override with CLI flags (highest priority)
	applyCLI(cfg, flags)

	// Interpolate ${ENV_VAR} in all string fields
	interpolateConfig(cfg)

	// Validate
	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Timeout returns the parsed request timeout duration.
func (c *Config) Timeout() time.Duration {
	d, err := time.ParseDuration(c.RequestTimeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// defaults returns a Config with sensible default values.
func defaults() *Config {
	return &Config{
		LogLevel:        "warn",
		RequestTimeout:  "30s",
		MaxResponseSize: 102400,
		Transport:       "stdio",
		ListenAddr:      ":8080",
		Headers:         make(map[string]string),
	}
}

// loadTOML reads a TOML config file into cfg.
func loadTOML(cfg *Config, path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", path)
	}
	_, err := toml.DecodeFile(path, cfg)
	return err
}

// applyEnv overrides config with environment variables.
func applyEnv(cfg *Config) {
	if v := os.Getenv("BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("OPENAPI_SPEC"); v != "" {
		cfg.OpenAPISpec = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("REQUEST_TIMEOUT"); v != "" {
		cfg.RequestTimeout = v
	}
	if v := os.Getenv("MAX_RESPONSE_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxResponseSize = n
		}
	}
	if v := os.Getenv("DRY_RUN"); v != "" {
		cfg.DryRun = v == "true" || v == "1"
	}
	if v := os.Getenv("TRANSPORT"); v != "" {
		cfg.Transport = v
	}
	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("INCLUDE_TAGS"); v != "" {
		cfg.Filters.IncludeTags = splitCSV(v)
	}
	if v := os.Getenv("EXCLUDE_PATHS"); v != "" {
		cfg.Filters.ExcludePaths = splitCSV(v)
	}

	// HEADER_* env vars → injected as HTTP headers.
	// e.g. HEADER_Authorization="Bearer xxx" → Authorization: Bearer xxx
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "HEADER_") {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) == 2 {
				headerName := strings.TrimPrefix(parts[0], "HEADER_")
				if headerName != "" {
					cfg.Headers[headerName] = parts[1]
				}
			}
		}
	}
}

// applyCLI overrides config with CLI flag values.
func applyCLI(cfg *Config, flags CLIFlags) {
	if flags.BaseURL != "" {
		cfg.BaseURL = flags.BaseURL
	}
	if flags.Spec != "" {
		cfg.OpenAPISpec = flags.Spec
	}
	if flags.LogLevel != "" {
		cfg.LogLevel = flags.LogLevel
	}
	if flags.DryRun {
		cfg.DryRun = true
	}
	if flags.Transport != "" {
		cfg.Transport = flags.Transport
	}
	if flags.ListenAddr != "" {
		cfg.ListenAddr = flags.ListenAddr
	}
}

var envVarRegex = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Interpolate replaces ${ENV_VAR} patterns in a string with env values.
func Interpolate(s string) string {
	return envVarRegex.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1] // strip ${ and }
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		return match // leave unresolved if env var not set
	})
}

// interpolateConfig applies ${ENV_VAR} interpolation to all relevant fields.
func interpolateConfig(cfg *Config) {
	cfg.BaseURL = Interpolate(cfg.BaseURL)
	cfg.OpenAPISpec = Interpolate(cfg.OpenAPISpec)

	for k, v := range cfg.Headers {
		cfg.Headers[k] = Interpolate(v)
	}

	cfg.Auth.Key = Interpolate(cfg.Auth.Key)
	cfg.Auth.Value = Interpolate(cfg.Auth.Value)
	cfg.Auth.TokenURL = Interpolate(cfg.Auth.TokenURL)
	cfg.Auth.ClientID = Interpolate(cfg.Auth.ClientID)
	cfg.Auth.ClientSecret = Interpolate(cfg.Auth.ClientSecret)
}

// validate checks for required config values.
func validate(cfg *Config) error {
	if cfg.BaseURL == "" {
		return fmt.Errorf("BASE_URL is required (set via env, config file, or --base-url flag)")
	}
	// Ensure base URL doesn't end with /
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return nil
}

// splitCSV splits a comma-separated string into trimmed, non-empty parts.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
