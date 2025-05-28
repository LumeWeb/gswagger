package fiber

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.lumeweb.com/gswagger/apirouter"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

func TestFiberRouterSupport(t *testing.T) {
	t.Run("group with empty path prefix", func(t *testing.T) {
		fiberApp := fiber.New()
		ar := NewRouter(fiberApp)

		middlewareCalled := false
		mw := func(c *fiber.Ctx) error {
			middlewareCalled = true
			return c.Next()
		}

		// Create group with empty path
		group := ar.Group("")
		group.Use(mw)
		group.AddRoute(http.MethodGet, "/test", func(c *fiber.Ctx) error {
			return c.SendStatus(http.StatusOK)
		})

		resp, err := fiberApp.Test(httptest.NewRequest(http.MethodGet, "/test", nil))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.True(t, middlewareCalled)
	})

	t.Run("child group with empty path prefix", func(t *testing.T) {
		fiberApp := fiber.New()
		ar := NewRouter(fiberApp)

		middlewareCalled := false
		mw := func(c *fiber.Ctx) error {
			middlewareCalled = true
			return c.Next()
		}

		// Create parent group
		parentGroup := ar.Group("/api")
		// Create child group with empty path
		childGroup := parentGroup.Group("")
		childGroup.Use(mw)
		childGroup.AddRoute(http.MethodGet, "/test", func(c *fiber.Ctx) error {
			return c.SendStatus(http.StatusOK)
		})

		resp, err := fiberApp.Test(httptest.NewRequest(http.MethodGet, "/api/test", nil))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.True(t, middlewareCalled)
	})
	fiberRouter := fiber.New()
	ar := NewRouter(fiberRouter)

	t.Run("middleware is applied via Use", func(t *testing.T) {
		middlewareCalled := false
		middleware := func(c *fiber.Ctx) error {
			middlewareCalled = true
			return c.Next()
		}

		ar.Use(middleware)
		ar.AddRoute(http.MethodGet, "/middleware", func(c *fiber.Ctx) error {
			return c.SendStatus(http.StatusOK)
		})

		r := httptest.NewRequest(http.MethodGet, "/middleware", nil)
		resp, err := fiberRouter.Test(r)
		require.NoError(t, err)

		require.True(t, middlewareCalled)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("middleware is applied via AddRoute", func(t *testing.T) {
		mw1Called := false
		mw2Called := false
		mw1 := func(c *fiber.Ctx) error {
			mw1Called = true
			return c.Next()
		}
		mw2 := func(c *fiber.Ctx) error {
			mw2Called = true
			return c.Next()
		}

		ar.AddRoute(http.MethodGet, "/route-mw", func(c *fiber.Ctx) error {
			return c.SendStatus(http.StatusOK)
		}, mw1, mw2)

		r := httptest.NewRequest(http.MethodGet, "/route-mw", nil)
		resp, err := fiberRouter.Test(r)
		require.NoError(t, err)

		require.True(t, mw1Called)
		require.True(t, mw2Called)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("create a new api router", func(t *testing.T) {
		require.Implements(t, (*apirouter.Router[HandlerFunc, HandlerFunc, Route])(nil), ar)
	})

	t.Run("add new route", func(t *testing.T) {
		route := ar.AddRoute(http.MethodGet, "/foo", func(c *fiber.Ctx) error {
			return c.SendStatus(http.StatusOK)
		})
		require.IsType(t, route, fiber.New())

		t.Run("router exposes correctly api", func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/foo", nil)

			resp, err := fiberRouter.Test(r)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
		})

		t.Run("router exposes api only to the specific method", func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/foo", nil)

			resp, err := fiberRouter.Test(r)
			require.NoError(t, err)
			require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		})
	})

	t.Run("create openapi handler", func(t *testing.T) {
		handlerFunc := ar.SwaggerHandler("text/html", []byte("some data"))
		fiberRouter.Get("/oas", handlerFunc)

		t.Run("responds correctly to the API", func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/oas", nil)

			resp, err := fiberRouter.Test(r)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, "text/html", resp.Header.Get("Content-Type"))

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, "some data", string(body))
		})
	})
}
