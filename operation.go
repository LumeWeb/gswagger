package swagger

import (
	"github.com/getkin/kin-openapi/openapi3"
)

// Operation type
type Operation struct {
	*openapi3.Operation
}

// NewOperation returns an OpenApi operation.
func NewOperation() Operation {
	return Operation{
		openapi3.NewOperation(),
	}
}

// AddRequestBody set request body into operation.
func (o *Operation) AddRequestBody(requestBody *openapi3.RequestBody) {
	o.RequestBody = &openapi3.RequestBodyRef{
		Value: requestBody,
	}
}

// AddResponse adds a response to the operation.
// If description is missing, sets an empty description (required by OpenAPI spec).
// Always uses explicit status codes - defaults to 200 OK if status is 0.
func (o *Operation) AddResponse(status int, response *openapi3.Response) {
	if o.Responses == nil {
		o.Responses = &openapi3.Responses{}
	}
	if response.Description == nil {
		response.WithDescription("")
	}
	o.Operation.AddResponse(status, response)
}

func (o *Operation) addSecurityRequirements(securityRequirements SecurityRequirements) {
	if securityRequirements != nil && o.Security == nil {
		o.Security = openapi3.NewSecurityRequirements()
	}
	for _, securityRequirement := range securityRequirements {
		o.Security.With(openapi3.SecurityRequirement(securityRequirement))
	}
}
