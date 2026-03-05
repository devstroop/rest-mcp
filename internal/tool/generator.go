package tool

import (
	"encoding/json"
	"fmt"

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
// It detects name collisions and disambiguates by appending _2, _3, etc.
func Generate(ops []model.Operation) []ToolEntry {
	entries := make([]ToolEntry, 0, len(ops))
	nameCount := make(map[string]int, len(ops))

	for i := range ops {
		// Track how many times this name has appeared
		nameCount[ops[i].Name]++
		if nameCount[ops[i].Name] > 1 {
			ops[i].Name = fmt.Sprintf("%s_%d", ops[i].Name, nameCount[ops[i].Name])
		}

		tool := buildTool(ops[i])
		entries = append(entries, ToolEntry{
			Tool:      tool,
			Operation: ops[i],
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
	// Array type must include items schema (MCP/JSON Schema requirement)
	if p.Type == "array" {
		itemsType := p.ItemsType
		if itemsType == "" {
			itemsType = "string" // safe default
		}
		s["items"] = map[string]interface{}{"type": itemsType}
	}
	return s
}
