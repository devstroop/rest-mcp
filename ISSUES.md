# REST MCP — Issues & Backlog

Tracking sheet for implementation tasks, bugs, and design decisions.
Derived from [PRD.md](PRD.md).

---

## Legend

| Label | Meaning |
|-------|---------|
| `M0` – `M4` | Milestone |
| `P0` | Must-have for milestone |
| `P1` | Should-have |
| `P2` | Nice-to-have / future |
| `blocked` | Waiting on dependency or decision |
| `design` | Needs design/RFC before implementation |

---

## M0 — Skeleton

> Goal: working MCP server that reads a TOML config and calls a REST API.

- [ ] **M0-01** · Project scaffolding (`go mod init`, directory layout, Makefile, `.goreleaser.yml`) `P0`
- [ ] **M0-02** · Config loader — read TOML file, env vars, CLI flags; priority merge `P0`
- [ ] **M0-03** · `${ENV_VAR}` interpolation in config string values `P0`
- [ ] **M0-04** · Manual endpoint parser — `[[endpoints]]` → `[]Operation` internal model `P0`
- [ ] **M0-05** · Tool generator — `[]Operation` → MCP `tools/list` response with JSON Schema `P0`
- [ ] **M0-06** · Request executor — build HTTP request from tool call args (path, query, body, headers) `P0`
- [ ] **M0-07** · Static header injection from `[headers]` config `P0`
- [ ] **M0-08** · MCP stdio transport — JSON-RPC over stdin/stdout (`initialize`, `tools/list`, `tools/call`) `P0`
- [ ] **M0-09** · Response formatting — return JSON body + status code; `isError` for 4xx/5xx `P0`
- [ ] **M0-10** · Structured JSON logging to stderr with `LOG_LEVEL` control `P0`
- [ ] **M0-11** · Basic end-to-end test: TOML config → tool list → tool call → HTTP mock `P0`

---

## M1 — OpenAPI

> Goal: point at an OpenAPI spec, auto-generate all tools.

- [ ] **M1-01** · OpenAPI 3.0 parser — load JSON/YAML from file path `P0`
- [ ] **M1-02** · OpenAPI 3.1 parser (handle differences from 3.0) `P0`
- [ ] **M1-03** · Remote spec loading — fetch spec from URL `P0`
- [ ] **M1-04** · Operation extraction — iterate paths × methods → `[]Operation` `P0`
- [ ] **M1-05** · Tool naming — `operationId` → tool name; fallback `method_path` snake_case `P0`
- [ ] **M1-06** · Path parameter mapping → required tool arguments `P0`
- [ ] **M1-07** · Query parameter mapping → optional tool arguments with defaults `P0`
- [ ] **M1-08** · Request body schema → tool arguments (flatten top-level properties) `P0`
- [ ] **M1-09** · `enum` constraints on arguments `P0`
- [ ] **M1-10** · `description` / `summary` → tool description `P0`
- [ ] **M1-11** · `--dry-run` mode: print generated tools as JSON and exit `P1`
- [ ] **M1-12** · Handle `$ref` resolution (inline + external file refs) `P0`
- [ ] **M1-13** · Integration test: Petstore spec → tool list → call mock `P0`

---

## M2 — Production Hardening

> Goal: reliable in real-world usage with messy specs and large APIs.

- [ ] **M2-01** · Include/exclude filters: `include_tags`, `exclude_paths`, `include_operations`, `exclude_operations` `P1`
- [ ] **M2-02** · Response truncation — configurable `MAX_RESPONSE_SIZE` (default 100KB) `P0`
- [ ] **M2-03** · Request timeout — configurable `REQUEST_TIMEOUT` (default 30s) `P0`
- [ ] **M2-04** · Graceful error handling — network errors, DNS failures, TLS errors → clear MCP error `P0`
- [ ] **M2-05** · Raw JSON body pass-through for complex/nested schemas `P1`
- [ ] **M2-06** · Debug logging — log every outbound HTTP request (method, path, status, latency) `P0`
- [ ] **M2-07** · Handle specs with 500+ operations efficiently (lazy generation, memory) `P1`
- [ ] **M2-08** · Validate BASE_URL format at startup `P0`
- [ ] **M2-09** · Handle spec with no `operationId` on any operation (generate all names) `P0`
- [ ] **M2-10** · Tool name collision detection and deterministic disambiguation `P1`

---

## M3 — Extended Auth & Distribution

> Goal: support real auth patterns and ship cross-platform binaries.

- [ ] **M3-01** · API key in query parameter (`auth.type = "apikey_query"`) `P1`
- [ ] **M3-02** · Basic auth (`auth.type = "basic"`) `P1`
- [ ] **M3-03** · OAuth2 client-credentials flow with auto-refresh + token caching `P2`
- [ ] **M3-04** · Swagger 2.0 spec parser `P1`
- [ ] **M3-05** · goreleaser config — Linux amd64/arm64, macOS amd64/arm64, Windows amd64 `P0`
- [ ] **M3-06** · GitHub Actions CI — build + test + release on tag `P0`
- [ ] **M3-07** · Homebrew tap formula `P1`
- [ ] **M3-08** · npm wrapper package (`npx rest-mcp`) `P1`
- [ ] **M3-09** · Docker image (Alpine-based) `P2`
- [ ] **M3-10** · Install script (`curl | sh`) `P1`

---

## M4 — Advanced

> Post-launch improvements driven by community feedback.

- [ ] **M4-01** · SSE transport (MCP over HTTP SSE) `P2`
- [ ] **M4-02** · Streamable HTTP transport `P2`
- [ ] **M4-03** · File upload support (`multipart/form-data`) `P1`
- [ ] **M4-04** · JSON-path response extraction (`response_path = "$.data.items"`) `P2`
- [ ] **M4-05** · Config reload on `SIGHUP` without restart `P2`
- [ ] **M4-06** · OpenAPI extensions: `x-rest-mcp-name`, `x-rest-mcp-hidden` `P2`
- [ ] **M4-07** · Per-endpoint header overrides `P2`
- [ ] **M4-08** · Request retry with exponential backoff (configurable) `P2`
- [ ] **M4-09** · Response caching (ETag / Last-Modified aware) `P2`
- [ ] **M4-10** · MCP resources — expose spec summary as a resource `P2`

---

## Design Decisions (needs RFC)

- [ ] **D-01** · Tool naming collision strategy — append version prefix? numeric suffix? `design`
- [ ] **D-02** · Nested request body handling — flatten depth limit? always raw JSON for depth > 1? `design`
- [ ] **D-03** · Pagination — expose params only, or optional auto-paginate for common patterns? `design`
- [ ] **D-04** · Binary name — `rest-mcp` vs `restmcp` vs `mcp-rest` `design`
- [ ] **D-05** · `headers` in MCP client config — how to pass to the binary (env? CLI? stdin handshake?) `design`

---

## Bugs

_None yet — project hasn't started._
