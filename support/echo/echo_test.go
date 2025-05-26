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
}
