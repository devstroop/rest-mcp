# REST MCP — Product Requirements Document

**Version:** 0.1.0-draft
**Date:** 2026-03-05
**Author:** devstroop

---

## 1. Problem Statement

Every REST API today requires a bespoke MCP server to be usable by AI assistants. Teams spend days writing Go/Rust/Python wrappers that do nothing more than translate HTTP calls into MCP tool definitions. This is repetitive, error-prone, and doesn't scale — a company with 20 internal APIs needs 20 MCP servers.

**REST MCP** eliminates this entirely. Point it at any REST API (with or without an OpenAPI spec), and it instantly becomes a set of MCP tools.

---

## 2. Product Overview

REST MCP is a single binary that acts as an MCP server (stdio transport). It reads an OpenAPI/Swagger specification (or a minimal manual config) and dynamically exposes every endpoint as an MCP tool that AI assistants can invoke.

### One-liner

> **Turn any REST API into MCP tools — zero code.**

---

## 3. Target Users

| Persona | Need |
|---------|------|
| **AI-first developers** | Connect internal/external APIs to Claude, Copilot, or custom agents without writing MCP servers |
| **Platform teams** | Expose existing microservices to AI assistants org-wide with a single config change |
| **API providers** | Ship an MCP integration alongside their REST API by publishing a config snippet |
| **Solo hackers** | Wire up a SaaS API (Stripe, GitHub, Notion) in 30 seconds |

---

## 4. Usage

### 4.1 MCP Client Configuration

```json
{
  "mcpServers": {
    "rest-mcp": {
      "command": "rest-mcp",
      "env": {
        "BASE_URL": "https://api.example.com",
        "OPENAPI_SPEC": "./openapi.json",
        "LOG_LEVEL": "warn"
      },
      "headers": {
        "Authorization": "Bearer xxxxxxxx",
        "X-API-Version": "2.0",
        "Custom-Client": "my-client",
        "Accept": "application/json"
      }
    }
  }
}
```

### 4.2 Minimal Config (No OpenAPI)

When no OpenAPI spec is available, endpoints can be defined manually:

```toml
# rest-mcp.toml

base_url = "https://api.example.com"

[headers]
Authorization = "Bearer ${API_KEY}"
Accept = "application/json"

[[endpoints]]
name = "list_users"
method = "GET"
path = "/users"
description = "List all users with optional filtering"

  [endpoints.query]
  page = { type = "integer", description = "Page number", default = 1 }
  per_page = { type = "integer", description = "Items per page", default = 20 }
  status = { type = "string", description = "Filter by status", enum = ["active", "inactive"] }

[[endpoints]]
name = "get_user"
method = "GET"
path = "/users/{id}"
description = "Get a user by ID"

  [endpoints.path]
  id = { type = "string", description = "User ID", required = true }

[[endpoints]]
name = "create_user"
method = "POST"
path = "/users"
description = "Create a new user"

  [endpoints.body]
  name = { type = "string", description = "Full name", required = true }
  email = { type = "string", description = "Email address", required = true }
  role = { type = "string", description = "User role", enum = ["admin", "member", "viewer"], default = "member" }
```

### 4.3 OpenAPI Auto-Discovery

```json
{
  "mcpServers": {
    "petstore": {
      "command": "rest-mcp",
      "env": {
        "BASE_URL": "https://petstore.swagger.io/v2",
        "OPENAPI_SPEC": "https://petstore.swagger.io/v2/swagger.json"
      },
      "headers": {
        "Authorization": "Bearer xxxxxxxx"
      }
    }
  }
}
```

REST MCP parses the spec and auto-generates one MCP tool per operation:

| OpenAPI Operation | Generated Tool Name | Description |
|-------------------|---------------------|-------------|
| `GET /pets` | `list_pets` | Uses `operationId` or generates from method+path |
| `POST /pets` | `create_pet` | |
| `GET /pets/{petId}` | `get_pet` | Path params become required tool arguments |
| `DELETE /pets/{petId}` | `delete_pet` | |

---

## 5. Functional Requirements

### 5.1 Core — Spec Ingestion

| ID | Requirement | Priority |
|----|-------------|----------|
| F-01 | Parse OpenAPI 3.0 and 3.1 specifications (JSON and YAML) | P0 |
| F-02 | Parse Swagger 2.0 specifications | P1 |
| F-03 | Load spec from local file path or remote URL | P0 |
| F-04 | Fall back to manual TOML/JSON endpoint config when no spec | P0 |
| F-05 | Re-read spec on `SIGHUP` / config reload without restart | P2 |

### 5.2 Core — Tool Generation

| ID | Requirement | Priority |
|----|-------------|----------|
| F-10 | Generate one MCP tool per OpenAPI operation | P0 |
| F-11 | Use `operationId` as tool name; fall back to `method_path` snake_case | P0 |
| F-12 | Use `summary` or `description` as tool description | P0 |
| F-13 | Map path parameters to required tool arguments | P0 |
| F-14 | Map query parameters to optional tool arguments with defaults | P0 |
| F-15 | Map request body schema to tool arguments (flattened for simple objects) | P0 |
| F-16 | Support `enum` constraints on arguments | P0 |
| F-17 | Allow include/exclude filters on operations via config (`include_tags`, `exclude_paths`, `include_operations`) | P1 |
| F-18 | Support `x-rest-mcp-name` and `x-rest-mcp-hidden` OpenAPI extensions | P2 |

### 5.3 Core — Request Execution

| ID | Requirement | Priority |
|----|-------------|----------|
| F-20 | Construct HTTP request from tool arguments (path, query, body, headers) | P0 |
| F-21 | Inject static headers from config / env | P0 |
| F-22 | Support `${ENV_VAR}` interpolation in headers and base URL | P0 |
| F-23 | Send `Content-Type: application/json` for JSON bodies | P0 |
| F-24 | Support `multipart/form-data` for file upload endpoints | P1 |
| F-25 | Configurable request timeout (default 30s) | P0 |
| F-26 | Support request body pass-through (raw JSON argument) for complex schemas | P1 |

### 5.4 Core — Response Handling

| ID | Requirement | Priority |
|----|-------------|----------|
| F-30 | Return JSON response body as tool result text | P0 |
| F-31 | Return HTTP status code in tool result metadata | P0 |
| F-32 | Return error responses (4xx/5xx) as `isError: true` with status + body | P0 |
| F-33 | Truncate oversized responses (configurable max, default 100KB) | P0 |
| F-34 | Support JSON-path extraction to return only a sub-field (`response_path = "$.data.items"`) | P2 |

### 5.5 Authentication

| ID | Requirement | Priority |
|----|-------------|----------|
| F-40 | Static headers (Bearer token, API key) via `headers` config | P0 |
| F-41 | `${ENV_VAR}` interpolation in header values | P0 |
| F-42 | API key in query parameter (`auth.type = "apikey_query"`, `auth.key = "api_key"`) | P1 |
| F-43 | OAuth2 client-credentials flow with auto-refresh | P2 |

### 5.6 Transport

| ID | Requirement | Priority |
|----|-------------|----------|
| F-50 | MCP stdio transport (stdin/stdout JSON-RPC) | P0 |
| F-51 | MCP SSE transport (HTTP server mode) | P2 |
| F-52 | Streamable HTTP transport | P2 |

### 5.7 Observability

| ID | Requirement | Priority |
|----|-------------|----------|
| F-60 | Structured JSON logging to stderr | P0 |
| F-61 | Configurable log level via `LOG_LEVEL` env | P0 |
| F-62 | Log every outbound HTTP request (method, path, status, latency) at debug level | P0 |
| F-63 | Dry-run mode: log generated tools without starting server | P1 |

---

## 6. Non-Functional Requirements

| ID | Requirement | Target |
|----|-------------|--------|
| N-01 | Single static binary, no runtime dependencies | Yes |
| N-02 | Binary size | < 15 MB |
| N-03 | Cold start (parse spec + serve first tool list) | < 500ms |
| N-04 | Memory at idle | < 20 MB |
| N-05 | Cross-platform | Linux amd64/arm64, macOS amd64/arm64, Windows amd64 |
| N-06 | Zero config for OpenAPI-ready APIs | `BASE_URL` + `OPENAPI_SPEC` + headers only |

---

## 7. Architecture

```
┌─────────────────────────────────────────────────┐
│                  AI Assistant                    │
│            (Claude, Copilot, etc.)               │
└──────────────────┬──────────────────────────────┘
                   │ stdio (JSON-RPC)
┌──────────────────▼──────────────────────────────┐
│               REST MCP Server                    │
│                                                  │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐ │
│  │   Spec     │  │   Tool     │  │  Request   │ │
│  │  Parser    │──│ Generator  │──│  Executor  │ │
│  │            │  │            │  │            │ │
│  │ OpenAPI 3  │  │ MCP tools  │  │ HTTP client│ │
│  │ Swagger 2  │  │ arguments  │  │ headers    │ │
│  │ Manual cfg │  │ validation │  │ auth       │ │
│  └────────────┘  └────────────┘  └─────┬──────┘ │
└────────────────────────────────────────┬────────┘
                                         │ HTTPS
                              ┌──────────▼─────────┐
                              │   Target REST API   │
                              │  api.example.com    │
                              └────────────────────┘
```

### 7.1 Components

| Component | Responsibility |
|-----------|----------------|
| **Spec Parser** | Load and normalize OpenAPI 3.x / Swagger 2.0 / manual TOML into a unified internal representation |
| **Tool Generator** | Convert parsed operations into MCP tool definitions with JSON Schema input schemas |
| **Request Executor** | Build HTTP requests from tool call arguments, inject headers/auth, execute, and format responses |
| **MCP Transport** | Handle stdio JSON-RPC protocol (tools/list, tools/call) |
| **Config Loader** | Merge env vars, CLI flags, and config file; resolve `${VAR}` interpolation |

### 7.2 Internal Data Flow

```
1. Startup
   env/config → Config Loader → Spec Parser → []Operation

2. tools/list
   []Operation → Tool Generator → []ToolDefinition → JSON-RPC response

3. tools/call
   tool name + args → find Operation → Request Executor → HTTP call → format → JSON-RPC response
```

---

## 8. Configuration Reference

All configuration is resolved in this priority order (highest wins):

```
CLI flags > Environment variables > Config file > Defaults
```

### 8.1 Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `BASE_URL` | Target API base URL (required) | — |
| `OPENAPI_SPEC` | Path or URL to OpenAPI spec | — |
| `CONFIG_FILE` | Path to TOML config file | `rest-mcp.toml` |
| `LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `warn` |
| `REQUEST_TIMEOUT` | HTTP request timeout | `30s` |
| `MAX_RESPONSE_SIZE` | Max response body bytes | `102400` |
| `INCLUDE_TAGS` | Comma-separated tags to include | — (all) |
| `EXCLUDE_PATHS` | Comma-separated path prefixes to skip | — |
| `DRY_RUN` | Print generated tools and exit | `false` |

### 8.2 Full Config File

```toml
base_url = "https://api.example.com"
openapi_spec = "./openapi.json"     # or URL
log_level = "warn"
request_timeout = "30s"
max_response_size = 102400

[headers]
Authorization = "Bearer ${API_KEY}"
Accept = "application/json"
X-Custom = "value"

[filters]
include_tags = ["users", "billing"]
exclude_paths = ["/internal", "/admin"]
include_operations = []     # empty = all
exclude_operations = ["deleteEverything"]

[auth]
type = "bearer"             # bearer | apikey_header | apikey_query | basic | oauth2_cc
# For apikey_query:
# key = "api_key"
# value = "${API_KEY}"
# For oauth2_cc:
# token_url = "https://auth.example.com/token"
# client_id = "${CLIENT_ID}"
# client_secret = "${CLIENT_SECRET}"
# scopes = ["read", "write"]

# Manual endpoints (used when no openapi_spec)
[[endpoints]]
name = "list_users"
method = "GET"
path = "/users"
description = "List all users"

  [endpoints.query]
  page = { type = "integer", description = "Page number", default = 1 }
  per_page = { type = "integer", description = "Items per page", default = 20 }

[[endpoints]]
name = "create_user"
method = "POST"
path = "/users"
description = "Create a new user"

  [endpoints.body]
  name = { type = "string", required = true }
  email = { type = "string", required = true }
```

---

## 9. CLI Interface

```
rest-mcp [flags]

Flags:
  --config          Path to config file (default: rest-mcp.toml)
  --base-url        Target API base URL (overrides config/env)
  --spec            Path or URL to OpenAPI spec
  --dry-run         Print generated tools as JSON and exit
  --log-level       Log level: debug, info, warn, error
  --version         Print version and exit
  --help            Show help
```

### 9.1 Example Commands

```bash
# Auto-discover from OpenAPI spec
rest-mcp --base-url https://api.example.com --spec ./openapi.json

# Manual config
rest-mcp --config ./my-api.toml

# Dry-run: see what tools would be generated
rest-mcp --spec ./openapi.json --dry-run

# Everything via env (typical MCP client usage)
BASE_URL=https://api.example.com OPENAPI_SPEC=./spec.json rest-mcp
```

---

## 10. Real-World Examples

### 10.1 GitHub API

```json
{
  "mcpServers": {
    "github": {
      "command": "rest-mcp",
      "env": {
        "BASE_URL": "https://api.github.com",
        "OPENAPI_SPEC": "https://raw.githubusercontent.com/github/rest-api-description/main/descriptions/api.github.com/api.github.com.json",
        "INCLUDE_TAGS": "repos,issues,pulls",
        "LOG_LEVEL": "warn"
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

### 10.2 Stripe API

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

### 10.3 Internal API (No OpenAPI)

```json
{
  "mcpServers": {
    "inventory": {
      "command": "rest-mcp",
      "env": {
        "BASE_URL": "https://internal.company.com/api",
        "CONFIG_FILE": "/etc/rest-mcp/inventory.toml"
      },
      "headers": {
        "Authorization": "Bearer ${INVENTORY_TOKEN}"
      }
    }
  }
}
```

---

## 11. Language Decision

| Criteria | Go | Rust |
|----------|-----|------|
| **MCP SDK maturity** | mcp-go (stable, widely used) | mcp-rust (newer, growing) |
| **OpenAPI parsing** | kin-openapi (battle-tested) | openapiv3 (solid) |
| **Binary size** | ~10-12 MB | ~5-8 MB |
| **Build speed** | Fast (2-5s) | Slower (30-60s) |
| **Cross-compile** | Trivial (`GOOS/GOARCH`) | Requires cross toolchains |
| **Error handling** | Simpler, convention-based | Stricter, `Result<T,E>` |
| **Ecosystem for HTTP clients** | net/http (stdlib) | reqwest (de facto) |
| **Distribution** | Single binary | Single binary |

**Recommendation:** **Go** — faster iteration, trivial cross-compilation (6 targets in one `goreleaser` config), and `mcp-go` + `kin-openapi` are both production-grade. Rust is viable but adds build complexity with no meaningful upside for a CLI tool that spends most of its time waiting on HTTP I/O.

---

## 12. Milestones

### M0 — Skeleton (Week 1)
- [ ] Project scaffolding, CI, goreleaser
- [ ] MCP stdio transport (tools/list, tools/call)
- [ ] Manual TOML endpoint config → MCP tools
- [ ] Basic HTTP execution with static headers

### M1 — OpenAPI (Week 2)
- [ ] OpenAPI 3.0/3.1 parser via kin-openapi
- [ ] Auto-generate tools from spec
- [ ] Path + query + body argument mapping
- [ ] Enum and default value support
- [ ] Dry-run mode

### M2 — Production Hardening (Week 3)
- [ ] `${ENV_VAR}` interpolation
- [ ] Include/exclude filters (tags, paths, operations)
- [ ] Response truncation
- [ ] Structured logging to stderr
- [ ] Error handling (timeouts, network errors, 4xx/5xx)

### M3 — Extended Auth + Distribution (Week 4)
- [ ] API key in query param
- [ ] OAuth2 client-credentials with token caching
- [ ] Swagger 2.0 support
- [ ] goreleaser: Linux/macOS/Windows binaries
- [ ] Homebrew tap, npm wrapper package
- [ ] README + docs site

### M4 — Advanced (Post-launch)
- [ ] SSE transport
- [ ] Streamable HTTP transport
- [ ] File upload (multipart)
- [ ] JSON-path response extraction
- [ ] Config reload on SIGHUP
- [ ] OpenAPI extensions (`x-rest-mcp-*`)

---

## 13. Success Metrics

| Metric | Target (3 months) |
|--------|-------------------|
| GitHub stars | 500+ |
| APIs confirmed working | 20+ (community-reported) |
| Binary downloads | 2,000+ |
| Time to first tool call (new API) | < 2 minutes |
| Issues with "bug" label | < 10 open |

---

## 14. Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Large OpenAPI specs (10K+ operations) cause slow startup | High | Lazy tool generation; include/exclude filters |
| Complex nested request bodies don't map cleanly to flat tool args | Medium | Support raw JSON pass-through arg; flatten only top-level |
| Auth token expiry mid-session | Medium | OAuth2 auto-refresh; clear error message for static tokens |
| API responses too large for MCP context | High | Configurable truncation; JSON-path extraction |
| Breaking changes in MCP protocol | Low | Pin to stable MCP spec version; abstract transport layer |

---

## 15. Out of Scope (v1)

- GraphQL APIs
- gRPC / Protobuf APIs
- WebSocket / streaming endpoints
- Request batching / chaining
- Response caching
- Rate limit handling (retry-after)
- Per-tool header overrides
- Custom response transformers (Lua/JS scripting)

---

## 16. Open Questions

1. **Tool naming collisions** — If two operations produce the same snake_case name (e.g., `GET /users` and `GET /v2/users` both → `list_users`), how should we disambiguate? Proposal: append version prefix (`v2_list_users`).

2. **Pagination** — Should REST MCP auto-paginate (follow `next` links) or expose pagination params as tool arguments? Proposal: expose params; auto-pagination is too API-specific.

3. **Binary name** — `rest-mcp`? `restmcp`? `mcp-rest`? Needs to be short, memorable, and `npm`/`brew`-friendly.

4. **Monorepo or separate repo** — Keep under `devstroop/rest-mcp` or nest under a larger MCP tools org?
