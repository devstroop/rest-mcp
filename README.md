<p align="center">
  <h1 align="center">REST MCP</h1>
  <p align="center">Turn any REST API into MCP tools — zero code.</p>
</p>

<p align="center">
  <a href="#quickstart">Quickstart</a> ·
  <a href="#how-it-works">How It Works</a> ·
  <a href="#configuration">Configuration</a> ·
  <a href="#examples">Examples</a> ·
  <a href="PRD.md">PRD</a>
</p>

---

**REST MCP** is a single binary that acts as an [MCP](https://modelcontextprotocol.io) server. Point it at any REST API — with or without an OpenAPI spec — and every endpoint becomes a tool that AI assistants (Claude, Copilot, custom agents) can call directly.

No SDK. No wrapper code. No per-API server.

## Quickstart

### 1. Install

```bash
# Homebrew
brew install devstroop/tap/rest-mcp

# Go
go install github.com/devstroop/rest-mcp@latest

# Or download a binary from Releases
```

### 2. Configure your MCP client

Add to your Claude Desktop / VS Code / agent config:

```json
{
  "mcpServers": {
    "my-api": {
      "command": "rest-mcp",
      "env": {
        "BASE_URL": "https://api.example.com",
        "OPENAPI_SPEC": "./openapi.json"
      },
      "headers": {
        "Authorization": "Bearer your-token-here"
      }
    }
  }
}
```

### 3. Use

Your AI assistant now sees every API endpoint as a callable tool. Ask it:

> "List all users with status active"

The assistant calls `list_users(status="active")`, REST MCP translates that to `GET /users?status=active` with your auth headers, and returns the response.

---

## How It Works

```
AI Assistant                  REST MCP                    Your API
     │                           │                           │
     │──── tools/list ──────────>│                           │
     │<─── [{list_users, ...}] ──│                           │
     │                           │                           │
     │──── tools/call ──────────>│                           │
     │     list_users(page=2)    │─── GET /users?page=2 ────>│
     │                           │   Authorization: Bearer…  │
     │                           │<──── 200 [{...}] ─────────│
     │<─── result: [{...}] ──────│                           │
```

1. **Startup** — REST MCP reads your OpenAPI spec (or TOML config) and builds a list of MCP tools
2. **`tools/list`** — The AI assistant asks what tools are available
3. **`tools/call`** — The assistant invokes a tool with arguments; REST MCP maps them to an HTTP request, executes it, and returns the response

---

## Configuration

REST MCP has three configuration modes, from zero-config to fully manual.

### Mode 1: OpenAPI Spec (Recommended)

If your API has an OpenAPI/Swagger spec, just point to it:

```json
{
  "mcpServers": {
    "my-api": {
      "command": "rest-mcp",
      "env": {
        "BASE_URL": "https://api.example.com",
        "OPENAPI_SPEC": "./openapi.json"
      },
      "headers": {
        "Authorization": "Bearer ${API_KEY}"
      }
    }
  }
}
```

REST MCP auto-generates one tool per operation:

| OpenAPI Operation | Tool Name | Arguments |
|-------------------|-----------|-----------|
| `GET /users` | `list_users` | `page`, `per_page`, `status` |
| `POST /users` | `create_user` | `name`, `email`, `role` |
| `GET /users/{id}` | `get_user` | `id` (required) |
| `DELETE /users/{id}` | `delete_user` | `id` (required) |

Tool names come from `operationId` when available, otherwise generated from `method + path`.

### Mode 2: Manual TOML Config

No OpenAPI spec? Define endpoints manually:

```toml
# rest-mcp.toml
base_url = "https://api.example.com"

[headers]
Authorization = "Bearer ${API_KEY}"

[[endpoints]]
name = "list_users"
method = "GET"
path = "/users"
description = "List all users"

  [endpoints.query]
  page = { type = "integer", default = 1 }
  status = { type = "string", enum = ["active", "inactive"] }

[[endpoints]]
name = "create_user"
method = "POST"
path = "/users"
description = "Create a new user"

  [endpoints.body]
  name = { type = "string", required = true }
  email = { type = "string", required = true }
```

```json
{
  "mcpServers": {
    "my-api": {
      "command": "rest-mcp",
      "env": {
        "CONFIG_FILE": "./rest-mcp.toml",
        "API_KEY": "your-key"
      }
    }
  }
}
```

### Mode 3: Remote Spec URL

Load the spec directly from a URL:

```json
{
  "mcpServers": {
    "petstore": {
      "command": "rest-mcp",
      "env": {
        "BASE_URL": "https://petstore.swagger.io/v2",
        "OPENAPI_SPEC": "https://petstore.swagger.io/v2/swagger.json"
      }
    }
  }
}
```

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `BASE_URL` | Target API base URL (**required**) | — |
| `OPENAPI_SPEC` | Path or URL to OpenAPI/Swagger spec | — |
| `CONFIG_FILE` | Path to TOML config file | `rest-mcp.toml` |
| `LOG_LEVEL` | `debug` · `info` · `warn` · `error` | `warn` |
| `REQUEST_TIMEOUT` | Per-request HTTP timeout | `30s` |
| `MAX_RESPONSE_SIZE` | Truncate responses larger than this (bytes) | `102400` |
| `INCLUDE_TAGS` | Only include operations with these tags (comma-separated) | all |
| `EXCLUDE_PATHS` | Skip operations matching these path prefixes | — |
| `DRY_RUN` | Print generated tools and exit | `false` |

---

## CLI

```
rest-mcp [flags]

Flags:
  --base-url string    Target API base URL
  --spec string        Path or URL to OpenAPI spec
  --config string      Path to TOML config file (default "rest-mcp.toml")
  --dry-run            Print tools as JSON and exit
  --log-level string   Log level (default "warn")
  --version            Print version
  --help               Show help
```

```bash
# Quick check — see what tools would be generated
rest-mcp --spec ./openapi.json --dry-run

# Run with spec
rest-mcp --base-url https://api.example.com --spec ./openapi.json

# Run with manual config
rest-mcp --config ./my-api.toml
```

---

## Examples

### GitHub API

```json
{
  "mcpServers": {
    "github": {
      "command": "rest-mcp",
      "env": {
        "BASE_URL": "https://api.github.com",
        "OPENAPI_SPEC": "https://raw.githubusercontent.com/github/rest-api-description/main/descriptions/api.github.com/api.github.com.json",
        "INCLUDE_TAGS": "repos,issues,pulls"
      },
      "headers": {
        "Authorization": "Bearer ghp_xxxxxxxxxxxx",
        "Accept": "application/vnd.github+json",
        "X-GitHub-Api-Version": "2022-11-28"
      }
    }
  }
}
```

### Stripe API

```json
{
  "mcpServers": {
    "stripe": {
      "command": "rest-mcp",
      "env": {
        "BASE_URL": "https://api.stripe.com/v1",
        "OPENAPI_SPEC": "https://raw.githubusercontent.com/stripe/openapi/master/openapi/spec3.json",
        "INCLUDE_TAGS": "customers,charges,invoices"
      },
      "headers": {
        "Authorization": "Bearer sk_test_xxxxxxxxxxxx"
      }
    }
  }
}
```

### Internal API (No Spec)

```json
{
  "mcpServers": {
    "inventory": {
      "command": "rest-mcp",
      "env": {
        "CONFIG_FILE": "./inventory.toml",
        "INVENTORY_TOKEN": "secret-token"
      }
    }
  }
}
```

---

## Authentication

| Method | Config |
|--------|--------|
| **Bearer token** | `headers: { "Authorization": "Bearer xxx" }` |
| **API key header** | `headers: { "X-API-Key": "xxx" }` |
| **API key in query** | `auth.type = "apikey_query"` in TOML |
| **Basic auth** | `auth.type = "basic"` in TOML |
| **OAuth2 client credentials** | `auth.type = "oauth2_cc"` in TOML (planned) |

Use `${ENV_VAR}` in any header value or config string to reference environment variables:

```json
"headers": {
  "Authorization": "Bearer ${MY_API_TOKEN}"
}
```

---

## Filtering Operations

For large APIs, limit which endpoints become tools:

```json
"env": {
  "INCLUDE_TAGS": "users,billing",
  "EXCLUDE_PATHS": "/internal,/admin"
}
```

Or in TOML:

```toml
[filters]
include_tags = ["users", "billing"]
exclude_paths = ["/internal", "/admin"]
exclude_operations = ["dangerousDelete"]
```

---

## Architecture

```
┌──────────────┐      ┌──────────────┐     ┌──────────────┐
│  Spec Parser │────▶│Tool Generator│────▶│Req. Executor │
│              │      │              │     │              │
│ OpenAPI 3.x  │      │ MCP tools    │     │ HTTP client  │
│ Swagger 2.0  │      │ JSON Schema  │     │ Headers/Auth │
│ Manual TOML  │      │ Validation   │     │ Timeouts     │
└──────────────┘      └──────────────┘     └──────┬───────┘
                                                  │
                          stdio (JSON-RPC)        │ HTTPS
                     ◀────────────────────        ▼
                      AI Assistant          Target REST API
```

See [PRD.md](PRD.md) for the full architecture and requirements.

---

## Development

```bash
# Clone
git clone https://github.com/devstroop/rest-mcp.git
cd rest-mcp

# Build
go build -o rest-mcp ./cmd/rest-mcp

# Test
go test ./...

# Dry-run against petstore
./rest-mcp --base-url https://petstore.swagger.io/v2 \
           --spec https://petstore.swagger.io/v2/swagger.json \
           --dry-run
```

### Project Structure

```
rest-mcp/
├── cmd/rest-mcp/          # CLI entrypoint
│   └── main.go
├── internal/
│   ├── config/            # Config loader (TOML + env + flags)
│   ├── spec/              # OpenAPI / Swagger / TOML parser → []Operation
│   ├── tool/              # Operation → MCP tool definition
│   ├── executor/          # HTTP request builder + executor
│   └── transport/         # MCP stdio JSON-RPC handler
├── rest-mcp.example.toml  # Example manual config
└── README.md
```

---

## Contributing

1. Check [ISSUES.md](ISSUES.md) for available tasks
2. Open an issue or discussion before large changes
3. PRs should include tests for new functionality

---

## License

MIT
