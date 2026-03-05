package tool

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/devstroop/rest-mcp/internal/model"
)

// GenerateTools converts a slice of Operations into MCP Tool definitions
// paired with their operation metadata (used by the tool handler).
type ToolEntry struct {
	Tool      mcp.Tool
	Operation model.Operation
}

// Generate creates MCP tools from operations.
func Generate(ops []model.Operation) []ToolEntry {
	entries := make([]ToolEntry, 0, len(ops))

	for _, op := range ops {
		tool := buildTool(op)
		entries = append(entries, ToolEntry{
			Tool:      tool,
			Operation: op,
		})
	}

	return entries
}

// buildTool creates an MCP Tool from an Operation using raw JSON Schema.
func buildTool(op model.Operation) mcp.Tool {
	schema := buildInputSchema(op)
	schemaJSON, _ := json.Marshal(schema)

	desc := op.Description
	if desc == "" {
		desc = op.Method + " " + op.Path
	}

	return mcp.NewToolWithRawSchema(op.Name, desc, schemaJSON)
}

// buildInputSchema creates a JSON Schema object for the tool's input arguments.
func buildInputSchema(op model.Operation) map[string]interface{} {
	properties := make(map[string]interface{})
	required := make([]string, 0)

	// Path parameters
	for _, p := range op.PathParams {
		properties[p.Name] = paramToSchema(p)
		if p.Required {
			required = append(required, p.Name)
		}
	}

	// Query parameters
	for _, p := range op.QueryParams {
		properties[p.Name] = paramToSchema(p)
		if p.Required {
			required = append(required, p.Name)
		}
	}

	// Body parameters
	for _, p := range op.BodyParams {
		properties[p.Name] = paramToSchema(p)
		if p.Required {
			required = append(required, p.Name)
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// paramToSchema converts a Param to a JSON Schema property.
func paramToSchema(p model.Param) map[string]interface{} {
	s := map[string]interface{}{
		"type": p.Type,
	}
	if p.Description != "" {
		s["description"] = p.Description
	}
	if p.Default != nil {
		s["default"] = p.Default
	}
	if len(p.Enum) > 0 {
		s["enum"] = p.Enum
	}
	return s
}
