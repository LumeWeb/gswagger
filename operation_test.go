package swagger

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func getBaseSwagger(t *testing.T) *openapi3.T {
	t.Helper()

	return &openapi3.T{
		Info: &openapi3.Info{
			Title:   "test openapi title",
			Version: "test openapi version",
		},
	}
}

func getRootSchemaWithComponents(t *testing.T) *openapi3.T {
	t.Helper()
	return &openapi3.T{
		Components: &openapi3.Components{
			Schemas: map[string]*openapi3.SchemaRef{
				"UserSchema":       {Value: openapi3.NewObjectSchema()},
				"NestedSchema":     {Value: openapi3.NewIntegerSchema()},
				"ParentSchema":     {Value: openapi3.NewObjectSchema().WithPropertyRef("nested", &openapi3.SchemaRef{Ref: "#/components/schemas/NestedSchema"})},
				"ItemSchema":       {Value: openapi3.NewStringSchema()},
				"AdditionalSchema": {Value: openapi3.NewBoolSchema()},
				"AllOfSchema":      {Value: openapi3.NewFloat64Schema()},
				"AnyOfSchema":      {Value: openapi3.NewInt32Schema()},
				"OneOfSchema":      {Value: openapi3.NewInt64Schema()},
				"MyDef":            {Value: openapi3.NewStringSchema()},  // For $defs and definitions tests
				"AnotherDef":       {Value: openapi3.NewIntegerSchema()}, // For $defs and definitions tests
			},
			Responses: map[string]*openapi3.ResponseRef{
				"NotFoundResponse": {Value: openapi3.NewResponse().WithDescription("Not Found")},
				"TestResponse":     {Value: openapi3.NewResponse().WithDescription("Test Response")},
			},
			Parameters: map[string]*openapi3.ParameterRef{
				"UserIdParameter": {Value: &openapi3.Parameter{Name: "userId", In: "path"}},
				"TestParameter":   {Value: &openapi3.Parameter{Name: "testParam", In: "query"}},
			},
			RequestBodies: map[string]*openapi3.RequestBodyRef{
				"UserRequestBody": {Value: openapi3.NewRequestBody()},
				"TestRequestBody": {Value: openapi3.NewRequestBody().WithDescription("Test Body")},
			},
		},
	}
}

func TestNewOperation(t *testing.T) {
	schema := openapi3.NewObjectSchema().WithProperties(map[string]*openapi3.Schema{
		"foo": openapi3.NewStringSchema(),
		"bar": openapi3.NewIntegerSchema().WithMax(15).WithMin(5),
	})

	tests := []struct {
		name         string
		getOperation func(t *testing.T, operation Operation) Operation
		expectedJSON string
	}{
		{
			name: "add request body",
			getOperation: func(t *testing.T, operation Operation) Operation {
				requestBody := openapi3.NewRequestBody().WithJSONSchema(schema)
				operation.AddRequestBody(requestBody)
				operation.Responses = openapi3.NewResponses()
				return operation
			},
			expectedJSON: `{"info":{"title":"test openapi title","version":"test openapi version"},"openapi":"3.0.0","paths":{"/":{"post":{"requestBody":{"content":{"application/json":{"schema":{"properties":{"bar":{"maximum":15,"minimum":5,"type":"integer"},"foo":{"type":"string"}},"type":"object"}}}},"responses":{"default":{"description":""}}}}}}`,
		},
		{
			name: "add response",
			getOperation: func(t *testing.T, operation Operation) Operation {
				response := openapi3.NewResponse().WithJSONSchema(schema)
				operation.AddResponse(200, response)
				return operation
			},
			expectedJSON: `{"info":{"title":"test openapi title","version":"test openapi version"},"openapi":"3.0.0","paths":{"/":{"post":{"responses":{"200":{"content":{"application/json":{"schema":{"properties":{"bar":{"maximum":15,"minimum":5,"type":"integer"},"foo":{"type":"string"}},"type":"object"}}},"description":""}}}}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			openapi := getBaseSwagger(t)
			openapi.OpenAPI = "3.0.0"
			operation := test.getOperation(t, NewOperation())

			openapi.AddOperation("/", http.MethodPost, operation.Operation)

			data, _ := openapi.MarshalJSON()
			jsonData := string(data)
			require.JSONEq(t, test.expectedJSON, jsonData, "actual json data: %s", jsonData)
		})
	}
}

func TestResolveReferences(t *testing.T) {
	t.Run("resolve references in request body", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)

		op := Operation{openapi3.NewOperation()}
		op.RequestBody = &openapi3.RequestBodyRef{Ref: "#/components/requestBodies/TestRequestBody"}

		err := op.ResolveReferences(rootSchema)
		require.NoError(t, err)

		require.NotNil(t, op.RequestBody.Value)
		require.Equal(t, "Test Body", op.RequestBody.Value.Description)
		require.Equal(t, "#/components/requestBodies/TestRequestBody", op.RequestBody.Ref) // Ref should be kept
	})

	t.Run("resolve references in responses", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)

		op := Operation{openapi3.NewOperation()}
		op.Responses = openapi3.NewResponses()
		op.Responses.Set("200", &openapi3.ResponseRef{Ref: "#/components/responses/TestResponse"})

		err := op.ResolveReferences(rootSchema)
		require.NoError(t, err)

		respRef := op.Responses.Value("200")
		require.NotNil(t, respRef)
		require.NotNil(t, respRef.Value)
		require.Equal(t, "Test Response", *respRef.Value.Description)
		require.Equal(t, "#/components/responses/TestResponse", respRef.Ref) // Ref should be kept
	})

	t.Run("resolve references in parameters", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)

		op := Operation{openapi3.NewOperation()}
		op.Parameters = openapi3.NewParameters()
		op.Parameters = append(op.Parameters, &openapi3.ParameterRef{Ref: "#/components/parameters/TestParameter"})

		err := op.ResolveReferences(rootSchema)
		require.NoError(t, err)

		require.Len(t, op.Parameters, 1)
		paramRef := op.Parameters[0]
		require.NotNil(t, paramRef.Value)
		require.Equal(t, "testParam", paramRef.Value.Name)
		require.Equal(t, "query", paramRef.Value.In)
		require.Equal(t, "#/components/parameters/TestParameter", paramRef.Ref) // Ref should be kept
	})

	t.Run("resolve references in content schemas", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)
		// Add the test schema to components
		schema := openapi3.NewStringSchema()
		schema.Description = "Test Schema"
		registerTestSchema(rootSchema, "TestSchema", schema)

		op := Operation{openapi3.NewOperation()}
		op.RequestBody = &openapi3.RequestBodyRef{}
		op.RequestBody.Value = openapi3.NewRequestBody()
		op.RequestBody.Value.Content = openapi3.Content{
			"application/json": openapi3.NewMediaType().WithSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/TestSchema"}),
		}

		err := op.ResolveReferences(rootSchema)
		require.NoError(t, err)

		mediaType := op.RequestBody.Value.Content["application/json"]
		require.NotNil(t, mediaType)
		require.NotNil(t, mediaType.Schema)
		require.NotNil(t, mediaType.Schema.Value)
		require.Equal(t, "Test Schema", mediaType.Schema.Value.Description)
		require.Equal(t, "#/components/schemas/TestSchema", mediaType.Schema.Ref) // Ref should be converted to standard format and kept
	})

	t.Run("resolve references recursively in schemas", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)

		op := Operation{openapi3.NewOperation()}
		op.Responses = openapi3.NewResponses()
		op.Responses.Set("200", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithJSONSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/ParentSchema"})})

		err := op.ResolveReferences(rootSchema)
		require.NoError(t, err)

		respRef := op.Responses.Value("200")
		require.NotNil(t, respRef)
		require.NotNil(t, respRef.Value)
		require.NotNil(t, respRef.Value.Content["application/json"])
		schemaRef := respRef.Value.Content["application/json"].Schema
		require.NotNil(t, schemaRef)
		require.NotNil(t, schemaRef.Value)
		require.Equal(t, "#/components/schemas/ParentSchema", schemaRef.Ref) // Ref should be converted and kept

		nestedSchemaRef := schemaRef.Value.Properties["nested"]
		require.NotNil(t, nestedSchemaRef)
		require.NotNil(t, nestedSchemaRef.Value)
		require.Equal(t, "#/components/schemas/NestedSchema", nestedSchemaRef.Ref) // Ref should be converted and kept
		require.Equal(t, openapi3.TypeInteger, nestedSchemaRef.Value.Type.Slice()[0])
	})

	t.Run("handle missing components", func(t *testing.T) {
		rootSchema := &openapi3.T{} // No components defined

		op := Operation{openapi3.NewOperation()}
		op.RequestBody = &openapi3.RequestBodyRef{Ref: "#/components/requestBodies/TestRequestBody"}

		err := op.ResolveReferences(rootSchema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "requestBody: no requestBodies defined in components")
	})

	t.Run("handle missing component definition", func(t *testing.T) {
		rootSchema := &openapi3.T{
			Components: &openapi3.Components{
				RequestBodies: map[string]*openapi3.RequestBodyRef{}, // Empty map
			},
		}

		op := Operation{openapi3.NewOperation()}
		op.RequestBody = &openapi3.RequestBodyRef{Ref: "#/components/requestBodies/TestRequestBody"}

		err := op.ResolveReferences(rootSchema)
		require.Error(t, err)
		require.Contains(t, err.Error(), `requestBody: requestBody "TestRequestBody" not found`)
	})

	t.Run("handle invalid reference format", func(t *testing.T) {
		rootSchema := &openapi3.T{}

		op := Operation{openapi3.NewOperation()}
		op.RequestBody = &openapi3.RequestBodyRef{Ref: "#/invalid"}

		err := op.ResolveReferences(rootSchema)
		require.Error(t, err)
		require.Contains(t, err.Error(), `requestBody: invalid reference format: "#/invalid"`)
	})

	t.Run("handle non-component reference", func(t *testing.T) {
		rootSchema := &openapi3.T{}

		op := Operation{openapi3.NewOperation()}
		op.RequestBody = &openapi3.RequestBodyRef{Ref: "/some/path"}

		err := op.ResolveReferences(rootSchema)
		require.Error(t, err)
		require.Contains(t, err.Error(), `requestBody: reference "/some/path" must point to a component`)
	})

	t.Run("handle jsonschema $defs reference", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)

		op := Operation{openapi3.NewOperation()}
		op.RequestBody = &openapi3.RequestBodyRef{}
		op.RequestBody.Value = openapi3.NewRequestBody()
		op.RequestBody.Value.Content = openapi3.Content{
			"application/json": openapi3.NewMediaType().WithSchemaRef(&openapi3.SchemaRef{Ref: "#/$defs/MyDef"}),
		}

		err := op.ResolveReferences(rootSchema)
		require.NoError(t, err)

		mediaType := op.RequestBody.Value.Content["application/json"]
		require.NotNil(t, mediaType)
		require.NotNil(t, mediaType.Schema)
		require.NotNil(t, mediaType.Schema.Value)
		require.Equal(t, "#/components/schemas/MyDef", mediaType.Schema.Ref) // Ref should be converted and kept
		require.Equal(t, openapi3.TypeString, mediaType.Schema.Value.Type.Slice()[0])
	})

	t.Run("handle jsonschema definitions reference", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)

		op := Operation{openapi3.NewOperation()}
		op.RequestBody = &openapi3.RequestBodyRef{}
		op.RequestBody.Value = openapi3.NewRequestBody()
		op.RequestBody.Value.Content = openapi3.Content{
			"application/json": openapi3.NewMediaType().WithSchemaRef(&openapi3.SchemaRef{Ref: "#/definitions/AnotherDef"}),
		}

		err := op.ResolveReferences(rootSchema)
		require.NoError(t, err)

		mediaType := op.RequestBody.Value.Content["application/json"]
		require.NotNil(t, mediaType)
		require.NotNil(t, mediaType.Schema)
		require.NotNil(t, mediaType.Schema.Value)
		require.Equal(t, "#/components/schemas/AnotherDef", mediaType.Schema.Ref) // Ref should be converted and kept
		require.Equal(t, openapi3.TypeInteger, mediaType.Schema.Value.Type.Slice()[0])
	})

	t.Run("handle schema reference without component name", func(t *testing.T) {
		rootSchema := &openapi3.T{}

		op := Operation{openapi3.NewOperation()}
		op.RequestBody = &openapi3.RequestBodyRef{}
		op.RequestBody.Value = openapi3.NewRequestBody()
		op.RequestBody.Value.Content = openapi3.Content{
			"application/json": openapi3.NewMediaType().WithSchemaRef(&openapi3.SchemaRef{Ref: "#/"}), // Invalid ref
		}

		err := op.ResolveReferences(rootSchema)
		require.Error(t, err)
		require.Contains(t, err.Error(), `requestBody content "application/json": could not determine component name from reference "#/"`)
	})
}

func TestMakeComponentRef(t *testing.T) {
	tests := []struct {
		name          string
		componentName string
		expectedRef   string
	}{
		{"simple name", "User", "#/components/schemas/User"},
		{"name with slash", "User/Profile", "#/components/schemas/User/Profile"},
		{"empty name", "", "#/components/schemas/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualRef := makeComponentRef(tt.componentName)
			require.Equal(t, tt.expectedRef, actualRef)
		})
	}
}

func TestConvertSchemaRefToStandardFormat(t *testing.T) {
	tests := []struct {
		name          string
		inputRef      string
		expectedRef   string
		expectError   bool
		errorContains string
	}{
		{"standard components ref", "#/components/schemas/User", "#/components/schemas/User", false, ""},
		{"jsonschema $defs ref", "#/$defs/Product", "#/components/schemas/Product", false, ""},
		{"jsonschema definitions ref", "#/definitions/Order", "#/components/schemas/Order", false, ""},
		{"local ref (treated as component)", "#/MyComponent", "#/components/schemas/MyComponent", false, ""},
		{"invalid ref format", "#/", "", true, "could not determine component name from reference"},
		{"empty ref", "", "", true, "could not determine component name from reference"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schemaRef := &openapi3.SchemaRef{Ref: tt.inputRef}
			err := convertSchemaRefToStandardFormat(schemaRef, "test context", "application/json")

			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedRef, schemaRef.Ref)
			}
		})
	}
}

func TestProcessSchemaRefs(t *testing.T) {
	t.Run("recursively resolves references in properties", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)
		parentSchema := openapi3.NewObjectSchema().WithPropertyRef("nested", &openapi3.SchemaRef{Ref: "#/components/schemas/NestedSchema"})

		err := processSchemaRefs(rootSchema, parentSchema)
		require.NoError(t, err)

		nestedSchemaRef := parentSchema.Properties["nested"]
		require.NotNil(t, nestedSchemaRef)
		require.NotNil(t, nestedSchemaRef.Value)
		require.Equal(t, "#/components/schemas/NestedSchema", nestedSchemaRef.Ref) // Ref should be converted and kept
		require.Equal(t, openapi3.TypeInteger, nestedSchemaRef.Value.Type.Slice()[0])
	})

	t.Run("recursively resolves references in array items", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)
		// Corrected: Create a new Schema and assign the SchemaRef to its Items field
		arraySchema := openapi3.NewArraySchema()
		arraySchema.Items = &openapi3.SchemaRef{Ref: "#/components/schemas/ItemSchema"}

		err := processSchemaRefs(rootSchema, arraySchema)
		require.NoError(t, err)

		itemSchemaRef := arraySchema.Items
		require.NotNil(t, itemSchemaRef)
		require.NotNil(t, itemSchemaRef.Value)
		require.Equal(t, "#/components/schemas/ItemSchema", itemSchemaRef.Ref) // Ref should be converted and kept
		require.Equal(t, openapi3.TypeString, itemSchemaRef.Value.Type.Slice()[0])
	})

	t.Run("recursively resolves references in additional properties", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)
		// Standard structure: SchemaRef directly in AdditionalProperties
		objectSchema := openapi3.NewObjectSchema().WithAdditionalProperties(nil)
		objectSchema.AdditionalProperties.Schema = openapi3.NewSchemaRef("#/components/schemas/AdditionalSchema", nil)

		err := processSchemaRefs(rootSchema, objectSchema)
		require.NoError(t, err)

		// Access the SchemaRef at the standard level
		additionalSchemaRef := objectSchema.AdditionalProperties.Schema

		require.NotNil(t, additionalSchemaRef)
		require.NotNil(t, additionalSchemaRef.Value)
		// Assert that the Ref is still the original reference string
		require.Equal(t, "#/components/schemas/AdditionalSchema", additionalSchemaRef.Ref)
		// Assert that the Value has been populated with the resolved schema
		require.Equal(t, openapi3.TypeBoolean, additionalSchemaRef.Value.Type.Slice()[0])
	})

	t.Run("recursively resolves references in allOf", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)
		allOfSchema := openapi3.NewAllOfSchema()
		allOfSchema.AllOf = append(allOfSchema.AllOf, &openapi3.SchemaRef{Ref: "#/components/schemas/AllOfSchema"})

		err := processSchemaRefs(rootSchema, allOfSchema)
		require.NoError(t, err)

		allOfRef := allOfSchema.AllOf[0]
		require.NotNil(t, allOfRef)
		require.NotNil(t, allOfRef.Value)
		require.Equal(t, "#/components/schemas/AllOfSchema", allOfRef.Ref) // Ref should be converted and kept
		require.Equal(t, openapi3.TypeNumber, allOfRef.Value.Type.Slice()[0])
	})

	t.Run("recursively resolves references in anyOf", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)
		anyOfSchema := openapi3.NewAnyOfSchema()
		anyOfSchema.AnyOf = append(anyOfSchema.AnyOf, &openapi3.SchemaRef{Ref: "#/components/schemas/AnyOfSchema"})

		err := processSchemaRefs(rootSchema, anyOfSchema)
		require.NoError(t, err)

		anyOfRef := anyOfSchema.AnyOf[0]
		require.NotNil(t, anyOfRef)
		require.NotNil(t, anyOfRef.Value)
		require.Equal(t, "#/components/schemas/AnyOfSchema", anyOfRef.Ref) // Ref should be converted and kept
		require.Equal(t, openapi3.TypeInteger, anyOfRef.Value.Type.Slice()[0])
		require.Equal(t, "int32", anyOfRef.Value.Format)
	})

	t.Run("recursively resolves references in oneOf", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)
		oneOfSchema := openapi3.NewOneOfSchema()
		oneOfSchema.OneOf = append(oneOfSchema.OneOf, &openapi3.SchemaRef{Ref: "#/components/schemas/OneOfSchema"})

		err := processSchemaRefs(rootSchema, oneOfSchema)
		require.NoError(t, err)

		oneOfRef := oneOfSchema.OneOf[0]
		require.NotNil(t, oneOfRef)
		require.NotNil(t, oneOfRef.Value)
		require.Equal(t, "#/components/schemas/OneOfSchema", oneOfRef.Ref) // Ref should be converted and kept
		require.Equal(t, openapi3.TypeInteger, oneOfRef.Value.Type.Slice()[0])
		require.Equal(t, "int64", oneOfRef.Value.Format)
	})

	t.Run("handle missing schema component during recursion", func(t *testing.T) {
		rootSchema := &openapi3.T{
			Components: &openapi3.Components{
				Schemas: map[string]*openapi3.SchemaRef{}, // Empty map
			},
		}
		parentSchema := openapi3.NewObjectSchema().WithPropertyRef("nested", &openapi3.SchemaRef{Ref: "#/components/schemas/MissingSchema"})

		err := processSchemaRefs(rootSchema, parentSchema)
		require.Error(t, err)
		require.Contains(t, err.Error(), `schema property "#/components/schemas/MissingSchema": schema "MissingSchema" not found`)
	})
}

func TestResolveComponentRef(t *testing.T) {
	rootSchema := getRootSchemaWithComponents(t)

	tests := []struct {
		name          string
		ref           string
		expectedType  string
		expectError   bool
		errorContains string
	}{
		{"schema ref", "#/components/schemas/UserSchema", "*openapi3.Schema", false, ""},
		{"response ref", "#/components/responses/NotFoundResponse", "*openapi3.Response", false, ""},
		{"parameter ref", "#/components/parameters/UserIdParameter", "*openapi3.Parameter", false, ""},
		{"request body ref", "#/components/requestBodies/UserRequestBody", "*openapi3.RequestBody", false, ""},
		{"jsonschema $defs ref", "#/$defs/UserSchema", "*openapi3.Schema", false, ""},
		{"jsonschema definitions ref", "#/definitions/UserSchema", "*openapi3.Schema", false, ""},
		{"missing schema", "#/components/schemas/MissingSchema", "", true, `schema "MissingSchema" not found`},
		{"missing response", "#/components/responses/MissingResponse", "", true, `response "MissingResponse" not found`},
		{"missing parameter", "#/components/parameters/MissingParameter", "", true, `parameter "MissingParameter" not found`},
		{"missing request body", "#/components/requestBodies/MissingRequestBody", "", true, `requestBody "MissingRequestBody" not found`},
		{"invalid component type", "#/components/invalid/MyComponent", "", true, `unsupported component type: "invalid"`},
		{"invalid ref format", "#/components", "", true, "invalid reference format"},
		{"non-component ref", "/some/path", "", true, `reference "/some/path" must point to a component`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := resolveComponentRef(rootSchema, tt.ref)

			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorContains)
				require.Nil(t, resolved)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resolved)
				require.Equal(t, tt.expectedType, fmt.Sprintf("%T", resolved))
			}
		})
	}
}

func TestResolveRequestBodyRefs(t *testing.T) {
	t.Run("resolves request body reference", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)
		// Add the expected request body component
		reqBody := openapi3.NewRequestBody()
		reqBody.Description = "Test Body"
		registerTestRequestBody(rootSchema, "TestBody", reqBody)

		op := Operation{openapi3.NewOperation()}
		op.RequestBody = &openapi3.RequestBodyRef{Ref: "#/components/requestBodies/TestBody"}

		err := op.resolveRequestBodyRefs(rootSchema)
		require.NoError(t, err)
		require.NotNil(t, op.RequestBody.Value)
		require.Equal(t, "Test Body", op.RequestBody.Value.Description)
		require.Equal(t, "#/components/requestBodies/TestBody", op.RequestBody.Ref) // Ref should be kept
	})

	t.Run("handles nil request body", func(t *testing.T) {
		rootSchema := &openapi3.T{}
		op := Operation{openapi3.NewOperation()}
		op.RequestBody = nil

		err := op.resolveRequestBodyRefs(rootSchema)
		require.NoError(t, err)
		require.Nil(t, op.RequestBody)
	})

	t.Run("handles request body without reference", func(t *testing.T) {
		rootSchema := &openapi3.T{}
		op := Operation{openapi3.NewOperation()}
		op.RequestBody = &openapi3.RequestBodyRef{Value: openapi3.NewRequestBody().WithDescription("Direct Body")}

		err := op.resolveRequestBodyRefs(rootSchema)
		require.NoError(t, err)
		require.NotNil(t, op.RequestBody.Value)
		require.Equal(t, "Direct Body", op.RequestBody.Value.Description)
		require.Empty(t, op.RequestBody.Ref) // Ref should remain empty if it was initially empty
	})

	t.Run("handles missing request body component", func(t *testing.T) {
		rootSchema := &openapi3.T{
			Components: &openapi3.Components{
				RequestBodies: map[string]*openapi3.RequestBodyRef{},
			},
		}
		op := Operation{openapi3.NewOperation()}
		op.RequestBody = &openapi3.RequestBodyRef{Ref: "#/components/requestBodies/MissingBody"}

		err := op.resolveRequestBodyRefs(rootSchema)
		require.Error(t, err)
		require.Contains(t, err.Error(), `requestBody: requestBody "MissingBody" not found`)
	})

	t.Run("handles invalid request body reference format", func(t *testing.T) {
		rootSchema := &openapi3.T{}
		op := Operation{openapi3.NewOperation()}
		op.RequestBody = &openapi3.RequestBodyRef{Ref: "#/invalid/TestBody"}

		err := op.resolveRequestBodyRefs(rootSchema)
		require.Error(t, err)
		require.Contains(t, err.Error(), `requestBody: invalid reference format: "#/invalid/TestBody"`)
	})

	t.Run("resolves references within request body content", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)
		// Add the expected schema component
		registerTestSchema(rootSchema, "TestSchema", openapi3.NewStringSchema())

		op := Operation{openapi3.NewOperation()}
		op.RequestBody = &openapi3.RequestBodyRef{
			Value: openapi3.NewRequestBody().WithContent(openapi3.Content{
				"application/json": openapi3.NewMediaType().WithSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/TestSchema"}),
			}),
		}

		err := op.resolveRequestBodyRefs(rootSchema)
		require.NoError(t, err)
		require.NotNil(t, op.RequestBody.Value)
		mediaType := op.RequestBody.Value.Content["application/json"]
		require.NotNil(t, mediaType)
		require.NotNil(t, mediaType.Schema)
		require.NotNil(t, mediaType.Schema.Value)
		require.Equal(t, openapi3.TypeString, mediaType.Schema.Value.Type.Slice()[0])
		require.Equal(t, "#/components/schemas/TestSchema", mediaType.Schema.Ref) // Ref should be converted and kept
	})
}

func TestResolveResponseRefs(t *testing.T) {
	t.Run("resolves response reference", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)
		op := Operation{openapi3.NewOperation()}
		op.Responses = openapi3.NewResponses()
		op.Responses.Set("200", &openapi3.ResponseRef{Ref: "#/components/responses/TestResponse"})

		err := op.resolveResponseRefs(rootSchema)
		require.NoError(t, err)
		respRef := op.Responses.Value("200")
		require.NotNil(t, respRef)
		require.NotNil(t, respRef.Value)
		require.Equal(t, "Test Response", *respRef.Value.Description)
		require.Equal(t, "#/components/responses/TestResponse", respRef.Ref) // Ref should be kept
	})

	t.Run("handles nil responses", func(t *testing.T) {
		rootSchema := &openapi3.T{}
		op := Operation{openapi3.NewOperation()}
		op.Responses = nil

		err := op.resolveResponseRefs(rootSchema)
		require.NoError(t, err)
		require.Nil(t, op.Responses)
	})

	t.Run("handles response without reference", func(t *testing.T) {
		rootSchema := &openapi3.T{}
		op := Operation{openapi3.NewOperation()}
		op.Responses = openapi3.NewResponses()
		op.Responses.Set("200", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("Direct Response")})

		err := op.resolveResponseRefs(rootSchema)
		require.NoError(t, err)
		respRef := op.Responses.Value("200")
		require.NotNil(t, respRef)
		require.NotNil(t, respRef.Value)
		require.Equal(t, "Direct Response", *respRef.Value.Description)
		require.Empty(t, respRef.Ref) // Ref should remain empty if it was initially empty
	})

	t.Run("handles missing response component", func(t *testing.T) {
		rootSchema := &openapi3.T{
			Components: &openapi3.Components{
				Responses: map[string]*openapi3.ResponseRef{},
			},
		}
		op := Operation{openapi3.NewOperation()}
		op.Responses = openapi3.NewResponses()
		op.Responses.Set("404", &openapi3.ResponseRef{Ref: "#/components/responses/MissingResponse"})

		err := op.resolveResponseRefs(rootSchema)
		require.Error(t, err)
		require.Contains(t, err.Error(), `response 404: response "MissingResponse" not found`)
	})

	t.Run("handles invalid response reference format", func(t *testing.T) {
		rootSchema := &openapi3.T{}
		op := Operation{openapi3.NewOperation()}
		op.Responses = openapi3.NewResponses()
		op.Responses.Set("500", &openapi3.ResponseRef{Ref: "#/invalid/TestResponse"})

		err := op.resolveResponseRefs(rootSchema)
		require.Error(t, err)
		require.Contains(t, err.Error(), `response 500: invalid reference format: "#/invalid/TestResponse"`)
	})

	t.Run("resolves references within response content", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)
		op := Operation{openapi3.NewOperation()}
		op.Responses = openapi3.NewResponses()
		registerTestSchema(rootSchema, "TestSchema", openapi3.NewStringSchema())

		op.Responses.Set("200", &openapi3.ResponseRef{
			Value: openapi3.NewResponse().WithDescription("").WithContent(openapi3.Content{
				"application/json": openapi3.NewMediaType().WithSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/TestSchema"}),
			}),
		})

		err := op.resolveResponseRefs(rootSchema)
		require.NoError(t, err)
		respRef := op.Responses.Value("200")
		require.NotNil(t, respRef)
		require.NotNil(t, respRef.Value)
		mediaType := respRef.Value.Content["application/json"]
		require.NotNil(t, mediaType)
		require.NotNil(t, mediaType.Schema)
		require.NotNil(t, mediaType.Schema.Value)
		require.Equal(t, openapi3.TypeString, mediaType.Schema.Value.Type.Slice()[0])
		require.Equal(t, "#/components/schemas/TestSchema", mediaType.Schema.Ref) // Ref should be converted and kept
	})
}

func TestResolveParameterRefs(t *testing.T) {
	t.Run("resolves parameter reference", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)
		op := Operation{openapi3.NewOperation()}
		op.Parameters = openapi3.NewParameters()
		op.Parameters = append(op.Parameters, &openapi3.ParameterRef{Ref: "#/components/parameters/TestParameter"})

		err := op.resolveParameterRefs(rootSchema)
		require.NoError(t, err)
		require.Len(t, op.Parameters, 1)
		paramRef := op.Parameters[0]
		require.NotNil(t, paramRef.Value)
		require.Equal(t, "testParam", paramRef.Value.Name)
		require.Equal(t, "query", paramRef.Value.In)
		require.Equal(t, "#/components/parameters/TestParameter", paramRef.Ref) // Ref should be kept
	})

	t.Run("handles nil parameters", func(t *testing.T) {
		rootSchema := &openapi3.T{}
		op := Operation{openapi3.NewOperation()}
		op.Parameters = nil

		err := op.resolveParameterRefs(rootSchema)
		require.NoError(t, err)
		require.Nil(t, op.Parameters)
	})

	t.Run("handles parameter without reference", func(t *testing.T) {
		rootSchema := &openapi3.T{}
		op := Operation{openapi3.NewOperation()}
		op.Parameters = openapi3.NewParameters()
		op.Parameters = append(op.Parameters, &openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "directParam", In: "header"}})

		err := op.resolveParameterRefs(rootSchema)
		require.NoError(t, err)
		require.Len(t, op.Parameters, 1)
		paramRef := op.Parameters[0]
		require.NotNil(t, paramRef.Value)
		require.Equal(t, "directParam", paramRef.Value.Name)
		require.Equal(t, "header", paramRef.Value.In)
		require.Empty(t, paramRef.Ref) // Ref should remain empty if it was initially empty
	})

	t.Run("handles missing parameter component", func(t *testing.T) {
		rootSchema := &openapi3.T{
			Components: &openapi3.Components{
				Parameters: map[string]*openapi3.ParameterRef{},
			},
		}
		op := Operation{openapi3.NewOperation()}
		op.Parameters = openapi3.NewParameters()
		op.Parameters = append(op.Parameters, &openapi3.ParameterRef{Ref: "#/components/parameters/MissingParam"})

		err := op.resolveParameterRefs(rootSchema)
		require.Error(t, err)
		require.Contains(t, err.Error(), `parameter "#/components/parameters/MissingParam": parameter "MissingParam" not found`)
	})

	t.Run("handles invalid parameter reference format", func(t *testing.T) {
		rootSchema := &openapi3.T{
			Components: &openapi3.Components{
				Schemas: map[string]*openapi3.SchemaRef{}, // Empty map to trigger "no schemas" error
			},
		}
		op := Operation{openapi3.NewOperation()}
		op.Parameters = openapi3.NewParameters()
		op.Parameters = append(op.Parameters, &openapi3.ParameterRef{Ref: "#/invalid/TestParam"})

		err := op.resolveParameterRefs(rootSchema)
		require.Error(t, err)
		require.Contains(t, err.Error(), `parameter "#/invalid/TestParam": invalid reference format: "#/invalid/TestParam"`)
	})

	t.Run("resolves references within parameter content", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)
		// Add TestSchema to components
		registerTestSchema(rootSchema, "TestSchema", openapi3.NewStringSchema())
		op := Operation{openapi3.NewOperation()}
		op.Parameters = openapi3.NewParameters()
		op.Parameters = append(op.Parameters, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name: "paramWithContent",
				In:   "query",
				Content: openapi3.Content{
					"application/json": openapi3.NewMediaType().WithSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/TestSchema"}),
				},
			},
		})

		err := op.resolveParameterRefs(rootSchema)
		require.NoError(t, err)
		require.Len(t, op.Parameters, 1)
		paramRef := op.Parameters[0]
		require.NotNil(t, paramRef.Value)
		mediaType := paramRef.Value.Content["application/json"]
		require.NotNil(t, mediaType)
		require.NotNil(t, mediaType.Schema)
		require.NotNil(t, mediaType.Schema.Value)
		require.Equal(t, openapi3.TypeString, mediaType.Schema.Value.Type.Slice()[0])
		require.Equal(t, "#/components/schemas/TestSchema", mediaType.Schema.Ref) // Ref should be converted and kept
	})

	t.Run("resolves references within parameter schema", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)
		registerTestSchema(rootSchema, "TestSchema", openapi3.NewStringSchema())
		op := Operation{openapi3.NewOperation()}
		op.Parameters = openapi3.NewParameters()
		op.Parameters = append(op.Parameters, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:   "paramWithSchema",
				In:     "query",
				Schema: &openapi3.SchemaRef{Ref: "#/components/schemas/TestSchema"},
			},
		})

		err := op.resolveParameterRefs(rootSchema)
		require.NoError(t, err)
		require.Len(t, op.Parameters, 1)
		paramRef := op.Parameters[0]
		require.NotNil(t, paramRef.Value)
		require.NotNil(t, paramRef.Value.Schema)
		require.NotNil(t, paramRef.Value.Schema.Value)
		require.Equal(t, openapi3.TypeString, paramRef.Value.Schema.Value.Type.Slice()[0])
		require.Equal(t, "#/components/schemas/TestSchema", paramRef.Value.Schema.Ref) // Ref should be converted and kept
	})
}

func TestResolveContentRefs(t *testing.T) {
	t.Run("resolves schema references in content", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)
		registerTestSchema(rootSchema, "TestSchema", openapi3.NewStringSchema())
		content := openapi3.Content{
			"application/json": openapi3.NewMediaType().WithSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/TestSchema"}),
			"text/plain":       openapi3.NewMediaType().WithSchemaRef(&openapi3.SchemaRef{Ref: "#/$defs/MyDef"}), // Test jsonschema ref
		}

		err := resolveContentRefs(rootSchema, content, "test context")
		require.NoError(t, err)

		jsonMediaType := content["application/json"]
		require.NotNil(t, jsonMediaType)
		require.NotNil(t, jsonMediaType.Schema)
		require.NotNil(t, jsonMediaType.Schema.Value)
		require.Equal(t, openapi3.TypeString, jsonMediaType.Schema.Value.Type.Slice()[0])
		require.Equal(t, "#/components/schemas/TestSchema", jsonMediaType.Schema.Ref) // Ref should be converted and kept

		textMediaType := content["text/plain"]
		require.NotNil(t, textMediaType)
		require.NotNil(t, textMediaType.Schema)
		require.NotNil(t, textMediaType.Schema.Value)
		require.Equal(t, openapi3.TypeString, textMediaType.Schema.Value.Type.Slice()[0])
		require.Equal(t, "#/components/schemas/MyDef", textMediaType.Schema.Ref) // Ref should be converted and kept
	})

	t.Run("handles content without schema references", func(t *testing.T) {
		rootSchema := &openapi3.T{}
		content := openapi3.Content{
			"application/json": openapi3.NewMediaType().WithSchema(openapi3.NewObjectSchema()),
		}

		err := resolveContentRefs(rootSchema, content, "test context")
		require.NoError(t, err)
		require.NotNil(t, content["application/json"].Schema.Value)
		require.Empty(t, content["application/json"].Schema.Ref) // Ref should remain empty if it was initially empty
	})

	t.Run("handles missing schema component in content", func(t *testing.T) {
		rootSchema := &openapi3.T{
			Components: &openapi3.Components{
				Schemas: map[string]*openapi3.SchemaRef{},
			},
		}
		content := openapi3.Content{
			"application/json": openapi3.NewMediaType().WithSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/MissingSchema"}),
		}

		err := resolveContentRefs(rootSchema, content, "test context")
		require.Error(t, err)
		require.Contains(t, err.Error(), `test context content "application/json" schema: schema "MissingSchema" not found`)
	})

	t.Run("handles invalid schema reference format in content", func(t *testing.T) {
		rootSchema := &openapi3.T{}
		content := openapi3.Content{
			"application/json": openapi3.NewMediaType().WithSchemaRef(&openapi3.SchemaRef{Ref: "#/"}),
		}

		err := resolveContentRefs(rootSchema, content, "test context")
		require.Error(t, err)
		require.Contains(t, err.Error(), `test context content "application/json": could not determine component name from reference "#/"`)
	})
}

func TestResolveReferences_NestedContentSchemaRefs(t *testing.T) {
	rootSchema := &openapi3.T{
		Components: &openapi3.Components{
			Schemas: map[string]*openapi3.SchemaRef{
				"ParentSchema": {
					Value: openapi3.NewObjectSchema().WithPropertyRef("child",
						&openapi3.SchemaRef{Ref: "#/components/schemas/ChildSchema"}),
				},
				"ChildSchema": {
					Value: openapi3.NewStringSchema(),
				},
			},
		},
	}

	op := Operation{openapi3.NewOperation()}
	op.RequestBody = &openapi3.RequestBodyRef{}
	op.RequestBody.Value = openapi3.NewRequestBody()
	op.RequestBody.Value.Content = openapi3.Content{
		"application/json": openapi3.NewMediaType().WithSchemaRef(
			&openapi3.SchemaRef{Ref: "#/components/schemas/ParentSchema"}),
	}

	err := op.ResolveReferences(rootSchema)
	require.NoError(t, err)

	mediaType := op.RequestBody.Value.Content["application/json"]
	require.NotNil(t, mediaType.Schema.Value)

	// Verify parent schema was resolved
	require.Equal(t, "#/components/schemas/ParentSchema", mediaType.Schema.Ref)

	// Verify child schema reference was resolved
	childRef := mediaType.Schema.Value.Properties["child"]
	require.NotNil(t, childRef)
	require.NotNil(t, childRef.Value)
	require.Equal(t, "#/components/schemas/ChildSchema", childRef.Ref)
	require.Equal(t, openapi3.TypeString, childRef.Value.Type.Slice()[0])
}

func TestResolveAllComponents(t *testing.T) {
	t.Run("resolves all component types", func(t *testing.T) {
		rootSchema := getRootSchemaWithComponents(t)

		// Add some nested references to test recursive resolution
		registerTestSchema(rootSchema, "NestedSchema", openapi3.NewObjectSchema().WithPropertyRef("child", &openapi3.SchemaRef{Ref: "#/components/schemas/ChildSchema"}))
		registerTestSchema(rootSchema, "ChildSchema", openapi3.NewStringSchema())

		// Add parameter with schema reference
		if rootSchema.Components.Parameters == nil {
			rootSchema.Components.Parameters = make(map[string]*openapi3.ParameterRef)
		}
		rootSchema.Components.Parameters["ParamWithRef"] = &openapi3.ParameterRef{
			Ref: "",
			Value: &openapi3.Parameter{
				Name: "param",
				In:   "query",
				Schema: &openapi3.SchemaRef{
					Ref: "#/components/schemas/NestedSchema",
				},
			},
		}

		// Add request body with content reference
		if rootSchema.Components.RequestBodies == nil {
			rootSchema.Components.RequestBodies = make(map[string]*openapi3.RequestBodyRef)
		}
		rootSchema.Components.RequestBodies["BodyWithRef"] = &openapi3.RequestBodyRef{
			Ref: "",
			Value: &openapi3.RequestBody{
				Content: openapi3.Content{
					"application/json": openapi3.NewMediaType().WithSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/NestedSchema"}),
				},
			},
		}

		// Add response with content reference
		if rootSchema.Components.Responses == nil {
			rootSchema.Components.Responses = make(map[string]*openapi3.ResponseRef)
		}
		rootSchema.Components.Responses["ResponseWithRef"] = &openapi3.ResponseRef{
			Ref: "",
			Value: &openapi3.Response{
				Content: openapi3.Content{
					"application/json": openapi3.NewMediaType().WithSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/NestedSchema"}),
				},
			},
		}

		err := ResolveAllComponents(rootSchema)
		require.NoError(t, err)

		// Verify schemas were resolved
		schema := rootSchema.Components.Schemas["NestedSchema"]
		require.NotNil(t, schema.Value)
		require.NotNil(t, schema.Value.Properties["child"].Value)
		require.Equal(t, openapi3.TypeString, schema.Value.Properties["child"].Value.Type.Slice()[0])

		// Verify parameter was resolved
		param := rootSchema.Components.Parameters["ParamWithRef"]
		require.NotNil(t, param.Value.Schema.Value)
		require.Equal(t, openapi3.TypeObject, param.Value.Schema.Value.Type.Slice()[0])

		// Verify request body was resolved
		body := rootSchema.Components.RequestBodies["BodyWithRef"]
		require.NotNil(t, body.Value.Content["application/json"].Schema.Value)
		require.Equal(t, openapi3.TypeObject, body.Value.Content["application/json"].Schema.Value.Type.Slice()[0])

		// Verify response was resolved
		resp := rootSchema.Components.Responses["ResponseWithRef"]
		require.NotNil(t, resp.Value.Content["application/json"].Schema.Value)
		require.Equal(t, openapi3.TypeObject, resp.Value.Content["application/json"].Schema.Value.Type.Slice()[0])
	})

	t.Run("handles empty components", func(t *testing.T) {
		rootSchema := &openapi3.T{
			Components: &openapi3.Components{},
		}

		err := ResolveAllComponents(rootSchema)
		require.NoError(t, err)
	})

	t.Run("handles nil components", func(t *testing.T) {
		rootSchema := &openapi3.T{}

		err := ResolveAllComponents(rootSchema)
		require.NoError(t, err)
	})

	t.Run("handles missing referenced components", func(t *testing.T) {
		rootSchema := &openapi3.T{
			Components: &openapi3.Components{
				Schemas: map[string]*openapi3.SchemaRef{
					"BadRef": {
						Ref: "#/components/schemas/MissingSchema",
					},
				},
			},
		}

		err := ResolveAllComponents(rootSchema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "schemas \"BadRef\": schema \"MissingSchema\" not found")
	})
}

func TestDetermineComponentName(t *testing.T) {
	tests := []struct {
		name         string
		ref          string
		inputName    string
		expectedName string
	}{
		{"standard components ref", "#/components/schemas/User", "User", "User"},
		{"jsonschema $defs ref", "#/$defs/Product", "Product", "Product"},
		{"jsonschema definitions ref", "#/definitions/Order", "Order", "Order"},
		{"local ref (treated as component)", "#/MyComponent", "MyComponent", "MyComponent"},
		{"ref with slash", "#/components/schemas/User/Profile", "User/Profile", "User/Profile"},
		{"empty ref, with name", "", "FallbackName", "FallbackName"},
		{"empty ref, empty name", "", "", ""},
		{"invalid ref format", "#/", "Invalid", ""}, // Should return empty string as name cannot be determined
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualName := determineComponentName(tt.ref, tt.inputName)
			require.Equal(t, tt.expectedName, actualName)
		})
	}
}

// registerTestSchema adds a schema to the root schema's components for testing
func registerTestSchema(rootSchema *openapi3.T, name string, schema *openapi3.Schema) {
	if rootSchema.Components == nil {
		rootSchema.Components = &openapi3.Components{}
	}
	if rootSchema.Components.Schemas == nil {
		rootSchema.Components.Schemas = make(map[string]*openapi3.SchemaRef)
	}
	rootSchema.Components.Schemas[name] = &openapi3.SchemaRef{
		Value: schema,
	}
}

// registerTestRequestBody adds a request body to the root schema's components for testing
func registerTestRequestBody(rootSchema *openapi3.T, name string, requestBody *openapi3.RequestBody) {
	if rootSchema.Components == nil {
		rootSchema.Components = &openapi3.Components{}
	}
	if rootSchema.Components.RequestBodies == nil {
		rootSchema.Components.RequestBodies = make(map[string]*openapi3.RequestBodyRef)
	}
	rootSchema.Components.RequestBodies[name] = &openapi3.RequestBodyRef{
		Value: requestBody,
	}
}
