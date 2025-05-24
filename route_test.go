package swagger

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/gswagger/support/gorilla"
	"go.lumeweb.com/gswagger/support/testutils"
)

type TestRouter = Router[gorilla.HandlerFunc, gorilla.Route]

var muxRouter *mux.Router

func setupRouter(t *testing.T) (*TestRouter, error) {
	t.Helper()

	ctx := context.Background()
	muxRouter = mux.NewRouter()

	router, err := NewRouter(gorilla.NewRouter(muxRouter), Options{
		Context: ctx,
		Openapi: getBaseSwagger(t),
	})
	require.NoError(t, err)

	return router, nil
}

func okHandler(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`OK`))
}

func TestAddRoutes(t *testing.T) {
	t.Run("schema with path and cookie parameters from Definitions.Parameters", func(t *testing.T) {
		router, _ := setupRouter(t)

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
		muxRouter.ServeHTTP(w, req)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		body := readBody(t, w.Result().Body)

		testutils.AssertJSONMatchesFile(t, []byte(body), "testdata/schema-path-cookie-params.json")
	})

	t.Run("schema with parameter using Content", func(t *testing.T) {
		router, _ := setupRouter(t)

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
		muxRouter.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		body := readBody(t, w.Result().Body)

		testutils.AssertJSONMatchesFile(t, []byte(body), "testdata/schema-content-param.json")
	})

	t.Run("schema with headers in multiple responses", func(t *testing.T) {
		router, _ := setupRouter(t)

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
		muxRouter.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		body := readBody(t, w.Result().Body)

		testutils.AssertJSONMatchesFile(t, []byte(body), "testdata/schema-multi-response-headers.json")
	})

	t.Run("schema with parameters and response headers", func(t *testing.T) {
		router, _ := setupRouter(t)

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
		muxRouter.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		body := readBody(t, w.Result().Body)

		testutils.AssertJSONMatchesFile(t, []byte(body), "testdata/schema-params-response-headers.json")
	})

	t.Run("prioritizes Definitions.Parameters over older parameter fields", func(t *testing.T) {
		router, _ := setupRouter(t)

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
		muxRouter.ServeHTTP(w, req)

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
					},
					Responses: map[int]ContentValue{
						204: {},
					},
					PathParams: ParameterValue{
						"id": {},
					},
					Querystring: ParameterValue{
						"q": {},
					},
					Headers: ParameterValue{
						"key": {},
					},
					Cookies: ParameterValue{
						"cookie1": {},
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
							"application/json": {
								Value: &UserProfileRequest{},
							},
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
				route, err := router.AddRoute(http.MethodGet, "/", okHandler, Definitions{
					Extensions: map[string]interface{}{
						"extension-field": map[string]string{
							"foo": "bar",
						},
					},
				})
				require.EqualError(t, err, "extra sibling fields: [extension-field]")
				require.Nil(t, route)
			},
			fixturesPath: "testdata/empty.json",
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

			router, err := NewRouter(gorilla.NewRouter(r), Options{
				Context: context,
				Openapi: getBaseSwagger(t),
			})
			require.NoError(t, err)
			require.NotNil(t, router)

			// Add routes to test
			test.routes(t, router)

			err = router.GenerateAndExposeOpenapi()
			require.NoError(t, err)

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
				expected, err := os.ReadFile(test.fixturesPath)
				require.NoError(t, err)
				require.JSONEq(t, string(expected), body, "actual json data: %s", body)
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
								"type":"object",
								"additionalProperties":false,
								"properties": {
									"id": {"type":"string"}
								}
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
								"type":"object",
								"additionalProperties":false,
								"properties": {
									"id": {"type":"string"}
								}
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
			},
			expectedJSON: `{
				"requestBody": {
					"description": "my custom description",
					"required": true,
					"content": {
						"application/json": {
							"schema": {
								"type":"object",
								"additionalProperties":false,
								"properties": {
									"id": {"type":"string"}
								}
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
								"type":"string"
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
								"type":"object",
								"properties": {
									"id": {"type": "string"}
								}
							}
						}
					}
				},
				"responses": null
			}`,
		},
	}

	mux := mux.NewRouter()
	router, err := NewRouter(gorilla.NewRouter(mux), Options{
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
									"type": "object",
									"properties": {
										"message": {
											"type": "string"
										}
									},
									"additionalProperties": false
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
						  "additionalProperties": false,
						  "properties": {
							"communication": {
							  "type": "string"
							},
							"mapOfStructs": {
							  "additionalProperties": {
								"additionalProperties": false,
								"properties": {
								  "nestedMapOfStructs": {
									"additionalProperties": {
									  "additionalProperties": false,
									  "properties": {
										"message": {
										  "type": "string"
										}
									  },
									  "type": "object"
									},
									"type": "object"
								  },
								  "notification": {
									"type": "string"
								  }
								},
								"required": [
								  "notification"
								],
								"type": "object"
							  },
							  "type": "object"
							}
						  },
						  "required": [
							"communication"
						  ],
						  "type": "object"
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
									"type": "object",
									"properties": {
										"message": {
											"type": "string"
										}
									},
									"additionalProperties": false
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

	mux := mux.NewRouter()
	router, err := NewRouter(gorilla.NewRouter(mux), Options{
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

func TestResolveParametersSchema(t *testing.T) {
	type TestStruct struct {
		Message string `json:"message,omitempty"`
	}
	tests := []struct {
		name         string
		paramsSchema ParameterValue
		paramType    string
		expectedErr  error
		expectedJSON string
	}{
		{
			name:         "empty responses schema",
			paramType:    pathParamsType,
			expectedJSON: `{"responses": null}`,
		},
		{
			name:      "path param",
			paramType: pathParamsType,
			paramsSchema: ParameterValue{
				"foo": {
					Schema: &Schema{
						Value: "",
					},
				},
			},
			expectedJSON: `{
				"parameters": [{
					"in": "path",
					"name": "foo",
					"required": true,
					"schema": {
						"type": "string"
					}
				}],
				"responses": null
			}`,
		},
		{
			name:      "query param",
			paramType: queryParamType,
			paramsSchema: ParameterValue{
				"foo": {
					Schema: &Schema{
						Value: "",
					},
				},
			},
			expectedJSON: `{
				"parameters": [{
					"in": "query",
					"name": "foo",
					"schema": {
						"type": "string"
					}
				}],
				"responses": null
			}`,
		},
		{
			name:      "cookie param",
			paramType: cookieParamType,
			paramsSchema: ParameterValue{
				"foo": {
					Schema: &Schema{
						Value: "",
					},
				},
			},
			expectedJSON: `{
				"parameters": [{
					"in": "cookie",
					"name": "foo",
					"schema": {
						"type": "string"
					}
				}],
				"responses": null
			}`,
		},
		{
			name:      "header param",
			paramType: headerParamType,
			paramsSchema: ParameterValue{
				"foo": {
					Schema: &Schema{
						Value: "",
					},
				},
			},
			expectedJSON: `{
				"parameters": [{
					"in": "header",
					"name": "foo",
					"schema": {
						"type": "string"
					}
				}],
				"responses": null
			}`,
		},
		{
			name:      "wrong param type",
			paramType: "wrong",
			paramsSchema: ParameterValue{
				"foo": {
					Schema: &Schema{
						Value: "",
					},
				},
			},
			expectedErr: fmt.Errorf("invalid param type"),
		},
		{
			name:      "content param",
			paramType: "query",
			paramsSchema: ParameterValue{
				"foo": {
					Content: Content{
						jsonType: {
							Value: &TestStruct{},
						},
					},
				},
			},
			expectedJSON: `{
				"parameters": [{
					"in": "query",
					"name": "foo",
					"content": {
						"application/json": {
							"schema": {
								"type": "object",
								"properties": {
									"message": {"type": "string"}
								},
								"additionalProperties": false
							}
						}
					}
				}],
				"responses": null
			}`,
		},
	}

	mux := mux.NewRouter()
	router, err := NewRouter(gorilla.NewRouter(mux), Options{
		Openapi: getBaseSwagger(t),
	})
	require.NoError(t, err)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operation := NewOperation()

			err := router.resolveParameterSchema(test.paramType, test.paramsSchema, operation)

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

func getBaseSwagger(t *testing.T) *openapi3.T {
	t.Helper()

	return &openapi3.T{
		Info: &openapi3.Info{
			Title:   "test openapi title",
			Version: "test openapi version",
		},
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
