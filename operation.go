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

// ResolveReferences replaces all $ref references in the operation with their actual component
// definitions from the root schema. Returns an error if any references cannot be resolved.
func (o *Operation) ResolveReferences(rootSchema *openapi3.T) error {
	if o == nil || o.Operation == nil {
		return nil
	}

	return o.resolveReferences(rootSchema)
}

// ResolveAllComponents processes all reusable components to resolve their references
func ResolveAllComponents(rootSchema *openapi3.T) error {
	if rootSchema.Components == nil {
		return nil
	}

	// Process all component types in dependency order
	componentTypes := []struct {
		name   string
		values map[string]interface{}
	}{
		{"schemas", interfaceMap(rootSchema.Components.Schemas)},
		{"parameters", interfaceMap(rootSchema.Components.Parameters)},
		{"requestBodies", interfaceMap(rootSchema.Components.RequestBodies)},
		{"responses", interfaceMap(rootSchema.Components.Responses)},
	}

	for _, ct := range componentTypes {
		if ct.values == nil {
			continue
		}

		for name, comp := range ct.values {
			ref := getRef(comp)
			if ref != "" {
				// Convert to standard format if it's a schema ref
				if schemaRef, ok := comp.(*openapi3.SchemaRef); ok {
					if err := convertSchemaRefToStandardFormat(schemaRef, ct.name+" "+name, ""); err != nil {
						return fmt.Errorf("component %s %q: %w", ct.name, name, err)
					}
					ref = schemaRef.Ref // Use converted ref
				}

				resolved, err := resolveComponent(rootSchema, ref)
				if err != nil {
					return fmt.Errorf("%s %q: %w", ct.name, name, err)
				}

				if err := setValue(comp, resolved); err != nil {
					return fmt.Errorf("%s %q: %w", ct.name, name, err)
				}
			}

			// Process nested references
			if err := processNestedRefs(rootSchema, getValue(comp)); err != nil {
				return fmt.Errorf("%s %q: %w", ct.name, name, err)
			}
		}
	}

	return nil
}

// Helper function to convert component maps to interface{} maps
func interfaceMap[T any](m map[string]*T) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// getRef extracts the reference from a component
func getRef(v interface{}) string {
	switch x := v.(type) {
	case *openapi3.SchemaRef:
		return x.Ref
	case *openapi3.ParameterRef:
		return x.Ref
	case *openapi3.RequestBodyRef:
		return x.Ref
	case *openapi3.ResponseRef:
		return x.Ref
	}
	return ""
}

// getValue extracts the value from a component
func getValue(v interface{}) interface{} {
	switch x := v.(type) {
	case *openapi3.SchemaRef:
		return x.Value
	case *openapi3.ParameterRef:
		return x.Value
	case *openapi3.RequestBodyRef:
		return x.Value
	case *openapi3.ResponseRef:
		return x.Value
	}
	return nil
}

// setValue sets the resolved value on a component
func setValue(v interface{}, resolved interface{}) error {
	switch x := v.(type) {
	case *openapi3.SchemaRef:
		schema, ok := resolved.(*openapi3.Schema)
		if !ok {
			return fmt.Errorf("expected *openapi3.Schema, got %T", resolved)
		}
		x.Value = schema
	case *openapi3.ParameterRef:
		param, ok := resolved.(*openapi3.Parameter)
		if !ok {
			return fmt.Errorf("expected *openapi3.Parameter, got %T", resolved)
		}
		x.Value = param
	case *openapi3.RequestBodyRef:
		body, ok := resolved.(*openapi3.RequestBody)
		if !ok {
			return fmt.Errorf("expected *openapi3.RequestBody, got %T", resolved)
		}
		x.Value = body
	case *openapi3.ResponseRef:
		resp, ok := resolved.(*openapi3.Response)
		if !ok {
			return fmt.Errorf("expected *openapi3.Response, got %T", resolved)
		}
		x.Value = resp
	}
	return nil
}

// processNestedRefs handles references within component values
func processNestedRefs(rootSchema *openapi3.T, v interface{}) error {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case *openapi3.Schema:
		return processSchemaRefs(rootSchema, val)
	case *openapi3.Parameter:
		if val.Schema != nil {
			if val.Schema.Ref != "" {
				if err := convertSchemaRefToStandardFormat(val.Schema, "parameter schema", ""); err != nil {
					return err
				}
				resolved, err := resolveComponent(rootSchema, val.Schema.Ref)
				if err != nil {
					return fmt.Errorf("parameter schema: %w", err)
				}
				val.Schema.Value = resolved.(*openapi3.Schema)
			}
			if val.Schema.Value != nil {
				return processSchemaRefs(rootSchema, val.Schema.Value)
			}
		}
		if val.Content != nil {
			return resolveContentRefs(rootSchema, val.Content, "parameter content")
		}
	case *openapi3.RequestBody:
		if val.Content != nil {
			return resolveContentRefs(rootSchema, val.Content, "request body")
		}
	case *openapi3.Response:
		if val.Content != nil {
			return resolveContentRefs(rootSchema, val.Content, "response")
		}
	}
	return nil
}

// resolveComponent resolves a reference to a component
func resolveComponent(rootSchema *openapi3.T, ref string) (interface{}, error) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("reference %q must point to a component", ref)
	}

	parts := strings.Split(ref, "/")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid reference format: %q", ref)
	}

	componentType := parts[1]
	componentName := parts[2]

	// Handle standard OpenAPI component references
	if componentType == "components" && len(parts) >= 4 {
		componentType = parts[2] // e.g. "schemas", "responses" etc.
		componentName = parts[3]
	} else if componentType == "$defs" || componentType == "definitions" {
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

func (o *Operation) resolveReferences(rootSchema *openapi3.T) error {
	// Resolve top-level references first (breadth-first)
	if err := o.resolveRequestBodyRefs(rootSchema); err != nil {
		return err
	}

	if err := o.resolveResponseRefs(rootSchema); err != nil {
		return err
	}

	return o.resolveParameterRefs(rootSchema)
}

func (o *Operation) resolveRequestBodyRefs(rootSchema *openapi3.T) error {
	if o.RequestBody == nil {
		return nil
	}

	if o.RequestBody.Ref != "" {
		resolved, err := resolveComponentRef(rootSchema, o.RequestBody.Ref)
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
		return resolveContentRefs(rootSchema, o.RequestBody.Value.Content, "requestBody")
	}

	return nil
}

func (o *Operation) resolveResponseRefs(rootSchema *openapi3.T) error {
	if o.Responses == nil {
		return nil
	}
	for status, respRef := range o.Responses.Map() {
		if respRef.Ref != "" {
			resolved, err := resolveComponentRef(rootSchema, respRef.Ref)
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
			if err := resolveContentRefs(rootSchema, respRef.Value.Content, fmt.Sprintf("response %s", status)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (o *Operation) resolveParameterRefs(rootSchema *openapi3.T) error {
	if o.Parameters == nil {
		return nil
	}
	for _, paramRef := range o.Parameters {
		if paramRef.Ref != "" {
			resolved, err := resolveComponentRef(rootSchema, paramRef.Ref)
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
				if err := resolveContentRefs(rootSchema, paramRef.Value.Content, fmt.Sprintf("parameter %q", paramRef.Value.Name)); err != nil {
					return err
				}
			}

			if paramRef.Value.Schema != nil && paramRef.Value.Schema.Ref != "" {
				if err := convertSchemaRefToStandardFormat(paramRef.Value.Schema, fmt.Sprintf("parameter %q", paramRef.Value.Name), ""); err != nil {
					return err
				}
				resolved, err := resolveComponentRef(rootSchema, paramRef.Value.Schema.Ref)
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
func resolveContentRefs(rootSchema *openapi3.T, content openapi3.Content, context string) error {
	for mediaType, media := range content {
		if media.Schema != nil {
			if media.Schema.Ref != "" {
				if err := convertSchemaRefToStandardFormat(media.Schema, context, mediaType); err != nil {
					return err
				}
				resolved, err := resolveComponentRef(rootSchema, media.Schema.Ref)
				if err != nil {
					return fmt.Errorf("%s content %q schema: %w", context, mediaType, err)
				}
				media.Schema.Value = resolved.(*openapi3.Schema)
				// Keep the converted Ref in the schema for documentation generation
			}

			// Recursively process any references within the schema itself
			if media.Schema.Value != nil {
				if err := processSchemaRefs(rootSchema, media.Schema.Value); err != nil {
					return fmt.Errorf("%s content %q schema: %w", context, mediaType, err)
				}
			}
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
func processSchemaRefs(rootSchema *openapi3.T, schema *openapi3.Schema) error {
	if schema == nil {
		return nil
	}

	// Process properties
	for _, prop := range schema.Properties {
		if prop.Ref != "" {
			if err := convertSchemaRefToStandardFormat(prop, "schema property", ""); err != nil {
				return err
			}
			resolved, err := resolveComponentRef(rootSchema, prop.Ref)
			if err != nil {
				return fmt.Errorf("schema property %q: %w", prop.Ref, err)
			}
			prop.Value = resolved.(*openapi3.Schema)
		}
		if prop.Value != nil {
			if err := processSchemaRefs(rootSchema, prop.Value); err != nil {
				return err
			}
		}
	}

	// Process array items
	if schema.Items != nil {
		if schema.Items.Ref != "" {
			if err := convertSchemaRefToStandardFormat(schema.Items, "schema items", ""); err != nil {
				return err
			}
			resolved, err := resolveComponentRef(rootSchema, schema.Items.Ref)
			if err != nil {
				return fmt.Errorf("schema items: %w", err)
			}
			schema.Items.Value = resolved.(*openapi3.Schema)
		}
		if schema.Items.Value != nil {
			if err := processSchemaRefs(rootSchema, schema.Items.Value); err != nil {
				return err
			}
		}
	}

	// Process additional properties
	if schema.AdditionalProperties.Schema != nil {
		if schema.AdditionalProperties.Schema.Ref != "" {
			if err := convertSchemaRefToStandardFormat(schema.AdditionalProperties.Schema, "schema additionalProperties", ""); err != nil {
				return err
			}
			resolved, err := resolveComponentRef(rootSchema, schema.AdditionalProperties.Schema.Ref)
			if err != nil {
				return fmt.Errorf("schema additionalProperties: %w", err)
			}
			schema.AdditionalProperties.Schema.Value = resolved.(*openapi3.Schema)
		}
		if schema.AdditionalProperties.Schema.Value != nil {
			if err := processSchemaRefs(rootSchema, schema.AdditionalProperties.Schema.Value); err != nil {
				return err
			}
		}
	}

	// Process allOf/anyOf/oneOf
	for i, s := range schema.AllOf {
		if s.Ref != "" {
			if err := convertSchemaRefToStandardFormat(s, "schema allOf", ""); err != nil {
				return err
			}
			resolved, err := resolveComponentRef(rootSchema, s.Ref)
			if err != nil {
				return fmt.Errorf("schema allOf[%d]: %w", i, err)
			}
			s.Value = resolved.(*openapi3.Schema)
		}
		if s.Value != nil {
			if err := processSchemaRefs(rootSchema, s.Value); err != nil {
				return err
			}
		}
	}
	for i, s := range schema.AnyOf {
		if s.Ref != "" {
			if err := convertSchemaRefToStandardFormat(s, "schema anyOf", ""); err != nil {
				return err
			}
			resolved, err := resolveComponentRef(rootSchema, s.Ref)
			if err != nil {
				return fmt.Errorf("schema anyOf[%d]: %w", i, err)
			}
			s.Value = resolved.(*openapi3.Schema)
		}
		if s.Value != nil {
			if err := processSchemaRefs(rootSchema, s.Value); err != nil {
				return err
			}
		}
	}
	for i, s := range schema.OneOf {
		if s.Ref != "" {
			if err := convertSchemaRefToStandardFormat(s, "schema oneOf", ""); err != nil {
				return err
			}
			resolved, err := resolveComponentRef(rootSchema, s.Ref)
			if err != nil {
				return fmt.Errorf("schema oneOf[%d]: %w", i, err)
			}
			s.Value = resolved.(*openapi3.Schema)
		}
		if s.Value != nil {
			if err := processSchemaRefs(rootSchema, s.Value); err != nil {
				return err
			}
		}
	}

	return nil
}

func resolveComponentRef(rootSchema *openapi3.T, ref string) (any, error) {
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
			if err := processSchemaRefs(rootSchema, schema.Value); err != nil {
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
