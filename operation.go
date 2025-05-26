package swagger

import (
	"github.com/getkin/kin-openapi/openapi3"
)

// Operation wraps an OpenAPI 3.0 operation with additional helper methods.
// It provides a more convenient interface for building OpenAPI operations
// while maintaining compatibility with the underlying openapi3.Operation.
type Operation struct {
	*openapi3.Operation
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
//     - Description
//     - Content types and schemas
//     - Headers
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
