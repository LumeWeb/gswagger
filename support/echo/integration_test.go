package echo_test

import (
	"context"
	"go.lumeweb.com/gswagger/apirouter"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	oasEcho "go.lumeweb.com/gswagger/support/echo"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	swagger "go.lumeweb.com/gswagger"
	"go.lumeweb.com/gswagger/support/testutils"
)

const (
	swaggerOpenapiTitle   = "test openapi title"
	swaggerOpenapiVersion = "test openapi version"
)

func TestEchoIntegration(t *testing.T) {
	t.Run("router works correctly - echo", func(t *testing.T) {
		echoRouter, oasRouter := setupEchoSwagger(t)

		err := oasRouter.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		t.Run("host router only matches requests for its host", func(t *testing.T) {
			// Add host route
			hostRouter, err := oasRouter.Host("api.example.com")
			require.NoError(t, err)
			hostRouter.AddRoute(http.MethodGet, "/host", func(c echo.Context) error {
				return c.String(http.StatusOK, "host-response")
			}, swagger.Definitions{})

			// Add fallback route to default router
			oasRouter.AddRoute(http.MethodGet, "/fallback", func(c echo.Context) error {
				return c.String(http.StatusOK, "fallback-response")
			}, swagger.Definitions{})

			tests := []struct {
				name           string
				host           string
				path           string
				expectedStatus int
				expectedBody   string
			}{
				{
					name:           "matches correct host",
					host:           "api.example.com",
					path:           "/host",
					expectedStatus: http.StatusOK,
					expectedBody:   "host-response",
				},
				{
					name:           "rejects wrong host",
					host:           "other.example.com",
					path:           "/host",
					expectedStatus: http.StatusNotFound,
				},
				{
					name:           "fallback works for other host",
					host:           "other.example.com",
					path:           "/fallback",
					expectedStatus: http.StatusOK,
					expectedBody:   "fallback-response",
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					w := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodGet, test.path, nil)
					req.Host = test.host
					oasRouter.ServeHTTP(w, req)

					require.Equal(t, test.expectedStatus, w.Result().StatusCode)
					if test.expectedBody != "" {
						body := readBody(t, w.Result().Body)
						require.Equal(t, test.expectedBody, body)
					}
				})
			}
		})

		t.Run("/hello", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/hello", nil)

			echoRouter.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Result().StatusCode)

			body := readBody(t, w.Result().Body)
			require.Equal(t, "OK", body)
		})

		t.Run("/hello/:value", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/hello/something", nil)

			echoRouter.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Result().StatusCode)

			body := readBody(t, w.Result().Body)
			require.Equal(t, "OK", body)
		})

		t.Run("and generate swagger", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, swagger.DefaultJSONDocumentationPath, nil)

			echoRouter.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Result().StatusCode)

			body := readBody(t, w.Result().Body)
			testutils.AssertJSONMatchesFile(t, []byte(body), "../testdata/integration.json")
		})
	})

	t.Run("works correctly with subrouter - handles path prefix - echo", func(t *testing.T) {
		eRouter, oasRouter := setupEchoSwagger(t)

		subRouter, err := oasRouter.Group("/prefix")
		require.NoError(t, err)

		_, err = subRouter.AddRoute(http.MethodGet, "/foo", okHandler, swagger.Definitions{})
		require.NoError(t, err)

		err = oasRouter.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		t.Run("correctly call /hello", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/hello", nil)

			eRouter.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Result().StatusCode)
			require.Equal(t, "OK", readBody(t, w.Result().Body))
		})

		t.Run("correctly call sub router", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/prefix/foo", nil)

			eRouter.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Result().StatusCode)
			require.Equal(t, "OK", readBody(t, w.Result().Body))
		})

		t.Run("returns 404 for non-prefixed path", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/foo", nil)

			eRouter.ServeHTTP(w, r)

			require.Equal(t, http.StatusNotFound, w.Result().StatusCode)
		})

		t.Run("and generate swagger", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, swagger.DefaultJSONDocumentationPath, nil)

			eRouter.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Result().StatusCode)

			body := readBody(t, w.Result().Body)
			require.JSONEq(t, readFile(t, "../testdata/intergation-subrouter.json"), body, body)
		})
	})
}

func readBody(t *testing.T, requestBody io.ReadCloser) string {
	t.Helper()

	body, err := io.ReadAll(requestBody)
	require.NoError(t, err)

	return string(body)
}

func setupEchoSwagger(t *testing.T) (*echo.Echo, *swagger.Router[echo.HandlerFunc, echo.MiddlewareFunc, *echo.Route]) {
	t.Helper()

	ctx := context.Background()
	e := echo.New()

	router, err := swagger.NewRouter(oasEcho.NewRouter(e), swagger.Options[echo.HandlerFunc, echo.MiddlewareFunc, *echo.Route]{
		Context: ctx,
		Openapi: &openapi3.T{
			Info: &openapi3.Info{
				Title:   swaggerOpenapiTitle,
				Version: swaggerOpenapiVersion,
			},
		},
		FrameworkRouterFactory: func() apirouter.Router[echo.HandlerFunc, echo.MiddlewareFunc, *echo.Route] {
			return oasEcho.NewRouter(echo.New())
		},
	})
	require.NoError(t, err)

	operation := swagger.Operation{}

	_, err = router.AddRawRoute(http.MethodGet, "/hello", okHandler, operation)
	require.NoError(t, err)

	_, err = router.AddRoute(http.MethodPost, "/hello/:value", okHandler, swagger.Definitions{})
	require.NoError(t, err)

	return e, router
}

func okHandler(c echo.Context) error {
	return c.String(http.StatusOK, "OK")
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	fileContent, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(fileContent)
}
