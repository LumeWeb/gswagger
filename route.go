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

var (
	// ErrResponses is thrown if error occurs generating responses schemas.
	ErrResponses = errors.New("errors generating responses schema")
	// ErrRequestBody is thrown if error occurs generating responses schemas.
	ErrRequestBody = errors.New("errors generating request body schema")
	// ErrPathParams is thrown if error occurs generating path params schemas.
	ErrPathParams = errors.New("errors generating path parameters schema")
	// ErrQuerystring is thrown if error occurs generating querystring params schemas.
	ErrQuerystring = errors.New("errors generating querystring schema")
)

// AddRawRoute add route to router with specific method, path and handler. Add the
// router also to the openapi schema, after validating it
func (r Router[HandlerFunc, Route]) AddRawRoute(method string, routePath string, handler HandlerFunc, operation Operation) (Route, error) {
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

	// Handle, when content-type is json, the request/response marshalling? Maybe with a specific option.
	return r.router.AddRoute(method, pathWithPrefix, handler), nil
}

// Content is the type of a content.
// The key of the map define the content-type.
type Content map[string]Schema

// Schema contains the value and if properties allow additional properties.
type Schema struct {
	Value                     interface{}
	AllowAdditionalProperties bool
}

type Parameter struct {
	Content     Content
	Schema      *Schema
	Description string
	Required    bool
}

// ParameterValue is the struct containing the schema or the content information.
// If content is specified, it takes precedence.
type ParameterValue map[string]Parameter

// ParameterDefinition defines a single parameter with its location and schema
type ParameterDefinition struct {
	In          string // "query", "header", "path", "cookie"
	Required    bool
	Description string
	Content     Content
	Schema      *Schema
}

// ContentValue is the struct containing the content information.
type ContentValue struct {
	Content     Content
	Description string
	Headers     map[string]string
	Required    bool
}

type SecurityRequirements []SecurityRequirement
type SecurityRequirement map[string][]string

// Definitions of the route.
// To see how to use, refer to https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.0.3.md
type Definitions struct {
	// Specification extensions https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.0.3.md#specification-extensions
	Extensions map[string]interface{}
	// Optional field for documentation
	Tags        []string
	Summary     string
	Description string
	Deprecated  bool

	// Parameters contains all parameters (query, header, path, cookie)
	Parameters map[string]ParameterDefinition

	// PathParams contains the path parameters. If empty is autocompleted from the path
	PathParams  ParameterValue
	Querystring ParameterValue
	Headers     ParameterValue
	Cookies     ParameterValue
	RequestBody *ContentValue
	Responses   map[int]ContentValue

	Security SecurityRequirements
}

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

const (
	pathParamsType  = "path"
	queryParamType  = "query"
	headerParamType = "header"
	cookieParamType = "cookie"
	jsonType        = "application/json"
	formDataType    = "multipart/form-data"
)

// AddRoute add a route with json schema inferred by passed schema.
func (r Router[HandlerFunc, Route]) AddRoute(method string, path string, handler HandlerFunc, schema Definitions) (Route, error) {
	operation := newOperationFromDefinition(schema)

	// Process parameters from Definitions.Parameters, ensuring path params come first
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
				// Log or handle error appropriately, skipping parameter for now
				continue
			}
			param.Content = content
		} else if paramDef.Schema != nil {
			schema, err := r.getSchemaFromInterface(paramDef.Schema.Value, paramDef.Schema.AllowAdditionalProperties)
			if err != nil {
				// Log or handle error appropriately, skipping parameter for now
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

	// Add path parameters first, then others
	for _, param := range pathParams {
		operation.AddParameter(param)
	}
	for _, param := range otherParams {
		operation.AddParameter(param)
	}

	// Helper function to add parameters from older fields if they don't already exist
	addParameterIfNotExists := func(paramType string, paramConfig ParameterValue) error {
		var keys = make([]string, 0, len(paramConfig))
		for k := range paramConfig {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			// Check if a parameter with the same name and in value already exists
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
					param.Required = true // Path parameters are always required
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

	// Process parameters from older fields for backward compatibility
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

	return r.AddRawRoute(method, path, handler, operation)
}

func (r Router[_, _]) getSchemaFromInterface(v interface{}, allowAdditionalProperties bool) (*openapi3.Schema, error) {
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
	// Empty definitions. Definitions are not valid in openapi3, which use components.
	// In the future, we could add an option to fill the components in openapi spec.
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

func (r Router[_, _]) resolveRequestBodySchema(bodySchema *ContentValue, operation Operation) error {
	if bodySchema == nil {
		return nil
	}
	content, err := r.addContentToOASSchema(bodySchema.Content)
	if err != nil {
		return err
	}

	requestBody := openapi3.NewRequestBody().WithContent(content)

	requestBody.WithDescription(bodySchema.Description)
	// Only set required=true for JSON content when description is present
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

func (r Router[_, _]) resolveResponsesSchema(responses map[int]ContentValue, operation Operation) error {
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

		// Add headers to response if specified
		// Add headers to response if specified
		if len(v.Headers) > 0 {
			response.Headers = make(map[string]*openapi3.HeaderRef)
			for headerName, headerDesc := range v.Headers {
				// Create a new OpenAPI Header object
				header := &openapi3.Header{
					Parameter: openapi3.Parameter{
						Description: headerDesc,
						// Add a default string schema for headers with just a description
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

func (r Router[_, _]) resolveParameterSchema(paramType string, paramConfig ParameterValue, operation Operation) error {
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

func (r Router[_, _]) addContentToOASSchema(content Content) (openapi3.Content, error) {
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
