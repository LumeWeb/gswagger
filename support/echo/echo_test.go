package echo

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/gswagger/apirouter"
)

func TestEchoRouter(t *testing.T) {
	t.Run("matches routes added to main router", func(t *testing.T) {
		echoRouter := echo.New()
		ar := NewRouter(echoRouter)

		// Add route
		ar.AddRoute(http.MethodGet, "/test", func(c echo.Context) error {
			return c.String(http.StatusOK, "")
		})

		// Test matching route
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		exists, _ := ar.HasRoute(req)
		require.True(t, exists)

		// Test non-matching route
		req = httptest.NewRequest(http.MethodGet, "/not-found", nil)
		exists, _ = ar.HasRoute(req)
		require.False(t, exists)

		// Test matching method
		req = httptest.NewRequest(http.MethodPost, "/test", nil)
		exists, _ = ar.HasRoute(req)
		require.False(t, exists)
	})

	t.Run("group with empty path prefix", func(t *testing.T) {
		echoRouter := echo.New()
		ar := NewRouter(echoRouter)

		middlewareCalled := false
		mw := func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				middlewareCalled = true
				return next(c)
			}
		}

		// Create group with empty path
		group := ar.Group("")
		group.Use(mw)
		group.AddRoute(http.MethodGet, "/test", func(c echo.Context) error {
			return c.String(http.StatusOK, "")
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		echoRouter.ServeHTTP(w, r)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		require.True(t, middlewareCalled)
	})

	t.Run("child group with empty path prefix", func(t *testing.T) {
		echoRouter := echo.New()
		ar := NewRouter(echoRouter)

		middlewareCalled := false
		mw := func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				middlewareCalled = true
				return next(c)
			}
		}

		// Create parent group
		parentGroup := ar.Group("/api")
		// Create child group with empty path
		childGroup := parentGroup.Group("")
		childGroup.Use(mw)
		childGroup.AddRoute(http.MethodGet, "/test", func(c echo.Context) error {
			return c.String(http.StatusOK, "")
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		echoRouter.ServeHTTP(w, r)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		require.True(t, middlewareCalled)
	})
	echoRouter := echo.New()
	ar := NewRouter(echoRouter)

	t.Run("middleware is applied via Use", func(t *testing.T) {
		middlewareCalled := false
		middleware := func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				middlewareCalled = true
				return next(c)
			}
		}

		ar.Use(middleware)
		ar.AddRoute(http.MethodGet, "/middleware", func(c echo.Context) error {
			return c.String(http.StatusOK, "")
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/middleware", nil)
		echoRouter.ServeHTTP(w, r)

		require.True(t, middlewareCalled)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})

	t.Run("middleware is applied via AddRoute", func(t *testing.T) {
		mw1Called := false
		mw2Called := false
		mw1 := func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				mw1Called = true
				return next(c)
			}
		}
		mw2 := func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				mw2Called = true
				return next(c)
			}
		}

		ar.AddRoute(http.MethodGet, "/route-mw", func(c echo.Context) error {
			return c.String(http.StatusOK, "")
		}, mw1, mw2)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/route-mw", nil)
		echoRouter.ServeHTTP(w, r)

		require.True(t, mw1Called)
		require.True(t, mw2Called)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})

	t.Run("create a new api router", func(t *testing.T) {
		require.Implements(t, (*apirouter.Router[echo.HandlerFunc, echo.MiddlewareFunc, Route])(nil), ar)
	})

	t.Run("add new route", func(t *testing.T) {
		route := ar.AddRoute(http.MethodGet, "/foo", func(c echo.Context) error {
			return c.String(http.StatusOK, "")
		})
		require.IsType(t, route, &echo.Route{})

		t.Run("router exposes correctly api", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/foo", nil)

			echoRouter.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Result().StatusCode)
		})

		t.Run("router exposes api only to the specific method", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/foo", nil)

			echoRouter.ServeHTTP(w, r)

			require.Equal(t, http.StatusMethodNotAllowed, w.Result().StatusCode)
		})
	})

	t.Run("create openapi handler", func(t *testing.T) {
		handlerFunc := ar.SwaggerHandler("text/html", []byte("some data"))
		echoRouter.GET("/oas", handlerFunc)

		t.Run("responds correctly to the API", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/oas", nil)

			echoRouter.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Result().StatusCode)
			require.Equal(t, "text/html", w.Result().Header.Get("Content-Type"))

			body, err := io.ReadAll(w.Result().Body)
			require.NoError(t, err)
			require.Equal(t, "some data", string(body))
		})
	})

	t.Run("custom HTTP handler override", func(t *testing.T) {
		echoRouter := echo.New()
		ar := NewRouter(echoRouter)

		// Add a normal route
		ar.AddRoute(http.MethodGet, "/normal", func(c echo.Context) error {
			return c.String(http.StatusOK, "normal route")
		})

		// Set up test cases
		tests := []struct {
			name           string
			customHandler  http.Handler
			path           string
			expectedStatus int
			expectedBody   string
		}{
			{
				name: "no custom handler",
				path: "/normal",
				expectedStatus: http.StatusOK,
				expectedBody: "normal route",
			},
			{
				name: "custom handler takes precedence",
				customHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusTeapot)
					w.Write([]byte("custom handler"))
				}),
				path: "/normal",
				expectedStatus: http.StatusTeapot,
				expectedBody: "custom handler",
			},
			{
				name: "custom handler handles 404s",
				customHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/missing" {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte("handled 404"))
					} else {
						w.WriteHeader(http.StatusNotFound)
					}
				}),
				path: "/missing",
				expectedStatus: http.StatusOK,
				expectedBody: "handled 404",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Set custom handler if specified
				if tt.customHandler != nil {
					ar.customServeHTTPHandler = tt.customHandler
				} else {
					ar.customServeHTTPHandler = nil
				}

				w := httptest.NewRecorder()
				r := httptest.NewRequest(http.MethodGet, tt.path, nil)
				echoRouter.ServeHTTP(w, r)

				require.Equal(t, tt.expectedStatus, w.Result().StatusCode)
				body, err := io.ReadAll(w.Result().Body)
				require.NoError(t, err)
				require.Equal(t, tt.expectedBody, string(body))
			})
		}
	})
}
