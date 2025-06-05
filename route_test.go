package swagger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/gswagger/support/gorilla"
	"go.lumeweb.com/gswagger/support/testutils" // Import the new package
)

type TestRouter = Router[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]

func setupRouter(t *testing.T) *TestRouter {
	t.Helper()

	ctx := context.Background()
	muxRouter := mux.NewRouter()

	router, err := NewRouter(gorilla.NewRouter(muxRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
		Context: ctx,
		Openapi: getBaseSwagger(t),
	})
	require.NoError(t, err)

	return router
}

func okHandler(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`OK`))
}

func TestAddRoutes(t *testing.T) {
	t.Run("schema with path and cookie parameters from Definitions.Parameters", func(t *testing.T) {
		router := setupRouter(t)

		route, err := router.AddRoute(http.MethodGet, "/users/{userId}", okHandler, Definitions{
			Parameters: map[string]ParameterDefinition{
				"userId": {
					In:          "path",
					Required:    true,
					Description: "ID of the user",
					Schema:      &Schema{Value: 0}, // Test with integer schema
				},
				"sessionToken": {
					In:          "cookie",
					Required:    false,
					Description: "Optional session token",
					Schema:      &Schema{Value: ""}, // Test with string schema
				},
			},
			Responses: map[int]ContentValue{
				200: {
					Content: Content{
						"application/json": {Value: ""},
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, route)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, DefaultJSONDocumentationPath, nil)
		router.ServeHTTP(w, req)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		body := readBody(t, w.Result().Body)

		testutils.AssertJSONMatchesFile(t, []byte(body), "testdata/schema-path-cookie-params.json")
	})

	t.Run("schema with parameter using Content", func(t *testing.T) {
		router := setupRouter(t)

		type ParamContent struct {
			Value string `json:"value"`
		}

		route, err := router.AddRoute(http.MethodGet, "/test-content-param", okHandler, Definitions{
			Parameters: map[string]ParameterDefinition{
				"contentParam": {
					In:          "query",
					Required:    true,
					Description: "Parameter with content",
					Content: Content{
						"application/json": {Value: ParamContent{}},
					},
				},
			},
			Responses: map[int]ContentValue{
				200: {
					Content: Content{
						"application/json": {Value: ""},
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, route)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, DefaultJSONDocumentationPath, nil)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		body := readBody(t, w.Result().Body)

		testutils.AssertJSONMatchesFile(t, []byte(body), "testdata/schema-content-param.json")
	})

	t.Run("schema with headers in multiple responses", func(t *testing.T) {
		router := setupRouter(t)

		route, err := router.AddRoute(http.MethodGet, "/multi-response-headers", okHandler, Definitions{
			Responses: map[int]ContentValue{
				200: {
					Content: Content{
						"application/json": {Value: ""},
					},
					Headers: map[string]string{
						"X-Success-Header": "Success indicator",
					},
				},
				400: {
					Content: Content{
						"application/json": {Value: ""},
					},
					Headers: map[string]string{
						"X-Error-Header": "Error details",
					},
					Description: "Bad Request",
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, route)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, DefaultJSONDocumentationPath, nil)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		body := readBody(t, w.Result().Body)

		testutils.AssertJSONMatchesFile(t, []byte(body), "testdata/schema-multi-response-headers.json")
	})

	t.Run("schema with parameters and response headers", func(t *testing.T) {
		router := setupRouter(t)

		route, err := router.AddRoute(http.MethodGet, "/test", okHandler, Definitions{
			Parameters: map[string]ParameterDefinition{
				"queryParam": {
					In:          "query",
					Required:    true,
					Description: "required query param",
					Schema:      &Schema{Value: ""},
				},
				"headerParam": {
					In:          "header",
					Required:    false,
					Description: "optional header param",
					Schema:      &Schema{Value: 0},
				},
			},
			Responses: map[int]ContentValue{
				200: {
					Content: Content{
						"application/json": {Value: ""},
					},
					Headers: map[string]string{
						"X-RateLimit-Limit": "Request rate limit",
						"X-Request-ID":      "Request identifier",
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, route)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		// Verify the generated OpenAPI schema contains the parameters and headers
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, DefaultJSONDocumentationPath, nil)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		body := readBody(t, w.Result().Body)

		testutils.AssertJSONMatchesFile(t, []byte(body), "testdata/schema-params-response-headers.json")
	})

	t.Run("prioritizes Definitions.Parameters over older parameter fields", func(t *testing.T) {
		router := setupRouter(t)

		route, err := router.AddRoute(http.MethodGet, "/prioritization-test", okHandler, Definitions{
			Parameters: map[string]ParameterDefinition{
				"testParam": {
					In:          "query",
					Required:    true,
					Description: "Parameter from Definitions.Parameters",
					Schema:      &Schema{Value: 123}, // Integer schema
				},
			},
			Querystring: ParameterValue{
				"testParam": {
					Schema:      &Schema{Value: "abc"}, // String schema (should be ignored)
					Description: "Parameter from Querystring",
				},
			},
			Responses: map[int]ContentValue{
				200: {
					Content: Content{
						"application/json": {Value: ""},
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, route)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, DefaultJSONDocumentationPath, nil)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		body := readBody(t, w.Result().Body)

		testutils.AssertJSONMatchesFile(t, []byte(body), "testdata/schema-prioritization.json")
	})
	type User struct {
		Name        string   `json:"name" jsonschema:"title=The user name,required" jsonschema_extras:"example=Jane"`
		PhoneNumber int      `json:"phone" jsonschema:"title=mobile number of user"`
		Groups      []string `json:"groups,omitempty" jsonschema:"title=groups of the user,default=users"`
		Address     string   `json:"address" jsonschema:"title=user address"`
	}
	type Users []User
	type errorResponse struct {
		Message string `json:"message"`
	}

	type Employees struct {
		OrganizationName string `json:"organization_name"`
		Users            Users  `json:"users" jsonschema:"selected users"`
	}
	type FormData struct {
		ID      string `json:"id,omitempty"`
		Address struct {
			Street string `json:"street,omitempty"`
			City   string `json:"city,omitempty"`
		} `json:"address,omitempty"`
		ProfileImage string `json:"profileImage,omitempty" jsonschema_extras:"format=binary"`
	}

	type UserProfileRequest struct {
		FirstName string      `json:"firstName" jsonschema:"title=user first name"`
		LastName  string      `json:"lastName" jsonschema:"title=user last name"`
		Metadata  interface{} `json:"metadata,omitempty" jsonschema:"title=custom properties,oneof_type=string;number"`
		UserType  string      `json:"userType,omitempty" jsonschema:"title=type of user,enum=simple,enum=advanced"`
	}

	tests := []struct {
		name         string
		routes       func(t *testing.T, router *TestRouter)
		fixturesPath string
		testPath     string
		testMethod   string
		expectError  bool
	}{
		{
			name:         "no routes",
			routes:       func(t *testing.T, router *TestRouter) {},
			fixturesPath: "testdata/empty.json",
		},
		{
			name: "empty route schema",
			routes: func(t *testing.T, router *TestRouter) {
				route, err := router.AddRoute(http.MethodPost, "/", okHandler, Definitions{})
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			testPath:     "/",
			testMethod:   http.MethodPost,
			fixturesPath: "testdata/empty-route-schema.json",
		},
		{
			name: "multiple real routes",
			routes: func(t *testing.T, router *TestRouter) {
				route, err := router.AddRoute(http.MethodPost, "/users", okHandler, Definitions{
					RequestBody: &ContentValue{
						Content: Content{
							jsonType: {Value: User{}},
						},
					},
					Responses: map[int]ContentValue{
						201: {
							Content: Content{
								"text/html": {Value: ""},
							},
						},
						401: {
							Content: Content{
								jsonType: {Value: &errorResponse{}},
							},
							Description: "invalid request",
						},
					},
				})
				require.NoError(t, err)
				require.NotNil(t, route)

				route, err = router.AddRoute(http.MethodGet, "/users", okHandler, Definitions{
					Responses: map[int]ContentValue{
						200: {
							Content: Content{
								jsonType: {Value: &Users{}},
							},
						},
					},
				})
				require.NoError(t, err)
				require.NotNil(t, route)

				route, err = router.AddRoute(http.MethodGet, "/employees", okHandler, Definitions{
					Responses: map[int]ContentValue{
						200: {
							Content: Content{
								jsonType: {Value: &Employees{}},
							},
						},
					},
				})
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			testPath:     "/users",
			fixturesPath: "testdata/users_employees.json",
		},
		{
			name: "multipart request body",
			routes: func(t *testing.T, router *TestRouter) {
				route, err := router.AddRoute(http.MethodPost, "/files", okHandler, Definitions{
					RequestBody: &ContentValue{
						Content: Content{
							formDataType: {
								Value:                     &FormData{},
								AllowAdditionalProperties: true,
							},
						},
						Description: "upload file",
					},
					Responses: map[int]ContentValue{
						200: {
							Content: Content{
								jsonType: {
									Value: "",
								},
							},
						},
					},
				})
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			testPath:     "/files",
			testMethod:   http.MethodPost,
			fixturesPath: "testdata/multipart-requestbody.json",
		},
		{
			name: "schema with params",
			routes: func(t *testing.T, router *TestRouter) {
				var number = 0
				route, err := router.AddRoute(http.MethodGet, "/users/{userId}", okHandler, Definitions{
					PathParams: ParameterValue{
						"userId": {
							Schema:      &Schema{Value: number},
							Description: "userId is a number above 0",
						},
					},
				})
				require.NoError(t, err)
				require.NotNil(t, route)

				route, err = router.AddRoute(http.MethodGet, "/cars/{carId}/drivers/{driverId}", okHandler, Definitions{
					PathParams: ParameterValue{
						"carId": {
							Schema: &Schema{Value: ""},
						},
						"driverId": {
							Schema: &Schema{Value: ""},
						},
					},
				})
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			testPath:     "/users/12",
			fixturesPath: "testdata/params.json",
		},
		{
			name: "schema without explicit params autofill them",
			routes: func(t *testing.T, router *TestRouter) {
				route, err := router.AddRoute(http.MethodGet, "/users/{userId}", okHandler, Definitions{
					Querystring: ParameterValue{
						"query": {
							Schema: &Schema{Value: ""},
						},
					},
				})
				require.NoError(t, err)
				require.NotNil(t, route)

				route, err = router.AddRoute(http.MethodGet, "/cars/{carId}/drivers/{driverId}", okHandler, Definitions{})
				require.NoError(t, err)
				require.NotNil(t, route)

				route, err = router.AddRoute(http.MethodGet, "/files/{name}.{extension}", okHandler, Definitions{})
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			testPath:     "/files/myid.yaml",
			fixturesPath: "testdata/params-autofill.json",
		},
		{
			name: "schema with querystring",
			routes: func(t *testing.T, router *TestRouter) {
				route, err := router.AddRoute(http.MethodGet, "/projects", okHandler, Definitions{
					Querystring: ParameterValue{
						"projectId": {
							Schema:      &Schema{Value: ""},
							Description: "projectId is the project id",
						},
					},
				})
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			testPath:     "/projects",
			fixturesPath: "testdata/query.json",
		},
		{
			name: "schema with headers",
			routes: func(t *testing.T, router *TestRouter) {
				route, err := router.AddRoute(http.MethodGet, "/projects", okHandler, Definitions{
					Headers: ParameterValue{
						"foo": {
							Schema:      &Schema{Value: ""},
							Description: "foo description",
						},
						"bar": {
							Schema: &Schema{Value: ""},
						},
					},
				})
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			testPath:     "/projects",
			fixturesPath: "testdata/headers.json",
		},
		{
			name: "schema with cookies",
			routes: func(t *testing.T, router *TestRouter) {
				route, err := router.AddRoute(http.MethodGet, "/projects", okHandler, Definitions{
					Cookies: ParameterValue{
						"debug": {
							Schema:      &Schema{Value: 0},
							Description: "boolean. Set 0 to disable and 1 to enable",
						},
						"csrftoken": {
							Schema: &Schema{Value: ""},
						},
					},
				})
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			testPath:     "/projects",
			fixturesPath: "testdata/cookies.json",
		},
		{
			name: "schema defined without value",
			routes: func(t *testing.T, router *TestRouter) {
				route, err := router.AddRoute(http.MethodPost, "/{id}", okHandler, Definitions{
					RequestBody: &ContentValue{
						Description: "request body without schema",
						Content: Content{
							jsonType: {Value: ""}, // Provide a basic schema
						},
					},
					Responses: map[int]ContentValue{
						204: {
							Content: Content{
								jsonType: {Value: ""}, // Provide a basic schema
							},
						},
					},
					PathParams: ParameterValue{
						"id": {Schema: &Schema{Value: ""}}, // Provide a basic schema
					},
					Querystring: ParameterValue{
						"q": {Schema: &Schema{Value: ""}}, // Provide a basic schema
					},
					Headers: ParameterValue{
						"key": {Schema: &Schema{Value: ""}}, // Provide a basic schema
					},
					Cookies: ParameterValue{
						"cookie1": {Schema: &Schema{Value: ""}}, // Provide a basic schema
					},
				})
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			testPath:     "/foobar",
			testMethod:   http.MethodPost,
			fixturesPath: "testdata/schema-no-content.json",
		},
		{
			name: "allOf schema",
			routes: func(t *testing.T, router *TestRouter) {
				schema := openapi3.NewAllOfSchema()
				schema.AllOf = []*openapi3.SchemaRef{
					{
						Value: openapi3.NewFloat64Schema().
							WithMin(1).
							WithMax(2),
					},
					{
						Value: openapi3.NewFloat64Schema().
							WithMin(2).
							WithMax(3),
					},
				}

				request := openapi3.NewRequestBody().WithJSONSchema(schema)
				response := openapi3.NewResponse().WithJSONSchema(schema)

				allOperation := NewOperation()
				allOperation.AddResponse(200, response)
				allOperation.AddRequestBody(request)

				route, err := router.AddRawRoute(http.MethodPost, "/all-of", okHandler, allOperation)
				require.NoError(t, err)
				require.NotNil(t, route)

				nestedSchema := openapi3.NewSchema()
				nestedSchema.Properties = map[string]*openapi3.SchemaRef{
					"foo": {
						Value: openapi3.NewStringSchema(),
					},
					"nested": {
						Value: schema,
					},
				}
				responseNested := openapi3.NewResponse().WithJSONSchema(nestedSchema)
				nestedAllOfOperation := NewOperation()
				nestedAllOfOperation.AddResponse(200, responseNested)

				route, err = router.AddRawRoute(http.MethodGet, "/nested-schema", okHandler, nestedAllOfOperation)
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			fixturesPath: "testdata/allof.json",
		},
		{
			name: "anyOf schema",
			routes: func(t *testing.T, router *TestRouter) {
				schema := openapi3.NewAnyOfSchema()
				schema.AnyOf = []*openapi3.SchemaRef{
					{
						Value: openapi3.NewFloat64Schema().
							WithMin(1).
							WithMax(2),
					},
					{
						Value: openapi3.NewFloat64Schema().
							WithMin(2).
							WithMax(3),
					},
				}
				request := openapi3.NewRequestBody().WithJSONSchema(schema)
				response := openapi3.NewResponse().WithJSONSchema(schema)

				allOperation := NewOperation()
				allOperation.AddResponse(200, response)
				allOperation.AddRequestBody(request)

				route, err := router.AddRawRoute(http.MethodPost, "/any-of", okHandler, allOperation)
				require.NoError(t, err)
				require.NotNil(t, route)

				nestedSchema := openapi3.NewSchema()
				nestedSchema.Properties = map[string]*openapi3.SchemaRef{
					"foo": {
						Value: openapi3.NewStringSchema(),
					},
					"nested": {
						Value: schema,
					},
				}
				responseNested := openapi3.NewResponse().WithJSONSchema(nestedSchema)
				nestedAnyOfOperation := NewOperation()
				nestedAnyOfOperation.AddResponse(200, responseNested)

				route, err = router.AddRawRoute(http.MethodGet, "/nested-schema", okHandler, nestedAnyOfOperation)
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			fixturesPath: "testdata/anyof.json",
		},
		{
			name: "oneOf and enum are supported on properties",
			routes: func(t *testing.T, router *TestRouter) {
				route, err := router.AddRoute(http.MethodPost, "/user-profile", okHandler, Definitions{
					RequestBody: &ContentValue{
						Content: Content{
							jsonType: {Value: &UserProfileRequest{}},
						},
					},
					Responses: map[int]ContentValue{
						200: {
							Content: Content{
								"text/plain": {Value: ""},
							},
						},
					},
				})
				require.NoError(t, err)
				require.NotNil(t, route)

				schema := openapi3.NewOneOfSchema()
				schema.OneOf = []*openapi3.SchemaRef{
					{
						Value: openapi3.NewFloat64Schema().
							WithMin(1).
							WithMax(2),
					},
					{
						Value: openapi3.NewFloat64Schema().
							WithMin(2).
							WithMax(3),
					},
				}
				request := openapi3.NewRequestBody().WithJSONSchema(schema)
				response := openapi3.NewResponse().WithJSONSchema(schema)

				allOperation := NewOperation()
				allOperation.AddResponse(200, response)
				allOperation.AddRequestBody(request)

				route, err = router.AddRawRoute(http.MethodPost, "/one-of", okHandler, allOperation)
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			testPath:     "/user-profile",
			testMethod:   http.MethodPost,
			fixturesPath: "testdata/oneOf.json",
		},
		{
			name: "schema with tags",
			routes: func(t *testing.T, router *TestRouter) {
				route, err := router.AddRoute(http.MethodGet, "/users", okHandler, Definitions{
					Tags: []string{"Tag1", "Tag2"},
				})
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			testPath:     "/users",
			fixturesPath: "testdata/tags.json",
		},
		{
			name: "schema with security",
			routes: func(t *testing.T, router *TestRouter) {
				route, err := router.AddRoute(http.MethodGet, "/users", okHandler, Definitions{
					Security: SecurityRequirements{
						SecurityRequirement{
							"api_key": []string{},
							"auth": []string{
								"resource.write",
							},
						},
					},
				})
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			testPath:     "/users",
			fixturesPath: "testdata/security.json",
		},
		{
			name: "schema with extension",
			routes: func(t *testing.T, router *TestRouter) {
				route, err := router.AddRoute(http.MethodGet, "/users", okHandler, Definitions{
					Extensions: map[string]interface{}{
						"x-extension-field": map[string]string{
							"foo": "bar",
						},
					},
				})
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			testPath:     "/users",
			fixturesPath: "testdata/extension.json",
		},
		{
			name: "invalid extension - not starts with x-",
			routes: func(t *testing.T, router *TestRouter) {
				_, err := router.AddRoute(http.MethodGet, "/", okHandler, Definitions{
					Extensions: map[string]interface{}{
						"extension-field": map[string]string{
							"foo": "bar",
						},
					},
				})
				// AddRoute itself should not fail here, validation happens later
				require.NoError(t, err)
			},
			fixturesPath: "testdata/empty.json", // This fixture won't be used for the validation error test
			expectError:  true,                  // Expect error during GenerateAndExposeOpenapi
		},
		{
			name: "schema with summary, description, deprecated and operationID",
			routes: func(t *testing.T, router *TestRouter) {
				route, err := router.AddRoute(http.MethodGet, "/users", okHandler, Definitions{
					Summary:     "small description",
					Description: "this is the long route description",
					Deprecated:  true,
				})
				require.NoError(t, err)
				require.NotNil(t, route)
			},
			testPath:     "/users",
			fixturesPath: "testdata/users-deprecated-with-description.json",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context := context.Background()
			r := mux.NewRouter()

			router, err := NewRouter(gorilla.NewRouter(r), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
				Context: context,
				Openapi: getBaseSwagger(t),
			})
			require.NoError(t, err)
			require.NotNil(t, router)

			// Add routes to test
			test.routes(t, router)

			err = router.GenerateAndExposeOpenapi()
			if test.expectError {
				require.Error(t, err)
				// Check for a specific part of the validation error message
				require.Contains(t, err.Error(), "extra sibling fields: [extension-field]")
				return // Skip the rest of the test if an error is expected
			} else {
				require.NoError(t, err)
			}

			if test.testPath != "" {
				if test.testMethod == "" {
					test.testMethod = http.MethodGet
				}

				w := httptest.NewRecorder()
				req := httptest.NewRequest(test.testMethod, test.testPath, nil)
				r.ServeHTTP(w, req)

				require.Equal(t, http.StatusOK, w.Result().StatusCode)

				body := readBody(t, w.Result().Body)
				require.Equal(t, "OK", body)
			}

			t.Run("and generate openapi documentation in json", func(t *testing.T) {
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, DefaultJSONDocumentationPath, nil)

				r.ServeHTTP(w, req)

				require.Equal(t, http.StatusOK, w.Result().StatusCode)

				body := readBody(t, w.Result().Body)

				// Use the helper for order-insensitive JSON comparison
				testutils.AssertJSONMatchesFile(t, []byte(body), test.fixturesPath)
			})
		})
	}
}

func TestResolveRequestBodySchema(t *testing.T) {
	type TestStruct struct {
		ID string `json:"id,omitempty"`
	}
	tests := []struct {
		name         string
		bodySchema   *ContentValue
		expectedErr  error
		expectedJSON string
	}{
		{
			name:         "empty body schema",
			expectedErr:  nil,
			expectedJSON: `{"responses": null}`,
		},
		{
			name:        "schema multipart",
			expectedErr: nil,
			bodySchema: &ContentValue{
				Content: Content{
					formDataType: {
						Value: &TestStruct{},
					},
				},
			},
			expectedJSON: `{
				"requestBody": {
					"content": {
						"multipart/form-data": {
							"schema": {
								"$ref": "#/$defs/TestStruct"
							}
						}
					}
				},
				"responses": null
			}`,
		},
		{
			name:        "content-type application/json",
			expectedErr: nil,
			bodySchema: &ContentValue{
				Content: Content{
					jsonType: {Value: &TestStruct{}},
				},
				Required: true,
			},
			expectedJSON: `{
				"requestBody": {
					"content": {
						"application/json": {
							"schema": {
								"$ref": "#/$defs/TestStruct"
							}
						}
					},
					"required": true
				},
				"responses": null
			}`,
		},
		{
			name:        "with description",
			expectedErr: nil,
			bodySchema: &ContentValue{
				Content: Content{
					jsonType: {
						Value: &TestStruct{},
					},
				},
				Description: "my custom description",
				Required:    false, // Explicitly set to false
			},
			expectedJSON: `{
				"requestBody": {
					"description": "my custom description",
					"content": {
						"application/json": {
							"schema": {
								"$ref": "#/$defs/TestStruct"
							}
						}
					}
				},
				"responses": null
			}`,
		},
		{
			name: "content type text/plain",
			bodySchema: &ContentValue{
				Content: Content{
					"text/plain": {Value: ""},
				},
			},
			expectedJSON: `{
				"requestBody": {
					"content": {
						"text/plain": {
							"schema": {
								"type": "string"
							}
						}
					}
				},
				"responses": null
			}`,
		},
		{
			name: "generic content type - it represent all types",
			bodySchema: &ContentValue{
				Content: Content{
					"*/*": {
						Value:                     &TestStruct{},
						AllowAdditionalProperties: true,
					},
				},
			},
			expectedJSON: `{
				"requestBody": {
					"content": {
						"*/*": {
							"schema": {
								"$ref": "#/$defs/TestStruct"
							}
						}
					}
				},
				"responses": null
			}`,
		},
	}

	_mux := mux.NewRouter()
	router, err := NewRouter(gorilla.NewRouter(_mux), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
		Openapi: getBaseSwagger(t),
	})
	require.NoError(t, err)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operation := NewOperation()

			err := router.resolveRequestBodySchema(test.bodySchema, operation)

			if err == nil {
				data, _ := operation.MarshalJSON()
				jsonData := string(data)
				require.JSONEq(t, test.expectedJSON, jsonData, "actual json data: %s", jsonData)
				require.NoError(t, err)
			}
			require.Equal(t, test.expectedErr, err)
		})
	}
}

func TestResolveResponsesSchema(t *testing.T) {
	type TestStruct struct {
		Message string `json:"message,omitempty"`
	}
	type NestedTestStruct struct {
		Notification       string                `json:"notification"`
		NestedMapOfStructs map[string]TestStruct `json:"nestedMapOfStructs,omitempty"`
	}
	type ComplexTestStruct struct {
		Communication string                      `json:"communication"`
		MapOfStructs  map[string]NestedTestStruct `json:"mapOfStructs,omitempty"`
	}
	tests := []struct {
		name            string
		responsesSchema map[int]ContentValue
		expectedErr     error
		expectedJSON    string
	}{
		{
			name:         "empty responses schema",
			expectedErr:  nil,
			expectedJSON: `{"responses": {"default":{"description":""}}}`,
		},
		{
			name: "with 1 status code",
			responsesSchema: map[int]ContentValue{
				200: {
					Content: Content{
						jsonType: {Value: &TestStruct{}},
					},
				},
			},
			expectedErr: nil,
			expectedJSON: `{
				"responses": {
					"200": {
						"description": "",
						"content": {
							"application/json": {
								"schema": {
									"$ref": "#/$defs/TestStruct"
								}
							}
						}
					}
				}
			}`,
		},
		{
			name: "with complex schema",
			responsesSchema: map[int]ContentValue{
				200: {
					Content: Content{
						jsonType: {Value: &ComplexTestStruct{
							Communication: "myCommunication",
							MapOfStructs: map[string]NestedTestStruct{
								"myProperty": {
									Notification: "myNotification",
									NestedMapOfStructs: map[string]TestStruct{
										"myNestedProperty": {
											Message: "myMessage",
										},
									},
								},
							},
						}},
					},
				},
			},
			expectedErr: nil,
			expectedJSON: `{
				"responses": {
				  "200": {
					"content": {
					  "application/json": {
						"schema": {
						  "$ref": "#/$defs/ComplexTestStruct"
						}
					  }
					},
					"description": ""
				  }
				}
			  }`,
		},
		{
			name: "with more status codes",
			responsesSchema: map[int]ContentValue{
				200: {
					Content: Content{
						jsonType: {Value: &TestStruct{}},
					},
				},
				400: {
					Content: Content{
						jsonType: {Value: ""},
					},
				},
			},
			expectedErr: nil,
			expectedJSON: `{
				"responses": {
					"200": {
						"description": "",
						"content": {
							"application/json": {
								"schema": {
									"$ref": "#/$defs/TestStruct"
								}
							}
						}
					},
					"400": {
						"description": "",
						"content": {
							"application/json": {
								"schema": {
									"type": "string"
								}
							}
						}
					}
				}
			}`,
		},
		{
			name: "with custom description",
			responsesSchema: map[int]ContentValue{
				400: {
					Content: Content{
						jsonType: {Value: ""},
					},
					Description: "a description",
				},
			},
			expectedErr: nil,
			expectedJSON: `{
				"responses": {
					"400": {
						"description": "a description",
						"content": {
							"application/json": {
								"schema": {
									"type": "string"
								}
							}
						}
					}
				}
			}`,
		},
	}

	_mux := mux.NewRouter()
	router, err := NewRouter(gorilla.NewRouter(_mux), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
		Openapi: getBaseSwagger(t),
	})
	require.NoError(t, err)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operation := NewOperation()
			operation.Responses = &openapi3.Responses{}

			err := router.resolveResponsesSchema(test.responsesSchema, operation)

			if err == nil {
				data, _ := operation.MarshalJSON()
				jsonData := string(data)
				require.JSONEq(t, test.expectedJSON, jsonData, "actual json data: %s", jsonData)
				require.NoError(t, err)
			}
			require.Equal(t, test.expectedErr, err)
		})
	}
}

// Test types for cycle detection
type Node struct {
	Next *Node
}

type Parent struct {
	Child *Child
}
type Child struct {
	Parent *Parent
}

type User struct {
	Name   string
	Groups []*Group
}
type Group struct {
	Name  string
	Users []*User
}

type A struct {
	B *B
}
type B struct {
	C *C
}
type C struct {
	A *A
}

func TestCycleDetection(t *testing.T) {
	type UnexportedCycle struct {
		name string
		self *UnexportedCycle
	}

	type MixedFields struct {
		Name     string
		Exported *MixedFields
		hidden   *MixedFields // unexported
	}

	type EmbeddedCycle struct {
		*EmbeddedCycle // embedded pointer creates cycle
		Name           string
	}

	type IndirectUnexported struct {
		hidden struct {
			parent *IndirectUnexported
		}
	}

	tests := []struct {
		name          string
		input         any
		expectError   bool
		errorContains string // Optional expected error substring
	}{
		{
			name:        "no cycle",
			input:       struct{ Name string }{Name: "test"},
			expectError: false,
		},
		{
			name: "simple cycle",
			input: func() *Node {
				n := &Node{}
				n.Next = n
				return n
			}(),
			expectError: true,
		},
		{
			name:        "nested cycle",
			input:       &Parent{Child: &Child{Parent: &Parent{}}},
			expectError: true,
		},
		{
			name: "complex cycle with slices",
			input: &User{
				Groups: []*Group{
					{
						Users: []*User{
							{Name: "test"},
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "indirect cycle through multiple types",
			input: &A{
				B: &B{
					C: &C{
						A: &A{},
					},
				},
			},
			expectError: true,
		},
		{
			name: "primitive types don't trigger cycles",
			input: struct {
				Name string
				Age  int
			}{
				Name: "test",
				Age:  30,
			},
			expectError: false,
		},
		{
			name: "unexported fields are skipped",
			input: &UnexportedCycle{
				name: "test",
				self: &UnexportedCycle{},
			},
			expectError: false,
		},
		{
			name: "mixed exported/unexported fields",
			input: &MixedFields{
				Name:     "test",
				Exported: &MixedFields{},
				hidden:   &MixedFields{},
			},
			expectError: true, // Should error since there's a cycle through Exported
		},
		{
			name:          "embedded cycle",
			input:         &EmbeddedCycle{},
			expectError:   true,
			errorContains: "embedded struct *swagger.EmbeddedCycle detected - potential infinite recursion",
		},
		{
			name: "indirect embedded cycle",
			input: &struct {
				*Parent // Embedded type that will cycle
			}{},
			expectError:   true,
			errorContains: "embedded struct *swagger.Parent detected - potential infinite recursion",
		},
		{
			name: "indirect unexported cycle",
			input: &IndirectUnexported{
				hidden: struct {
					parent *IndirectUnexported
				}{
					parent: &IndirectUnexported{},
				},
			},
			expectError: false,
		},
		{
			name: "map with cyclic values",
			input: map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": nil, // Will be set to point back to root
					},
				},
			},
			expectError: true,
		},
	}

	router := setupRouter(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := router.getSchemaFromInterface(tt.input, false)
			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				} else {
					assert.Contains(t, err.Error(), "cycle detected in type graph", "Expected cycle error not found")
				}
				t.Logf("Error message:\n%s", err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetPathParamsAutoComplete(t *testing.T) {
	testCases := map[string]struct {
		schemaDefinition Definitions
		path             string
		expected         ParameterValue
	}{
		"no path params": {
			schemaDefinition: Definitions{},
			path:             "/users",
			expected:         nil,
		},
		"with path params": {
			schemaDefinition: Definitions{},
			path:             "/users/{userId}",
			expected: ParameterValue{
				"userId": {
					Schema: &Schema{Value: ""},
				},
			},
		},
		"with multiple path params": {
			schemaDefinition: Definitions{},
			path:             "/foo/{bar}.{taz}",
			expected: ParameterValue{
				"bar": {
					Schema: &Schema{Value: ""},
				},
				"taz": {
					Schema: &Schema{Value: ""},
				},
			},
		},
		"with nested multiple path params": {
			schemaDefinition: Definitions{},
			path:             "/foo/{bar}.{taz}/{baz}/ok",
			expected: ParameterValue{
				"bar": {
					Schema: &Schema{Value: ""},
				},
				"taz": {
					Schema: &Schema{Value: ""},
				},
				"baz": {
					Schema: &Schema{Value: ""},
				},
			},
		},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			actual := getPathParamsAutoComplete(test.schemaDefinition, test.path)

			require.Equal(t, test.expected, actual)
		})
	}
}
