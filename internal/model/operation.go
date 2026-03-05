package model

// Operation represents a single API endpoint that will become an MCP tool.
// It is the internal canonical representation used by both manual TOML config
// and OpenAPI spec parsers.
type Operation struct {
	// Name is the MCP tool name (e.g. "list_users", "create_order").
	Name string

	// Method is the HTTP method (GET, POST, PUT, PATCH, DELETE).
	Method string

	// Path is the URL path with optional path parameters (e.g. "/users/{id}").
	Path string

	// Description is a human-readable description shown to the LLM.
	Description string

	// PathParams are parameters embedded in the URL path.
	PathParams []Param

	// QueryParams are URL query parameters.
	QueryParams []Param

	// BodyParams are request body fields (top-level properties).
	BodyParams []Param

	// Tags from OpenAPI spec (used for filtering).
	Tags []string

	// Headers are per-endpoint header overrides (key → value).
	Headers map[string]string

	// ResponsePath is a dot-notation path to extract from the JSON response
	// (e.g. "data.items"). Empty means return the full response.
	ResponsePath string

	// Hidden indicates the operation should be skipped (from x-rest-mcp-hidden).
	Hidden bool
}

// Param represents a single parameter for a tool argument.
type Param struct {
	// Name is the parameter name.
	Name string

	// Type is the JSON Schema type: "string", "integer", "number", "boolean", "array", "object".
	Type string

	// Description of the parameter for LLM context.
	Description string

	// Required indicates whether the argument is mandatory.
	Required bool

	// Default value (as string representation).
	Default interface{}

	// Enum lists allowed values.
	Enum []string

	// ItemsType is the JSON Schema type of array elements (only used when Type == "array").
	// Defaults to "string" if empty.
	ItemsType string
}
