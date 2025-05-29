package swagger

import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	"strings"
)

// Operation wraps an OpenAPI 3.0 operation with additional helper methods.
// It provides a more convenient interface for building OpenAPI operations
// while maintaining compatibility with the underlying openapi3.Operation.
type Operation struct {
	*openapi3.Operation
}

type visitedRefs map[string]struct{}

// ResolveReferences replaces all $ref references in the operation with their actual component
// definitions from the root schema. Returns an error if any references cannot be resolved.
func (o *Operation) ResolveReferences(rootSchema *openapi3.T) error {
	if o == nil || o.Operation == nil {
		return nil
	}

	visited := make(visitedRefs)
	return o.resolveReferences(rootSchema, visited)
}

func (o *Operation) resolveReferences(rootSchema *openapi3.T, visited visitedRefs) error {
	// Resolve top-level references first (breadth-first)
	if err := o.resolveRequestBodyRefs(rootSchema, visited); err != nil {
		return err
	}

	if err := o.resolveResponseRefs(rootSchema, visited); err != nil {
		return err
	}

	return o.resolveParameterRefs(rootSchema, visited)
}

func (o *Operation) resolveRequestBodyRefs(rootSchema *openapi3.T, visited visitedRefs) error {
	if o.RequestBody == nil {
		return nil
	}

	if o.RequestBody.Ref != "" {
		if _, exists := visited[o.RequestBody.Ref]; exists {
			return nil // Already visited this reference
		}
		visited[o.RequestBody.Ref] = struct{}{}

		resolved, err := resolveComponentRef(rootSchema, o.RequestBody.Ref, visited)
		if err != nil {
			return fmt.Errorf("requestBody: %w", err)
		}
		reqBody, ok := resolved.(*openapi3.RequestBody)
		if !ok {
			return fmt.Errorf("requestBody: expected *openapi3.RequestBody, got %T", resolved)
		}
		o.RequestBody.Value = reqBody
	}

	if o.RequestBody.Value != nil {
		return resolveContentRefs(rootSchema, o.RequestBody.Value.Content, "requestBody", visited)
	}

	return nil
}

func (o *Operation) resolveResponseRefs(rootSchema *openapi3.T, visited visitedRefs) error {
	if o.Responses == nil {
		return nil
	}
	for status, respRef := range o.Responses.Map() {
		if respRef.Ref != "" {
			if _, exists := visited[respRef.Ref]; exists {
				continue // Already visited this reference
			}
			visited[respRef.Ref] = struct{}{}

			resolved, err := resolveComponentRef(rootSchema, respRef.Ref, visited)
			if err != nil {
				return fmt.Errorf("response %s: %w", status, err)
			}
			resp, ok := resolved.(*openapi3.Response)
			if !ok {
				return fmt.Errorf("response %s: expected *openapi3.Response, got %T", status, resolved)
			}
			respRef.Value = resp
		}

		if respRef.Value != nil {
			if err := resolveContentRefs(rootSchema, respRef.Value.Content, fmt.Sprintf("response %s", status), visited); err != nil {
				return err
			}
		}
	}
	return nil
}

func (o *Operation) resolveParameterRefs(rootSchema *openapi3.T, visited visitedRefs) error {
	if o.Parameters == nil {
		return nil
	}
	for _, paramRef := range o.Parameters {
		if paramRef.Ref != "" {
			if _, exists := visited[paramRef.Ref]; exists {
				continue // Already visited this reference
			}
			visited[paramRef.Ref] = struct{}{}

			resolved, err := resolveComponentRef(rootSchema, paramRef.Ref, visited)
			if err != nil {
				// If Value is nil, we can't get the name, so just use the ref
				paramName := paramRef.Ref
				if paramRef.Value != nil {
					paramName = paramRef.Value.Name
				}
				return fmt.Errorf("parameter %q: %w", paramName, err)
			}
			param, ok := resolved.(*openapi3.Parameter)
			if !ok {
				paramName := paramRef.Ref
				if paramRef.Value != nil {
					paramName = paramRef.Value.Name
				}
				return fmt.Errorf("parameter %q: expected *openapi3.Parameter, got %T", paramName, resolved)
			}
			paramRef.Value = param
		}

		if paramRef.Value != nil {
			if paramRef.Value.Content != nil {
				if err := resolveContentRefs(rootSchema, paramRef.Value.Content, fmt.Sprintf("parameter %q", paramRef.Value.Name), visited); err != nil {
					return err
				}
			}

			if paramRef.Value.Schema != nil && paramRef.Value.Schema.Ref != "" {
				if err := convertSchemaRefToStandardFormat(paramRef.Value.Schema, fmt.Sprintf("parameter %q", paramRef.Value.Name), ""); err != nil {
					return err
				}
				if _, exists := visited[paramRef.Value.Schema.Ref]; exists {
					continue // Already visited this reference
				}
				visited[paramRef.Value.Schema.Ref] = struct{}{}

				resolved, err := resolveComponentRef(rootSchema, paramRef.Value.Schema.Ref, visited)
				if err != nil {
					return fmt.Errorf("parameter %q schema: %w", paramRef.Value.Name, err)
				}
				schema, ok := resolved.(*openapi3.Schema)
				if !ok {
					return fmt.Errorf("parameter %q schema: expected *openapi3.Schema, got %T", paramRef.Value.Name, resolved)
				}
				paramRef.Value.Schema.Value = schema
				// Keep the converted Ref in the schema for documentation generation
			}
		}
	}
	return nil
}

// resolveContentRefs handles resolving references in Content maps consistently across all types
func resolveContentRefs(rootSchema *openapi3.T, content openapi3.Content, context string, visited visitedRefs) error {
	for mediaType, media := range content {
		if media.Schema != nil && media.Schema.Ref != "" {
			if err := convertSchemaRefToStandardFormat(media.Schema, context, mediaType); err != nil {
				return err
			}
			if _, exists := visited[media.Schema.Ref]; exists {
				continue // Already visited this reference
			}
			visited[media.Schema.Ref] = struct{}{}

			resolved, err := resolveComponentRef(rootSchema, media.Schema.Ref, visited)
			if err != nil {
				return fmt.Errorf("%s content %q schema: %w", context, mediaType, err)
			}
			media.Schema.Value = resolved.(*openapi3.Schema)
			// Keep the converted Ref in the schema for documentation generation
		}
	}
	return nil
}

// makeComponentRef creates a standard OpenAPI component reference path
func makeComponentRef(componentName string) string {
	return "#/components/schemas/" + componentName
}

// convertSchemaRefToStandardFormat converts a schema reference to standard OpenAPI format
// and preserves the original reference by converting it to #/components/schemas/ format
func convertSchemaRefToStandardFormat(schemaRef *openapi3.SchemaRef, context, mediaType string) error {
	componentName := determineComponentName(schemaRef.Ref, "")
	if componentName == "" {
		return fmt.Errorf("%s content %q: could not determine component name from reference %q",
			context, mediaType, schemaRef.Ref)
	}

	// Convert reference to standard OpenAPI format
	schemaRef.Ref = makeComponentRef(componentName)
	return nil
}

// processSchemaRefs recursively processes all references within a schema
func processSchemaRefs(rootSchema *openapi3.T, schema *openapi3.Schema, visited visitedRefs) error {
	// Process properties
	for _, prop := range schema.Properties {
		if prop.Ref != "" {
			if _, exists := visited[prop.Ref]; exists {
				continue // Skip already visited references to break cycles
			}
			visited[prop.Ref] = struct{}{}

			if err := convertSchemaRefToStandardFormat(prop, "schema property", ""); err != nil {
				return err
			}
			resolved, err := resolveComponentRef(rootSchema, prop.Ref, visited)
			if err != nil {
				return fmt.Errorf("schema property %q: %w", prop.Ref, err)
			}
			prop.Value = resolved.(*openapi3.Schema)
		}
		if prop.Value != nil {
			if err := processSchemaRefs(rootSchema, prop.Value, visited); err != nil {
				return err
			}
		}
	}

	// Process array items
	if schema.Items != nil {
		if schema.Items.Ref != "" {
			if _, exists := visited[schema.Items.Ref]; exists {
				return nil // Skip already visited references to break cycles
			}
			visited[schema.Items.Ref] = struct{}{}

			if err := convertSchemaRefToStandardFormat(schema.Items, "schema items", ""); err != nil {
				return err
			}
			resolved, err := resolveComponentRef(rootSchema, schema.Items.Ref, visited)
			if err != nil {
				return fmt.Errorf("schema items: %w", err)
			}
			schema.Items.Value = resolved.(*openapi3.Schema)
		}
		if schema.Items.Value != nil {
			if err := processSchemaRefs(rootSchema, schema.Items.Value, visited); err != nil {
				return err
			}
		}
	}

	// Process additional properties
	if schema.AdditionalProperties.Schema != nil {
		if schema.AdditionalProperties.Schema.Ref != "" {
			if _, exists := visited[schema.AdditionalProperties.Schema.Ref]; exists {
				return nil // Skip already visited references to break cycles
			}
			visited[schema.AdditionalProperties.Schema.Ref] = struct{}{}

			if err := convertSchemaRefToStandardFormat(schema.AdditionalProperties.Schema, "schema additionalProperties", ""); err != nil {
				return err
			}
			resolved, err := resolveComponentRef(rootSchema, schema.AdditionalProperties.Schema.Ref, visited)
			if err != nil {
				return fmt.Errorf("schema additionalProperties: %w", err)
			}
			schema.AdditionalProperties.Schema.Value = resolved.(*openapi3.Schema)
		}
		if schema.AdditionalProperties.Schema.Value != nil {
			if err := processSchemaRefs(rootSchema, schema.AdditionalProperties.Schema.Value, visited); err != nil {
				return err
			}
		}
	}

	// Process allOf/anyOf/oneOf
	for i, s := range schema.AllOf {
		if s.Ref != "" {
			if _, exists := visited[s.Ref]; exists {
				continue // Skip already visited references to break cycles
			}
			visited[s.Ref] = struct{}{}

			if err := convertSchemaRefToStandardFormat(s, "schema allOf", ""); err != nil {
				return err
			}
			resolved, err := resolveComponentRef(rootSchema, s.Ref, visited)
			if err != nil {
				return fmt.Errorf("schema allOf[%d]: %w", i, err)
			}
			s.Value = resolved.(*openapi3.Schema)
		}
		if s.Value != nil {
			if err := processSchemaRefs(rootSchema, s.Value, visited); err != nil {
				return err
			}
		}
	}
	for i, s := range schema.AnyOf {
		if s.Ref != "" {
			if _, exists := visited[s.Ref]; exists {
				continue // Skip already visited references to break cycles
			}
			visited[s.Ref] = struct{}{}

			if err := convertSchemaRefToStandardFormat(s, "schema anyOf", ""); err != nil {
				return err
			}
			resolved, err := resolveComponentRef(rootSchema, s.Ref, visited)
			if err != nil {
				return fmt.Errorf("schema anyOf[%d]: %w", i, err)
			}
			s.Value = resolved.(*openapi3.Schema)
		}
		if s.Value != nil {
			if err := processSchemaRefs(rootSchema, s.Value, visited); err != nil {
				return err
			}
		}
	}
	for i, s := range schema.OneOf {
		if s.Ref != "" {
			if _, exists := visited[s.Ref]; exists {
				continue // Skip already visited references to break cycles
			}
			visited[s.Ref] = struct{}{}

			if err := convertSchemaRefToStandardFormat(s, "schema oneOf", ""); err != nil {
				return err
			}
			resolved, err := resolveComponentRef(rootSchema, s.Ref, visited)
			if err != nil {
				return fmt.Errorf("schema oneOf[%d]: %w", i, err)
			}
			s.Value = resolved.(*openapi3.Schema)
		}
		if s.Value != nil {
			if err := processSchemaRefs(rootSchema, s.Value, visited); err != nil {
				return err
			}
		}
	}

	return nil
}

func resolveComponentRef(rootSchema *openapi3.T, ref string, visited visitedRefs) (interface{}, error) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("reference %q must point to a component", ref)
	}

	// Split reference into parts
	parts := strings.Split(ref, "/")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid reference format: %q", ref)
	}

	// Whitelist of allowed reference prefixes
	allowedPrefixes := map[string]bool{
		"components":  true,
		"$defs":       true,
		"definitions": true,
	}

	componentType := parts[1]
	if !allowedPrefixes[componentType] {
		return nil, fmt.Errorf("invalid reference format: %q", ref)
	}

	componentName := parts[2] // the actual component name

	// Handle standard OpenAPI component references
	if componentType == "components" && len(parts) >= 4 {
		componentType = parts[2] // e.g. "schemas", "responses" etc.
		componentName = parts[3]
	} else if componentType == "$defs" || componentType == "definitions" {
		// Handle jsonschema $defs and definitions references
		componentType = "schemas" // Treat them as schemas
	}

	switch componentType {
	case "schemas":
		if rootSchema.Components == nil || rootSchema.Components.Schemas == nil {
			return nil, fmt.Errorf("no schemas defined in components")
		}
		schema, exists := rootSchema.Components.Schemas[componentName]
		if !exists {
			return nil, fmt.Errorf("schema %q not found", componentName)
		}

		// Recursively resolve references within the schema itself
		if schema.Value != nil {
			if err := processSchemaRefs(rootSchema, schema.Value, visited); err != nil {
				return nil, fmt.Errorf("failed to process schema refs for %q: %w", componentName, err)
			}
		}

		return schema.Value, nil
	case "responses":
		if rootSchema.Components == nil || rootSchema.Components.Responses == nil {
			return nil, fmt.Errorf("no responses defined in components")
		}
		response, exists := rootSchema.Components.Responses[componentName]
		if !exists {
			return nil, fmt.Errorf("response %q not found", componentName)
		}
		return response.Value, nil
	case "parameters":
		if rootSchema.Components == nil || rootSchema.Components.Parameters == nil {
			return nil, fmt.Errorf("no parameters defined in components")
		}
		param, exists := rootSchema.Components.Parameters[componentName]
		if !exists {
			return nil, fmt.Errorf("parameter %q not found", componentName)
		}
		return param.Value, nil
	case "requestBodies":
		if rootSchema.Components == nil || rootSchema.Components.RequestBodies == nil {
			return nil, fmt.Errorf("no requestBodies defined in components")
		}
		reqBody, exists := rootSchema.Components.RequestBodies[componentName]
		if !exists {
			return nil, fmt.Errorf("requestBody %q not found", componentName)
		}
		return reqBody.Value, nil
	default:
		return nil, fmt.Errorf("unsupported component type: %q", componentType)
	}
}

// NewOperation creates and returns a new Operation instance.
// Initializes the underlying openapi3.Operation with:
// - Empty responses map
// - Nil request body
// - Nil security requirements
func NewOperation() Operation {
	return Operation{
		openapi3.NewOperation(),
	}
}

// AddRequestBody sets the request body definition for the operation.
// The requestBody parameter should be a fully configured openapi3.RequestBody
// containing the expected content types and schemas.
func (o *Operation) AddRequestBody(requestBody *openapi3.RequestBody) {
	o.RequestBody = &openapi3.RequestBodyRef{
		Value: requestBody,
	}
}

// AddResponse adds a response definition to the operation.
// Ensures responses map is initialized and sets empty description if none provided.
// Parameters:
//   - status: HTTP status code (e.g. 200, 404)
//   - response: OpenAPI response definition containing:
//   - Description
//   - Content types and schemas
//   - Headers
func (o *Operation) AddResponse(status int, response *openapi3.Response) {
	if o.Responses == nil {
		o.Responses = &openapi3.Responses{}
	}
	if response.Description == nil {
		response.WithDescription("")
	}
	o.Operation.AddResponse(status, response)
}

// addSecurityRequirements adds security requirements to the operation.
// This is an internal method used when converting from Definitions to Operation.
// securityRequirements: List of security schemes required for this operation
func (o *Operation) addSecurityRequirements(securityRequirements SecurityRequirements) {
	if securityRequirements != nil && o.Security == nil {
		o.Security = openapi3.NewSecurityRequirements()
	}
	for _, securityRequirement := range securityRequirements {
		o.Security.With(openapi3.SecurityRequirement(securityRequirement))
	}
}
