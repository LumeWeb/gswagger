package swagger

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/invopop/jsonschema"
)

// Error variables for OpenAPI schema generation failures
var (
	// ErrResponses indicates failure generating response schemas
	ErrResponses = errors.New("errors generating responses schema")
	// ErrRequestBody indicates failure generating request body schema
	ErrRequestBody = errors.New("errors generating request body schema")
	// ErrPathParams indicates failure generating path parameter schemas
	ErrPathParams = errors.New("errors generating path parameters schema")
	// ErrQuerystring indicates failure generating querystring parameter schemas
	ErrQuerystring = errors.New("errors generating querystring schema")
)

// AddRawRoute adds a route with explicit OpenAPI Operation definition.
// This lower-level method allows full control over the OpenAPI operation definition.
// Parameters:
//   - method: HTTP method (GET, POST, etc.)
//   - routePath: URL path pattern
//   - handler: Request handler function
//   - operation: Predefined OpenAPI Operation
//   - middleware: Optional middleware chain
//
// Returns:
//   - Route: Framework-specific route object
//   - error: Validation error if operation is invalid
func (r *Router[HandlerFunc, MiddlewareFunc, Route]) AddRawRoute(method string, routePath string, handler HandlerFunc, operation Operation, middleware ...MiddlewareFunc) (Route, error) {
	op := operation.Operation
	if op == nil {
		op = openapi3.NewOperation()
		if op.Responses == nil {
			op.Responses = openapi3.NewResponses()
		}
	}

	pathWithPrefix := path.Join(r.pathPrefix, routePath)
	oasPath := r.router.TransformPathToOasPath(pathWithPrefix)
	r.swaggerSchema.AddOperation(oasPath, method, op)

	pathWithPrefix = routePath
	if !r.isSubrouter {
		pathWithPrefix = path.Join(r.pathPrefix, routePath)
	}

	return r.router.AddRoute(method, pathWithPrefix, handler, middleware...), nil
}

// Content defines media type schemas for request/response bodies
// Key is media type (e.g. "application/json"), value is Schema
type Content map[string]Schema

// Schema defines the structure of request/response data
type Schema struct {
	Value                     interface{} // Go type to generate schema from
	AllowAdditionalProperties bool        // Whether to allow extra fields
}

// Parameter defines an API parameter (path, query, header, cookie)
type Parameter struct {
	Content     Content // Media type schemas (alternative to Schema)
	Schema      *Schema // Parameter schema definition
	Description string  // Human-readable description
	Required    bool    // Whether parameter is required
}

// ParameterValue maps parameter names to their definitions
type ParameterValue map[string]Parameter

// ParameterDefinition defines a reusable parameter component
type ParameterDefinition struct {
	In          string  // Location (path, query, header, cookie)
	Required    bool    // Whether parameter is required
	Description string  // Human-readable description
	Content     Content // Media type schemas (alternative to Schema)
	Schema      *Schema // Parameter schema definition
}

// ContentValue defines request/response body content
type ContentValue struct {
	Content     Content           // Media type schemas
	Description string            // Human-readable description
	Headers     map[string]string // Response headers
	Required    bool              // Whether body is required
}

// SecurityRequirements lists required security schemes
type SecurityRequirements []SecurityRequirement

// SecurityRequirement maps security scheme names to required scopes
type SecurityRequirement map[string][]string

// Definitions provides OpenAPI schema definitions for a route
type Definitions struct {
	Extensions  map[string]interface{}         // OpenAPI extensions
	Tags        []string                       // Logical grouping tags
	Summary     string                         // Short summary
	Description string                         // Detailed description
	Deprecated  bool                           // Whether endpoint is deprecated
	Parameters  map[string]ParameterDefinition // Reusable parameters
	PathParams  ParameterValue                 // Path parameters
	Querystring ParameterValue                 // Query parameters
	Headers     ParameterValue                 // Header parameters
	Cookies     ParameterValue                 // Cookie parameters
	RequestBody *ContentValue                  // Request body definition
	Responses   map[int]ContentValue           // Response definitions by status code
	Security    SecurityRequirements           // Security requirements
}

// newOperationFromDefinition converts Definitions to an OpenAPI Operation
// Handles:
// - Tags, summary, description
// - Security requirements
// - Parameters (path, query, header, cookie)
// - Request body
// - Responses
func newOperationFromDefinition(schema Definitions) Operation {
	operation := NewOperation()
	operation.Responses = &openapi3.Responses{}
	operation.Tags = schema.Tags
	operation.Extensions = schema.Extensions
	operation.addSecurityRequirements(schema.Security)
	operation.Description = schema.Description
	operation.Summary = schema.Summary
	operation.Deprecated = schema.Deprecated

	return operation
}

// Constants for OpenAPI parameter locations and content types
const (
	pathParamsType  = "path"                // Path parameter location
	queryParamType  = "query"               // Query parameter location
	headerParamType = "header"              // Header parameter location
	cookieParamType = "cookie"              // Cookie parameter location
	jsonType        = "application/json"    // JSON content type
	formDataType    = "multipart/form-data" // Form data content type
)

// AddRoute adds a route with OpenAPI schema inferred from Definitions.
// Automatically handles:
// - Path parameters from route path
// - Query parameters
// - Headers
// - Cookies
// - Request body
// - Responses
// Parameters:
//   - method: HTTP method (GET, POST, etc.)
//   - path: URL path pattern
//   - handler: Request handler function
//   - schema: OpenAPI definitions for the route
//   - middleware: Optional middleware chain
//
// Returns:
//   - Route: Framework-specific route object
//   - error: Validation error if schema is invalid
func (r *Router[HandlerFunc, MiddlewareFunc, Route]) AddRoute(method string, path string, handler HandlerFunc, schema Definitions, middleware ...MiddlewareFunc) (Route, error) {
	operation := newOperationFromDefinition(schema)

	// Collect all parameters from different sources
	allParams := make(map[string]ParameterDefinition)

	// Add parameters from Definitions.Parameters (highest priority)
	for name, paramDef := range schema.Parameters {
		allParams[name] = paramDef
	}

	oasPath := r.router.TransformPathToOasPath(path)

	// Add parameters from PathParams (if not already in Definitions.Parameters)
	pathParams := getPathParamsAutoComplete(schema, oasPath)
	for name, param := range pathParams {
		if _, exists := allParams[name]; !exists {
			allParams[name] = ParameterDefinition{
				In:          pathParamsType,
				Required:    true, // Path parameters are always required
				Description: param.Description,
				Content:     param.Content,
				Schema:      param.Schema,
			}
		}
	}

	// Add parameters from Querystring (if not already in Definitions.Parameters)
	for name, param := range schema.Querystring {
		if _, exists := allParams[name]; !exists {
			allParams[name] = ParameterDefinition{
				In:          queryParamType,
				Required:    param.Required, // Use Required from ParameterValue
				Description: param.Description,
				Content:     param.Content,
				Schema:      param.Schema,
			}
		}
	}

	// Add parameters from Headers (if not already in Definitions.Parameters)
	for name, param := range schema.Headers {
		if _, exists := allParams[name]; !exists {
			allParams[name] = ParameterDefinition{
				In:          headerParamType,
				Required:    param.Required, // Use Required from ParameterValue
				Description: param.Description,
				Content:     param.Content,
				Schema:      param.Schema,
			}
		}
	}

	// Add parameters from Cookies (if not already in Definitions.Parameters)
	for name, param := range schema.Cookies {
		if _, exists := allParams[name]; !exists {
			allParams[name] = ParameterDefinition{
				In:          cookieParamType,
				Required:    param.Required, // Use Required from ParameterValue
				Description: param.Description,
				Content:     param.Content,
				Schema:      param.Schema,
			}
		}
	}

	// Convert map to slice for sorting
	var sortedParamNames []string
	for name := range allParams {
		sortedParamNames = append(sortedParamNames, name)
	}
	// Sort parameters first by location, then by name for consistent order
	sort.SliceStable(sortedParamNames, func(i, j int) bool {
		paramI := allParams[sortedParamNames[i]]
		paramJ := allParams[sortedParamNames[j]]
		if paramI.In != paramJ.In {
			// Define a consistent order for 'in' values
			return paramLocationOrder[paramI.In] < paramLocationOrder[paramJ.In]
		}
		return sortedParamNames[i] < sortedParamNames[j]
	})

	// Add sorted parameters to the operation
	for _, name := range sortedParamNames {
		paramDef := allParams[name]
		param := &openapi3.Parameter{
			In:          paramDef.In,
			Name:        name,
			Required:    paramDef.Required,
			Description: paramDef.Description,
		}

		if paramDef.Content != nil {
			content, err := r.addContentToOASSchema(paramDef.Content)
			if err != nil {
				// Log or handle the error appropriately, but don't fail AddRoute for a single parameter
				continue
			}
			param.Content = content
		} else if paramDef.Schema != nil {
			schema, err := r.getSchemaFromInterface(paramDef.Schema.Value, paramDef.Schema.AllowAdditionalProperties)
			if err != nil {
				// Log or handle the error appropriately
				continue
			}
			param.Schema = schema
		}
		operation.AddParameter(param)
	}

	err := r.resolveRequestBodySchema(schema.RequestBody, operation)
	if err != nil {
		return getZero[Route](), fmt.Errorf("%w: %s", ErrRequestBody, err)
	}

	err = r.resolveResponsesSchema(schema.Responses, operation)
	if err != nil {
		return getZero[Route](), fmt.Errorf("%w: %s", ErrResponses, err)
	}

	return r.AddRawRoute(method, path, handler, operation, middleware...)
}

func (r Router[_, _, _]) resolveRequestBodySchema(bodySchema *ContentValue, operation Operation) error {
	if bodySchema == nil {
		return nil
	}
	content, err := r.addContentToOASSchema(bodySchema.Content)
	if err != nil {
		return err
	}

	requestBody := openapi3.NewRequestBody().WithContent(content)

	requestBody.WithDescription(bodySchema.Description)
	// Explicitly set required based on the ContentValue's Required field
	requestBody.Required = bodySchema.Required

	operation.AddRequestBody(requestBody)
	return nil
}

func (r Router[_, _, _]) resolveResponsesSchema(responses map[int]ContentValue, operation Operation) error {
	if responses == nil {
		operation.Responses = openapi3.NewResponses()
	}
	// Sort response status codes for consistent order
	var statusCodes []int
	for code := range responses {
		statusCodes = append(statusCodes, code)
	}
	sort.Ints(statusCodes)

	for _, statusCode := range statusCodes {
		v := responses[statusCode]
		response := openapi3.NewResponse()
		content, err := r.addContentToOASSchema(v.Content)
		if err != nil {
			return err
		}
		response = response.WithContent(content)
		response = response.WithDescription(v.Description)

		if len(v.Headers) > 0 {
			response.Headers = make(map[string]*openapi3.HeaderRef)
			// Sort header names for consistent order
			var headerNames []string
			for name := range v.Headers {
				headerNames = append(headerNames, name)
			}
			sort.Strings(headerNames)

			for _, headerName := range headerNames {
				headerDesc := v.Headers[headerName]
				header := &openapi3.Header{
					Parameter: openapi3.Parameter{
						Description: headerDesc,
						Schema: &openapi3.SchemaRef{
							Value: openapi3.NewStringSchema(),
						},
					},
				}
				response.Headers[headerName] = &openapi3.HeaderRef{
					Value: header,
				}
			}
		}

		operation.AddResponse(statusCode, response)
	}

	return nil
}

func (r Router[_, _, _]) getSchemaFromInterface(v interface{}, allowAdditionalProperties bool) (*openapi3.SchemaRef, error) {
	if v == nil {
		return &openapi3.SchemaRef{}, nil
	}

	reflector := &jsonschema.Reflector{
		DoNotReference:            false,
		AllowAdditionalProperties: allowAdditionalProperties,
		Anonymous:                 true,
	}
	if r.reflectorOptions != nil {
		reflector = &jsonschema.Reflector{
			DoNotReference:             r.reflectorOptions.DoNotReference,
			AllowAdditionalProperties:  allowAdditionalProperties,
			Anonymous:                  r.reflectorOptions.Anonymous,
			Mapper:                     r.reflectorOptions.Mapper,
			Namer:                      r.reflectorOptions.Namer,
			ExpandedStruct:             r.reflectorOptions.ExpandedStruct,
			FieldNameTag:               r.reflectorOptions.FieldNameTag,
			RequiredFromJSONSchemaTags: r.reflectorOptions.RequiredFromJSONSchemaTags,
		}
	}

	// Reflect the Go type into a jsonschema.Schema
	jsonSchema := reflector.Reflect(v)
	jsonSchema.Version = ""

	// Handle definitions first - this is where we store the full schema
	if len(jsonSchema.Definitions) > 0 {
		if r.swaggerSchema.Components == nil {
			r.swaggerSchema.Components = &openapi3.Components{}
		}
		if r.swaggerSchema.Components.Schemas == nil {
			r.swaggerSchema.Components.Schemas = make(map[string]*openapi3.SchemaRef)
		}

		// Sort definition names for consistent order
		var defNames []string
		for name := range jsonSchema.Definitions {
			defNames = append(defNames, name)
		}
		sort.Strings(defNames)

		for _, name := range defNames {
			def := jsonSchema.Definitions[name]
			// Marshal the jsonschema definition to JSON
			defData, err := json.Marshal(def)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal jsonschema definition %q: %w", name, err)
			}

			// Unmarshal the JSON definition into an openapi3.Schema
			oasSchema := openapi3.NewSchema()
			if err := oasSchema.UnmarshalJSON(defData); err != nil {
				return nil, fmt.Errorf("failed to unmarshal jsonschema definition %q into openapi3.Schema: %w", name, err)
			}

			// Determine the correct component name using the helper
			componentName := determineComponentName(def.Ref, name)

			// Only add if it doesn't exist yet
			if _, exists := r.swaggerSchema.Components.Schemas[componentName]; !exists {
				r.swaggerSchema.Components.Schemas[componentName] = &openapi3.SchemaRef{Value: oasSchema}
			}
		}
	}

	if jsonSchema.Type == "array" && jsonSchema.Definitions != nil {
		jsonSchema.Definitions = nil
	}

	// Check if the reflected schema has a $ref
	if jsonSchema.Ref != "" {
		return openapi3.NewSchemaRef(jsonSchema.Ref, nil), nil
	}

	// Marshal the jsonschema.Schema to JSON
	data, err := jsonSchema.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal jsonschema: %w", err)
	}

	// Unmarshal the main schema JSON into an openapi3.Schema
	oasSchema := openapi3.NewSchema()
	if err := oasSchema.UnmarshalJSON(data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal jsonschema JSON into openapi3.Schema: %w", err)
	}

	// For schemas that don't result in a $ref from jsonschema,
	// create a SchemaRef with the Value populated and an empty Ref.
	return openapi3.NewSchemaRef("", oasSchema), nil
}

func (r Router[_, _, _]) addContentToOASSchema(content Content) (openapi3.Content, error) {
	oasContent := openapi3.NewContent()
	// Sort content types for consistent order
	var mediaTypes []string
	for mt := range content {
		mediaTypes = append(mediaTypes, mt)
	}
	sort.Strings(mediaTypes)

	for _, k := range mediaTypes {
		v := content[k]
		var err error
		schema, err := r.getSchemaFromInterface(v.Value, v.AllowAdditionalProperties)
		if err != nil {
			return nil, err
		}
		oasContent[k] = openapi3.NewMediaType().WithSchemaRef(schema)
	}
	return oasContent, nil
}

func getPathParamsAutoComplete(schema Definitions, path string) ParameterValue {
	// If PathParams are explicitly defined, use them.
	if schema.PathParams != nil {
		return schema.PathParams
	}

	// Otherwise, auto-complete from the path string.
	autoCompletedParams := make(ParameterValue)
	re := regexp.MustCompile(`\{([^}]+)\}`)
	segments := strings.Split(path, "/")
	for _, segment := range segments {
		params := re.FindAllStringSubmatch(segment, -1)
		if len(params) == 0 {
			continue
		}
		for _, param := range params {
			autoCompletedParams[param[1]] = Parameter{
				Schema: &Schema{Value: ""}, // Default to string schema
			}
		}
	}

	// Return nil if no path parameters were found and schema.PathParams was nil
	if len(autoCompletedParams) == 0 && schema.PathParams == nil {
		return nil
	}

	return autoCompletedParams
}

func getZero[T any]() T {
	var result T
	return result
}

// determineComponentName extracts the component name from a jsonschema $ref or definition name.
// It handles different jsonschema reference formats (#/$defs/, #/definitions/, #/components/schemas/)
// and falls back to the provided name if no ref is present or recognized.
func determineComponentName(ref, name string) string {
	if ref == "" {
		return name
	}

	// Handle invalid reference format (e.g. "#/")
	if len(strings.TrimPrefix(ref, "#/")) == 0 {
		return ""
	}

	ref = strings.TrimSuffix(ref, "/")
	if strings.Contains(ref, "$defs") {
		return strings.TrimPrefix(ref, "#/$defs/")
	} else if strings.Contains(ref, "definitions") {
		return strings.TrimPrefix(ref, "#/definitions/")
	} else if strings.Contains(ref, "components/schemas") {
		return strings.TrimPrefix(ref, "#/components/schemas/")
	} else if strings.HasPrefix(ref, "#/") {
		// Local ref, but not in expected path, assume it's a component name
		return strings.TrimPrefix(ref, "#/")
	}
	return name
}
