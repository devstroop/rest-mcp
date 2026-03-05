package spec

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/devstroop/rest-mcp/internal/config"
	"github.com/devstroop/rest-mcp/internal/model"
)

// pathParamRegex matches {paramName} in URL paths.
var pathParamRegex = regexp.MustCompile(`\{([^}]+)\}`)

// ParseManualEndpoints converts TOML [[endpoints]] config into []Operation.
func ParseManualEndpoints(endpoints []config.Endpoint) ([]model.Operation, error) {
	ops := make([]model.Operation, 0, len(endpoints))

	for i, ep := range endpoints {
		if ep.Name == "" {
			return nil, fmt.Errorf("endpoint #%d: name is required", i+1)
		}
		if ep.Method == "" {
			return nil, fmt.Errorf("endpoint %q: method is required", ep.Name)
		}
		if ep.Path == "" {
			return nil, fmt.Errorf("endpoint %q: path is required", ep.Name)
		}

		op := model.Operation{
			Name:        ep.Name,
			Method:      strings.ToUpper(ep.Method),
			Path:        ep.Path,
			Description: ep.Description,
		}

		// Extract path parameters from path template AND from [endpoints.path] config
		pathParamNames := extractPathParamNames(ep.Path)
		for _, name := range pathParamNames {
			param := model.Param{
				Name:     name,
				Type:     "string",
				Required: true,
			}
			// Override with config if provided in [endpoints.path]
			if ep.PathParams != nil {
				if def, ok := ep.PathParams[name]; ok {
					if def.Type != "" {
						param.Type = def.Type
					}
					param.Description = def.Description
					param.Required = true // Path params are always required
				}
			}
			op.PathParams = append(op.PathParams, param)
		}

		// Query parameters
		for name, def := range ep.Query {
			op.QueryParams = append(op.QueryParams, model.Param{
				Name:        name,
				Type:        coerceType(def.Type),
				Description: def.Description,
				Required:    def.Required,
				Default:     def.Default,
				Enum:        def.Enum,
			})
		}

		// Body parameters
		for name, def := range ep.Body {
			op.BodyParams = append(op.BodyParams, model.Param{
				Name:        name,
				Type:        coerceType(def.Type),
				Description: def.Description,
				Required:    def.Required,
				Default:     def.Default,
				Enum:        def.Enum,
			})
		}

		ops = append(ops, op)
	}

	return ops, nil
}

// extractPathParamNames returns parameter names from a path template.
// e.g. "/users/{id}/posts/{postId}" → ["id", "postId"]
func extractPathParamNames(path string) []string {
	matches := pathParamRegex.FindAllStringSubmatch(path, -1)
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
	}
	return names
}

// coerceType normalizes type strings to JSON Schema types.
func coerceType(t string) string {
	switch strings.ToLower(t) {
	case "string", "str":
		return "string"
	case "integer", "int":
		return "integer"
	case "number", "float", "double":
		return "number"
	case "boolean", "bool":
		return "boolean"
	case "array":
		return "array"
	case "object":
		return "object"
	default:
		return "string"
	}
}
