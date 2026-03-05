package spec

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"

	"github.com/devstroop/rest-mcp/internal/config"
	"github.com/devstroop/rest-mcp/internal/logger"
	"github.com/devstroop/rest-mcp/internal/model"
)

// httpRegex matches URLs starting with http:// or https://
var httpRegex = regexp.MustCompile(`^https?://`)

// nonAlphaNum matches characters that are not alphanumeric or underscore.
var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// ParseOpenAPISpec loads an OpenAPI 3.x spec from a file path or URL,
// resolves all $ref references, extracts operations, applies filters,
// and returns a slice of model.Operation.
func ParseOpenAPISpec(specPath string, filters config.Filters) ([]model.Operation, error) {
	doc, err := loadSpec(specPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI spec: %w", err)
	}

	// Validate the document
	if err := doc.Validate(openapi3.NewLoader().Context); err != nil {
		logger.Warn("OpenAPI spec validation warnings", map[string]interface{}{
			"error": err.Error(),
		})
		// Continue despite validation warnings — many real-world specs aren't perfect
	}

	// Extract operations
	ops, err := extractOperations(doc, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to extract operations: %w", err)
	}

	logger.Info("OpenAPI spec parsed", map[string]interface{}{
		"spec":       specPath,
		"version":    doc.OpenAPI,
		"operations": len(ops),
	})

	return ops, nil
}

// loadSpec loads an OpenAPI spec from a file path or URL.
// Auto-detects Swagger 2.0 vs OpenAPI 3.x and converts 2.0 → 3.0 if needed.
func loadSpec(specPath string) (*openapi3.T, error) {
	// Read the raw spec data first to detect version
	data, err := readSpecData(specPath)
	if err != nil {
		return nil, err
	}

	// Quick version detection: check for "swagger" key (2.0) vs "openapi" key (3.x)
	if isSwagger2(data) {
		logger.Info("detected Swagger 2.0 spec, converting to OpenAPI 3.0", nil)
		return loadSwagger2(data)
	}

	// OpenAPI 3.x — use the standard loader
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	if httpRegex.MatchString(specPath) {
		u, err := url.Parse(specPath)
		if err != nil {
			return nil, fmt.Errorf("invalid spec URL %q: %w", specPath, err)
		}
		return loader.LoadFromURI(u)
	}

	return loader.LoadFromFile(specPath)
}

// readSpecData reads the raw bytes of a spec from file or URL.
func readSpecData(specPath string) ([]byte, error) {
	if httpRegex.MatchString(specPath) {
		resp, err := http.Get(specPath)
		if err != nil {
			return nil, fmt.Errorf("fetch spec from %q: %w", specPath, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetch spec from %q: HTTP %d", specPath, resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 50<<20)) // 50MB limit
	}

	return os.ReadFile(specPath)
}

// isSwagger2 checks if the raw spec data contains a "swagger" key indicating v2.0.
func isSwagger2(data []byte) bool {
	// Quick JSON probe
	var probe struct {
		Swagger string `json:"swagger" yaml:"swagger"`
		OpenAPI string `json:"openapi" yaml:"openapi"`
	}
	if err := json.Unmarshal(data, &probe); err == nil {
		if strings.HasPrefix(probe.Swagger, "2.") {
			return true
		}
		return false
	}

	// Fallback for YAML: look for the swagger field
	s := string(data[:min(500, len(data))])
	return strings.Contains(s, `"swagger"`) || strings.Contains(s, `swagger:`) || strings.Contains(s, `'swagger'`)
}

// loadSwagger2 parses raw Swagger 2.0 JSON/YAML and converts it to OpenAPI 3.0.
func loadSwagger2(data []byte) (*openapi3.T, error) {
	var doc2 openapi2.T
	if err := json.Unmarshal(data, &doc2); err != nil {
		return nil, fmt.Errorf("parse Swagger 2.0 spec: %w", err)
	}

	doc3, err := openapi2conv.ToV3(&doc2)
	if err != nil {
		return nil, fmt.Errorf("convert Swagger 2.0 → OpenAPI 3.0: %w", err)
	}

	logger.Info("Swagger 2.0 → OpenAPI 3.0 conversion complete", map[string]interface{}{
		"title":   doc3.Info.Title,
		"version": doc3.Info.Version,
	})

	return doc3, nil
}

// extractOperations iterates over all paths and methods in the spec,
// converting each to a model.Operation. It applies tag/path/operation filters.
func extractOperations(doc *openapi3.T, filters config.Filters) ([]model.Operation, error) {
	if doc.Paths == nil {
		return nil, fmt.Errorf("spec contains no paths")
	}

	// Pre-count paths for capacity hint
	pathList := doc.Paths.InMatchingOrder()
	// Rough estimate: ~2 methods per path on average
	ops := make([]model.Operation, 0, len(pathList)*2)

	// Iterate paths in matching order for deterministic output
	for i, path := range pathList {
		// Check path exclusion filter
		if isPathExcluded(path, filters.ExcludePaths) {
			logger.Debug("path excluded by filter", map[string]interface{}{"path": path})
			continue
		}

		// Log progress for large specs (every 100 paths)
		if len(pathList) > 100 && i > 0 && i%100 == 0 {
			logger.Info("parsing progress", map[string]interface{}{
				"paths_processed": i,
				"paths_total":     len(pathList),
				"operations":      len(ops),
			})
		}

		pathItem := doc.Paths.Find(path)
		if pathItem == nil {
			continue
		}

		// Get path-level parameters (inherited by all operations)
		pathParams := pathItem.Parameters

		// Iterate over HTTP methods
		for method, operation := range pathItem.Operations() {
			if operation == nil {
				continue
			}

			// Apply operation-level filters
			if !passesFilters(operation, path, method, filters) {
				logger.Debug("operation excluded by filter", map[string]interface{}{
					"path":   path,
					"method": method,
				})
				continue
			}

			op, err := convertOperation(path, method, operation, pathParams)
			if err != nil {
				logger.Warn("skipping operation", map[string]interface{}{
					"path":   path,
					"method": method,
					"error":  err.Error(),
				})
				continue
			}

			// Skip operations marked as hidden via x-rest-mcp-hidden
			if op.Hidden {
				logger.Debug("operation hidden by x-rest-mcp-hidden", map[string]interface{}{
					"path":   path,
					"method": method,
					"name":   op.Name,
				})
				continue
			}

			ops = append(ops, op)
		}
	}

	return ops, nil
}

// convertOperation converts an OpenAPI operation to a model.Operation.
func convertOperation(path, method string, op *openapi3.Operation, pathLevelParams openapi3.Parameters) (model.Operation, error) {
	name := deriveToolName(op, path, method)
	desc := deriveDescription(op)

	// Handle OpenAPI extensions (M4-06)
	// x-rest-mcp-name overrides the tool name
	if extName, ok := getExtensionString(op.Extensions, "x-rest-mcp-name"); ok {
		name = sanitizeToolName(extName)
	}

	// x-rest-mcp-hidden hides the operation
	hidden := false
	if extHidden, ok := getExtensionBool(op.Extensions, "x-rest-mcp-hidden"); ok {
		hidden = extHidden
	}

	result := model.Operation{
		Name:        name,
		Method:      strings.ToUpper(method),
		Path:        path,
		Description: desc,
		Tags:        op.Tags,
		Hidden:      hidden,
	}

	// Merge path-level and operation-level parameters.
	// Operation-level params override path-level ones with the same "in" + "name".
	mergedParams := mergeParameters(pathLevelParams, op.Parameters)

	// Process parameters by location
	for _, paramRef := range mergedParams {
		if paramRef == nil || paramRef.Value == nil {
			continue
		}
		param := paramRef.Value

		mp := convertParameter(param)

		switch param.In {
		case openapi3.ParameterInPath:
			mp.Required = true // Path params are always required per OpenAPI spec
			result.PathParams = append(result.PathParams, mp)
		case openapi3.ParameterInQuery:
			result.QueryParams = append(result.QueryParams, mp)
		case openapi3.ParameterInHeader:
			// Skip header params — they're set via config headers, not tool args
			logger.Debug("skipping header parameter", map[string]interface{}{
				"param": param.Name,
				"tool":  name,
			})
		case openapi3.ParameterInCookie:
			// Skip cookie params
		}
	}

	// Process request body
	if op.RequestBody != nil && op.RequestBody.Value != nil {
		bodyParams := extractBodyParams(op.RequestBody.Value)
		result.BodyParams = append(result.BodyParams, bodyParams...)
	}

	return result, nil
}

// deriveToolName generates the MCP tool name from an OpenAPI operation.
// Priority: operationId → method_path snake_case fallback.
func deriveToolName(op *openapi3.Operation, path, method string) string {
	if op.OperationID != "" {
		return sanitizeToolName(op.OperationID)
	}

	// Fallback: build name from method + path
	// e.g. GET /users/{id}/posts → get_users_id_posts
	cleanPath := strings.TrimPrefix(path, "/")
	cleanPath = strings.ReplaceAll(cleanPath, "{", "")
	cleanPath = strings.ReplaceAll(cleanPath, "}", "")
	name := strings.ToLower(method) + "_" + cleanPath
	return sanitizeToolName(name)
}

// sanitizeToolName makes a string safe for use as an MCP tool name.
// Replaces non-alphanumeric chars with underscores, trims, and lowercases.
func sanitizeToolName(name string) string {
	name = nonAlphaNum.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	name = strings.ToLower(name)

	// Collapse consecutive underscores
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}

	return name
}

// deriveDescription generates a tool description from Operation metadata.
func deriveDescription(op *openapi3.Operation) string {
	if op.Description != "" {
		return op.Description
	}
	if op.Summary != "" {
		return op.Summary
	}
	return ""
}

// convertParameter converts an OpenAPI Parameter to a model.Param.
func convertParameter(param *openapi3.Parameter) model.Param {
	mp := model.Param{
		Name:        param.Name,
		Type:        "string", // default
		Description: param.Description,
		Required:    param.Required,
	}

	if param.Schema != nil && param.Schema.Value != nil {
		schema := param.Schema.Value
		mp.Type = schemaType(schema)
		mp.Description = bestDescription(mp.Description, schema.Description)

		if schema.Default != nil {
			mp.Default = schema.Default
		}

		mp.Enum = extractEnum(schema)
	}

	return mp
}

// extractBodyParams extracts top-level properties from a request body schema
// and converts them to model.Param entries.
func extractBodyParams(body *openapi3.RequestBody) []model.Param {
	var params []model.Param

	// Try JSON content first, then form data
	mediaType := body.Content.Get("application/json")
	if mediaType == nil {
		mediaType = body.Content.Get("application/x-www-form-urlencoded")
	}
	if mediaType == nil {
		mediaType = body.Content.Get("multipart/form-data")
	}
	if mediaType == nil {
		// Try wildcard
		for _, mt := range body.Content {
			mediaType = mt
			break
		}
	}

	if mediaType == nil || mediaType.Schema == nil || mediaType.Schema.Value == nil {
		return params
	}

	schema := mediaType.Schema.Value

	// If the schema is an object with properties, flatten top-level props into params
	if schema.Type != nil && schema.Type.Is("object") && len(schema.Properties) > 0 {
		requiredSet := make(map[string]bool, len(schema.Required))
		for _, r := range schema.Required {
			requiredSet[r] = true
		}

		// Sort property names for deterministic output
		propNames := make([]string, 0, len(schema.Properties))
		for name := range schema.Properties {
			propNames = append(propNames, name)
		}
		sort.Strings(propNames)

		for _, name := range propNames {
			propRef := schema.Properties[name]
			if propRef == nil || propRef.Value == nil {
				continue
			}
			prop := propRef.Value

			mp := model.Param{
				Name:        name,
				Type:        schemaType(prop),
				Description: prop.Description,
				Required:    requiredSet[name],
			}

			if prop.Default != nil {
				mp.Default = prop.Default
			}

			mp.Enum = extractEnum(prop)
			params = append(params, mp)
		}
	} else if schema.Type != nil && schema.Type.Is("array") {
		// If body is an array, expose a single "body" param
		params = append(params, model.Param{
			Name:        "body",
			Type:        "array",
			Description: schema.Description,
			Required:    body.Required,
		})
	} else {
		// For non-object bodies (primitives, or no type), expose a single "body" param
		params = append(params, model.Param{
			Name:        "body",
			Type:        schemaType(schema),
			Description: schema.Description,
			Required:    body.Required,
		})
	}

	return params
}

// mergeParameters merges path-level parameters with operation-level parameters.
// Operation-level parameters override path-level ones with the same in+name.
func mergeParameters(pathLevel, opLevel openapi3.Parameters) openapi3.Parameters {
	if len(pathLevel) == 0 {
		return opLevel
	}
	if len(opLevel) == 0 {
		return pathLevel
	}

	// Build a set of op-level param keys
	opKeys := make(map[string]bool, len(opLevel))
	for _, p := range opLevel {
		if p != nil && p.Value != nil {
			opKeys[p.Value.In+":"+p.Value.Name] = true
		}
	}

	// Start with op-level params, then add path-level ones not already present
	merged := make(openapi3.Parameters, 0, len(opLevel)+len(pathLevel))
	merged = append(merged, opLevel...)
	for _, p := range pathLevel {
		if p != nil && p.Value != nil {
			key := p.Value.In + ":" + p.Value.Name
			if !opKeys[key] {
				merged = append(merged, p)
			}
		}
	}

	return merged
}

// schemaType extracts the JSON Schema type string from an OpenAPI Schema.
func schemaType(schema *openapi3.Schema) string {
	if schema.Type == nil {
		// Try to infer from other indicators
		if len(schema.Properties) > 0 {
			return "object"
		}
		if schema.Items != nil {
			return "array"
		}
		return "string" // safe default
	}

	// kin-openapi v0.124+ uses *Types ([]string) for type field
	if schema.Type.Is("string") {
		return "string"
	}
	if schema.Type.Is("integer") {
		return "integer"
	}
	if schema.Type.Is("number") {
		return "number"
	}
	if schema.Type.Is("boolean") {
		return "boolean"
	}
	if schema.Type.Is("array") {
		return "array"
	}
	if schema.Type.Is("object") {
		return "object"
	}

	// Fallback: try to get first type from the slice
	types := schema.Type.Slice()
	if len(types) > 0 {
		// Filter out "null" and return first real type
		for _, t := range types {
			if t != "null" {
				return t
			}
		}
	}

	return "string"
}

// extractEnum converts the []any Enum field from OpenAPI schema to []string.
func extractEnum(schema *openapi3.Schema) []string {
	if len(schema.Enum) == 0 {
		return nil
	}

	enums := make([]string, 0, len(schema.Enum))
	for _, v := range schema.Enum {
		enums = append(enums, fmt.Sprintf("%v", v))
	}
	return enums
}

// bestDescription returns the first non-empty description.
func bestDescription(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

// passesFilters checks whether an operation should be included based on filters.
func passesFilters(op *openapi3.Operation, path, method string, f config.Filters) bool {
	// Include tags filter: operation must have at least one matching tag
	if len(f.IncludeTags) > 0 {
		if !hasMatchingTag(op.Tags, f.IncludeTags) {
			return false
		}
	}

	// Include operations filter: operation ID must be in the list
	if len(f.IncludeOperations) > 0 {
		if op.OperationID == "" || !stringInSlice(op.OperationID, f.IncludeOperations) {
			return false
		}
	}

	// Exclude operations filter
	if len(f.ExcludeOperations) > 0 {
		if op.OperationID != "" && stringInSlice(op.OperationID, f.ExcludeOperations) {
			return false
		}
	}

	return true
}

// isPathExcluded checks if a path matches any exclusion pattern.
func isPathExcluded(path string, excludePaths []string) bool {
	for _, pattern := range excludePaths {
		// Support simple prefix matching and exact matching
		if pattern == path || strings.HasPrefix(path, pattern) {
			return true
		}
	}
	return false
}

// hasMatchingTag checks if any of the operation's tags match the include list.
func hasMatchingTag(opTags, includeTags []string) bool {
	for _, ot := range opTags {
		for _, it := range includeTags {
			if strings.EqualFold(ot, it) {
				return true
			}
		}
	}
	return false
}

// stringInSlice checks if a string exists in a slice (case-insensitive).
func stringInSlice(s string, list []string) bool {
	for _, item := range list {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}

// getExtensionString retrieves a string value from OpenAPI extensions map.
func getExtensionString(extensions map[string]interface{}, key string) (string, bool) {
	if extensions == nil {
		return "", false
	}
	val, ok := extensions[key]
	if !ok {
		return "", false
	}
	// The value may be a raw JSON string or already decoded
	switch v := val.(type) {
	case string:
		return v, true
	case json.RawMessage:
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			return s, true
		}
	}
	return fmt.Sprintf("%v", val), true
}

// getExtensionBool retrieves a boolean value from OpenAPI extensions map.
func getExtensionBool(extensions map[string]interface{}, key string) (bool, bool) {
	if extensions == nil {
		return false, false
	}
	val, ok := extensions[key]
	if !ok {
		return false, false
	}
	switch v := val.(type) {
	case bool:
		return v, true
	case json.RawMessage:
		var b bool
		if err := json.Unmarshal(v, &b); err == nil {
			return b, true
		}
	}
	return false, false
}
