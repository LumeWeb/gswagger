package swagger

import (
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
func (r Router[HandlerFunc, MiddlewareFunc, Route]) AddRawRoute(method string, routePath string, handler HandlerFunc, operation Operation, middleware ...MiddlewareFunc) (Route, error) {
	op := operation.Operation
	if op != nil {
		err := operation.Validate(r.context)
		if err != nil {
			return getZero[Route](), err
		}
	} else {
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
func (r Router[HandlerFunc, MiddlewareFunc, Route]) AddRoute(method string, path string, handler HandlerFunc, schema Definitions, middleware ...MiddlewareFunc) (Route, error) {
	operation := newOperationFromDefinition(schema)

	var pathParams, otherParams []*openapi3.Parameter

	for name, paramDef := range schema.Parameters {
		param := &openapi3.Parameter{
			In:          paramDef.In,
			Name:        name,
			Required:    paramDef.Required,
			Description: paramDef.Description,
		}

		if paramDef.Content != nil {
			content, err := r.addContentToOASSchema(paramDef.Content)
			if err != nil {
				continue
			}
			param.Content = content
		} else if paramDef.Schema != nil {
			schema, err := r.getSchemaFromInterface(paramDef.Schema.Value, paramDef.Schema.AllowAdditionalProperties)
			if err != nil {
				continue
			}
			param.Schema = &openapi3.SchemaRef{Value: schema}
		}

		if paramDef.In == pathParamsType {
			pathParams = append(pathParams, param)
		} else {
			otherParams = append(otherParams, param)
		}
	}

	for _, param := range pathParams {
		operation.AddParameter(param)
	}
	for _, param := range otherParams {
		operation.AddParameter(param)
	}

	addParameterIfNotExists := func(paramType string, paramConfig ParameterValue) error {
		var keys = make([]string, 0, len(paramConfig))
		for k := range paramConfig {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			exists := false
			if operation.Parameters != nil {
				for _, existingParamRef := range operation.Parameters {
					if existingParamRef != nil && existingParamRef.Value != nil {
						if existingParamRef.Value.Name == key && existingParamRef.Value.In == paramType {
							exists = true
							break
						}
					}
				}
			}

			if !exists {
				v := paramConfig[key]
				var param *openapi3.Parameter
				switch paramType {
				case pathParamsType:
					param = openapi3.NewPathParameter(key)
					param.Required = true
				case queryParamType:
					param = openapi3.NewQueryParameter(key)
				case headerParamType:
					param = openapi3.NewHeaderParameter(key)
				case cookieParamType:
					param = openapi3.NewCookieParameter(key)
				default:
					return fmt.Errorf("invalid param type")
				}

				if v.Description != "" {
					param.Description = v.Description
				}

				if v.Content != nil {
					content, err := r.addContentToOASSchema(v.Content)
					if err != nil {
						return err
					}
					param.Content = content
				} else {
					schema := &openapi3.Schema{}
					if v.Schema != nil {
						var err error
						schema, err = r.getSchemaFromInterface(v.Schema.Value, v.Schema.AllowAdditionalProperties)
						if err != nil {
							return err
						}
					}
					param.Schema = &openapi3.SchemaRef{Value: schema}
				}

				operation.AddParameter(param)
			}
		}
		return nil
	}

	oasPath := r.router.TransformPathToOasPath(path)
	err := addParameterIfNotExists(pathParamsType, getPathParamsAutoComplete(schema, oasPath))
	if err != nil {
		return getZero[Route](), fmt.Errorf("%w: %s", ErrPathParams, err)
	}

	err = addParameterIfNotExists(queryParamType, schema.Querystring)
	if err != nil {
		return getZero[Route](), fmt.Errorf("%w: %s", ErrPathParams, err)
	}

	err = addParameterIfNotExists(headerParamType, schema.Headers)
	if err != nil {
		return getZero[Route](), fmt.Errorf("%w: %s", ErrPathParams, err)
	}

	err = addParameterIfNotExists(cookieParamType, schema.Cookies)
	if err != nil {
		return getZero[Route](), fmt.Errorf("%w: %s", ErrPathParams, err)
	}

	err = r.resolveRequestBodySchema(schema.RequestBody, operation)
	if err != nil {
		return getZero[Route](), fmt.Errorf("%w: %s", ErrRequestBody, err)
	}

	err = r.resolveResponsesSchema(schema.Responses, operation)
	if err != nil {
		return getZero[Route](), fmt.Errorf("%w: %s", ErrResponses, err)
	}

	return r.AddRawRoute(method, path, handler, operation, middleware...)
}

func (r Router[_, _, _]) getSchemaFromInterface(v interface{}, allowAdditionalProperties bool) (*openapi3.Schema, error) {
	if v == nil {
		return &openapi3.Schema{}, nil
	}

	reflector := &jsonschema.Reflector{
		DoNotReference:            true,
		AllowAdditionalProperties: allowAdditionalProperties,
		Anonymous:                 true,
	}

	jsonSchema := reflector.Reflect(v)
	jsonSchema.Version = ""
	jsonSchema.Definitions = nil

	data, err := jsonSchema.MarshalJSON()
	if err != nil {
		return nil, err
	}

	schema := openapi3.NewSchema()
	err = schema.UnmarshalJSON(data)
	if err != nil {
		return nil, err
	}

	return schema, nil
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
	if bodySchema.Description != "" {
		for contentType := range bodySchema.Content {
			if contentType == jsonType {
				requestBody.Required = true
				break
			}
		}
	} else {
		requestBody.Required = bodySchema.Required
	}

	operation.AddRequestBody(requestBody)
	return nil
}

func (r Router[_, _, _]) resolveResponsesSchema(responses map[int]ContentValue, operation Operation) error {
	if responses == nil {
		operation.Responses = openapi3.NewResponses()
	}
	for statusCode, v := range responses {
		response := openapi3.NewResponse()
		content, err := r.addContentToOASSchema(v.Content)
		if err != nil {
			return err
		}
		response = response.WithContent(content)
		response = response.WithDescription(v.Description)

		if len(v.Headers) > 0 {
			response.Headers = make(map[string]*openapi3.HeaderRef)
			for headerName, headerDesc := range v.Headers {
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

func (r Router[_, _, _]) resolveParameterSchema(paramType string, paramConfig ParameterValue, operation Operation) error {
	var keys = make([]string, 0, len(paramConfig))
	for k := range paramConfig {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		v := paramConfig[key]
		var param *openapi3.Parameter
		switch paramType {
		case pathParamsType:
			param = openapi3.NewPathParameter(key)
			param.Required = true
		case queryParamType:
			param = openapi3.NewQueryParameter(key)
		case headerParamType:
			param = openapi3.NewHeaderParameter(key)
		case cookieParamType:
			param = openapi3.NewCookieParameter(key)
		default:
			return fmt.Errorf("invalid param type")
		}

		if v.Description != "" {
			param.Description = v.Description
		}

		if paramType != pathParamsType {
			param.Required = v.Required
		}

		if v.Content != nil {
			content, err := r.addContentToOASSchema(v.Content)
			if err != nil {
				return err
			}
			param.Content = content
		} else {
			schema := &openapi3.Schema{}
			if v.Schema != nil {
				var err error
				schema, err = r.getSchemaFromInterface(v.Schema.Value, v.Schema.AllowAdditionalProperties)
				if err != nil {
					return err
				}
			}
			param.Schema = &openapi3.SchemaRef{Value: schema}
		}

		operation.AddParameter(param)
	}

	return nil
}

func (r Router[_, _, _]) addContentToOASSchema(content Content) (openapi3.Content, error) {
	oasContent := openapi3.NewContent()
	for k, v := range content {
		var err error
		schema, err := r.getSchemaFromInterface(v.Value, v.AllowAdditionalProperties)
		if err != nil {
			return nil, err
		}
		oasContent[k] = openapi3.NewMediaType().WithSchema(schema)
	}
	return oasContent, nil
}

func getPathParamsAutoComplete(schema Definitions, path string) ParameterValue {
	if schema.PathParams == nil {
		re := regexp.MustCompile(`\{([^}]+)\}`)
		segments := strings.Split(path, "/")
		for _, segment := range segments {
			params := re.FindAllStringSubmatch(segment, -1)
			if len(params) == 0 {
				continue
			}
			if schema.PathParams == nil {
				schema.PathParams = make(ParameterValue)
			}
			for _, param := range params {
				schema.PathParams[param[1]] = Parameter{
					Schema: &Schema{Value: ""},
				}
			}
		}
	}
	return schema.PathParams
}

func getZero[T any]() T {
	var result T
	return result
}
