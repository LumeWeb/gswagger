package swagger

import (
	"context"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/labstack/echo/v4"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/gswagger/apirouter"
	gecho "go.lumeweb.com/gswagger/support/echo"
	gfiber "go.lumeweb.com/gswagger/support/fiber"
	"go.lumeweb.com/gswagger/support/gorilla"
)

func setupGorillaMiddlewareTest(t *testing.T) (*Router[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route], *mux.Router) {
	t.Helper()

	muxRouter := mux.NewRouter()
	gorillaRouter := gorilla.NewRouter(muxRouter)

	openapi := &openapi3.T{
		Info: &openapi3.Info{
			Title:   "middleware test",
			Version: "1.0",
		},
		Paths: &openapi3.Paths{},
	}

	router, err := NewRouter(gorillaRouter, Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
		Openapi: openapi,
	})
	require.NoError(t, err)

	return router, muxRouter
}

func TestFiberMiddleware(t *testing.T) {
	t.Run("root router Use delegates to underlying router", func(t *testing.T) {
		router, fiberApp := setupFiberMiddlewareTest(t)
		middlewareCalled := false

		router.Use(func(c *fiber.Ctx) error {
			middlewareCalled = true
			return c.Next()
		})

		router.AddRoute(http.MethodGet, "/test", func(c *fiber.Ctx) error {
			return c.SendStatus(http.StatusOK)
		}, Definitions{})

		resp, err := fiberApp.Test(httptest.NewRequest(http.MethodGet, "/test", nil))
		require.NoError(t, err)
		defer resp.Body.Close()

		require.True(t, middlewareCalled)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("middleware is called via Router().Use", func(t *testing.T) {
		router, fiberApp := setupFiberMiddlewareTest(t)
		middlewareCalled := false

		router.Router().Use(func(c *fiber.Ctx) error {
			middlewareCalled = true
			return c.Next()
		})

		router.AddRoute(http.MethodGet, "/test-router-use", func(c *fiber.Ctx) error {
			return c.SendStatus(http.StatusOK)
		}, Definitions{})

		resp, err := fiberApp.Test(httptest.NewRequest(http.MethodGet, "/test-router-use", nil))
		require.NoError(t, err)
		defer resp.Body.Close()

		require.True(t, middlewareCalled)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("middleware is called via AddRoute", func(t *testing.T) {
		router, fiberApp := setupFiberMiddlewareTest(t)
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

		router.AddRoute(http.MethodGet, "/route-mw", func(c *fiber.Ctx) error {
			return c.SendStatus(http.StatusOK)
		}, Definitions{}, mw1, mw2)

		resp, err := fiberApp.Test(httptest.NewRequest(http.MethodGet, "/route-mw", nil))
		require.NoError(t, err)
		defer resp.Body.Close()

		require.True(t, mw1Called)
		require.True(t, mw2Called)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("multiple middleware are called in order", func(t *testing.T) {
		router, fiberApp := setupFiberMiddlewareTest(t)
		var callOrder []string

		router.Router().Use(func(c *fiber.Ctx) error {
			callOrder = append(callOrder, "first")
			return c.Next()
		})
		router.Router().Use(func(c *fiber.Ctx) error {
			callOrder = append(callOrder, "second")
			return c.Next()
		})

		router.AddRoute(http.MethodGet, "/order", func(c *fiber.Ctx) error {
			return c.SendStatus(http.StatusOK)
		}, Definitions{})

		resp, err := fiberApp.Test(httptest.NewRequest(http.MethodGet, "/order", nil))
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, []string{"first", "second"}, callOrder)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestFiberNewRouter(t *testing.T) {
	fiberApp := fiber.New()
	fiberRouter := gfiber.NewRouter(fiberApp)

	info := &openapi3.Info{
		Title:   "my title",
		Version: "my version",
	}
	openapi := &openapi3.T{
		Info:  info,
		Paths: &openapi3.Paths{},
	}

	t.Run("not ok - invalid Openapi option", func(t *testing.T) {
		r, err := NewRouter(fiberRouter, Options[fiber.Handler, fiber.Handler, gfiber.Route]{})

		require.Nil(t, r)
		require.EqualError(t, err, fmt.Sprintf("%s: openapi is required", ErrValidatingOAS))
	})

	t.Run("ok - with default context", func(t *testing.T) {
		r, err := NewRouter(fiberRouter, Options[fiber.Handler, fiber.Handler, gfiber.Route]{
			Openapi: openapi,
		})

		require.NoError(t, err)
		expected := &Router[fiber.Handler, fiber.Handler, gfiber.Route]{
			context:               context.Background(),
			router:                fiberRouter,
			swaggerSchema:         openapi,
			jsonDocumentationPath: DefaultJSONDocumentationPath,
			yamlDocumentationPath: DefaultYAMLDocumentationPath,
			hostRouters:           make(map[string]*Router[fiber.Handler, fiber.Handler, gfiber.Route]),
			rootRouter:            nil, // This will be set below
			defaultRouter:         nil, // This will be set below
		}
		expected.rootRouter = expected    // Set root reference to self
		expected.defaultRouter = expected // Set default router to self
		require.Equal(t, expected, r)
	})

	t.Run("ok - with custom context", func(t *testing.T) {
		type key struct{}
		ctx := context.WithValue(context.Background(), key{}, "value")
		r, err := NewRouter(fiberRouter, Options[fiber.Handler, fiber.Handler, gfiber.Route]{
			Openapi: openapi,
			Context: ctx,
		})

		require.NoError(t, err)
		expected := &Router[fiber.Handler, fiber.Handler, gfiber.Route]{
			context:               ctx,
			router:                fiberRouter,
			swaggerSchema:         openapi,
			jsonDocumentationPath: DefaultJSONDocumentationPath,
			yamlDocumentationPath: DefaultYAMLDocumentationPath,
			hostRouters:           make(map[string]*Router[fiber.Handler, fiber.Handler, gfiber.Route]),
			rootRouter:            nil, // This will be set below
			defaultRouter:         nil, // This will be set below
		}
		expected.rootRouter = expected    // Set root reference to self
		expected.defaultRouter = expected // Set default router to self
		require.Equal(t, expected, r)
	})

	t.Run("ok - with custom docs paths", func(t *testing.T) {
		type key struct{}
		ctx := context.WithValue(context.Background(), key{}, "value")
		r, err := NewRouter(fiberRouter, Options[fiber.Handler, fiber.Handler, gfiber.Route]{
			Openapi:               openapi,
			Context:               ctx,
			JSONDocumentationPath: "/json/path",
			YAMLDocumentationPath: "/yaml/path",
		})

		require.NoError(t, err)
		expected := &Router[fiber.Handler, fiber.Handler, gfiber.Route]{
			context:               ctx,
			router:                fiberRouter,
			swaggerSchema:         openapi,
			jsonDocumentationPath: "/json/path",
			yamlDocumentationPath: "/yaml/path",
			hostRouters:           make(map[string]*Router[fiber.Handler, fiber.Handler, gfiber.Route]),
			rootRouter:            nil, // This will be set below
			defaultRouter:         nil, // This will be set below
		}
		expected.rootRouter = expected    // Set root reference to self
		expected.defaultRouter = expected // Set default router to self
		require.Equal(t, expected, r)
	})

	t.Run("ko - json documentation path does not start with /", func(t *testing.T) {
		type key struct{}
		ctx := context.WithValue(context.Background(), key{}, "value")
		r, err := NewRouter(fiberRouter, Options[fiber.Handler, fiber.Handler, gfiber.Route]{
			Openapi:               openapi,
			Context:               ctx,
			JSONDocumentationPath: "json/path",
			YAMLDocumentationPath: "/yaml/path",
		})

		require.EqualError(t, err, "invalid path json/path. Path should start with '/'")
		require.Nil(t, r)
	})

	t.Run("ko - yaml documentation path does not start with /", func(t *testing.T) {
		type key struct{}
		ctx := context.WithValue(context.Background(), key{}, "value")
		r, err := NewRouter(fiberRouter, Options[fiber.Handler, fiber.Handler, gfiber.Route]{
			Openapi:               openapi,
			Context:               ctx,
			JSONDocumentationPath: "/json/path",
			YAMLDocumentationPath: "yaml/path",
		})

		require.EqualError(t, err, "invalid path yaml/path. Path should start with '/'")
		require.Nil(t, r)
	})
}

func TestFiberGenerateAndExposeSwagger(t *testing.T) {
	t.Run("correctly expose json documentation from loaded openapi file", func(t *testing.T) {
		fiberApp := fiber.New()
		router, err := NewRouter(gfiber.NewRouter(fiberApp), Options[fiber.Handler, fiber.Handler, gfiber.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "test openapi title",
					Version: "test openapi version",
				},
			},
		})
		require.NoError(t, err)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		resp, err := fiberApp.Test(httptest.NewRequest(http.MethodGet, DefaultJSONDocumentationPath, nil))
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.True(t, strings.Contains(resp.Header.Get("content-type"), "application/json"))
	})
}

func setupFiberHostRouterTest(t *testing.T) (*fiber.App, *Router[fiber.Handler, fiber.Handler, gfiber.Route]) {
	t.Helper()

	info := &openapi3.Info{
		Title:   "Host Routing Test",
		Version: "1.0",
	}
	openapi := &openapi3.T{
		Info:  info,
		Paths: &openapi3.Paths{},
	}

	fiberApp := fiber.New()
	fiberRouter := gfiber.NewRouter(fiberApp)

	router, err := NewRouter(fiberRouter, Options[fiber.Handler, fiber.Handler, gfiber.Route]{
		Openapi: openapi,
		FrameworkRouterFactory: func() apirouter.Router[fiber.Handler, fiber.Handler, gfiber.Route] {
			return gfiber.NewRouter(fiber.New())
		},
	})
	require.NoError(t, err)

	return fiberApp, router
}

func TestFiberHostRouting(t *testing.T) {
	t.Run("create host router", func(t *testing.T) {
		_, router := setupFiberHostRouterTest(t)

		hostRouter, err := router.Host("api.example.com")
		require.NoError(t, err)
		_, err = router.Host("api.example.com") // Test getting existing host
		require.NoError(t, err)
		require.Equal(t, "api.example.com", hostRouter.host)
	})
}

func setupFiberMiddlewareTest(t *testing.T) (*Router[fiber.Handler, fiber.Handler, gfiber.Route], *fiber.App) {
	t.Helper()

	fiberApp := fiber.New()
	fiberRouter := gfiber.NewRouter(fiberApp)

	openapi := &openapi3.T{
		Info: &openapi3.Info{
			Title:   "middleware test",
			Version: "1.0",
		},
		Paths: &openapi3.Paths{},
	}

	router, err := NewRouter(fiberRouter, Options[fiber.Handler, fiber.Handler, gfiber.Route]{
		Openapi: openapi,
	})
	require.NoError(t, err)

	return router, fiberApp
}

func setupEchoMiddlewareTest(t *testing.T) (*Router[echo.HandlerFunc, echo.MiddlewareFunc, gecho.Route], *echo.Echo) {
	t.Helper()

	echoRouter := echo.New()
	echoAPIRouter := gecho.NewRouter(echoRouter)

	openapi := &openapi3.T{
		Info: &openapi3.Info{
			Title:   "middleware test",
			Version: "1.0",
		},
		Paths: &openapi3.Paths{},
	}

	router, err := NewRouter(echoAPIRouter, Options[echo.HandlerFunc, echo.MiddlewareFunc, gecho.Route]{
		Openapi: openapi,
	})
	require.NoError(t, err)

	return router, echoRouter
}

func TestGorillaMiddleware(t *testing.T) {
	t.Run("root router Use delegates to underlying router", func(t *testing.T) {
		router, muxRouter := setupGorillaMiddlewareTest(t)
		middlewareCalled := false

		router.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				middlewareCalled = true
				next.ServeHTTP(w, r)
			})
		})

		router.AddRoute(http.MethodGet, "/test", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, Definitions{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		muxRouter.ServeHTTP(w, r)

		require.True(t, middlewareCalled)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})

	t.Run("middleware is called via Router().Use", func(t *testing.T) {
		router, muxRouter := setupGorillaMiddlewareTest(t)
		middlewareCalled := false

		router.Router().Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				middlewareCalled = true
				next.ServeHTTP(w, r)
			})
		})

		router.AddRoute(http.MethodGet, "/test-router-use", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, Definitions{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/test-router-use", nil)
		muxRouter.ServeHTTP(w, r)

		require.True(t, middlewareCalled)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})

	t.Run("middleware is called via AddRoute", func(t *testing.T) {
		router, muxRouter := setupGorillaMiddlewareTest(t)
		mw1Called := false
		mw2Called := false

		mw1 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mw1Called = true
				next.ServeHTTP(w, r)
			})
		}
		mw2 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mw2Called = true
				next.ServeHTTP(w, r)
			})
		}

		router.AddRoute(http.MethodGet, "/route-mw", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, Definitions{}, mw1, mw2)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/route-mw", nil)
		muxRouter.ServeHTTP(w, r)

		require.True(t, mw1Called)
		require.True(t, mw2Called)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})

	t.Run("multiple middleware are called in order", func(t *testing.T) {
		router, muxRouter := setupGorillaMiddlewareTest(t)
		var callOrder []string

		router.Router().Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callOrder = append(callOrder, "first")
				next.ServeHTTP(w, r)
			})
		})
		router.Router().Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callOrder = append(callOrder, "second")
				next.ServeHTTP(w, r)
			})
		})

		router.AddRoute(http.MethodGet, "/order", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, Definitions{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/order", nil)
		muxRouter.ServeHTTP(w, r)

		require.Equal(t, []string{"first", "second"}, callOrder)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})
}

func TestGorillaNewRouter(t *testing.T) {
	muxRouter := mux.NewRouter()
	mAPIRouter := gorilla.NewRouter(muxRouter)

	info := &openapi3.Info{
		Title:   "my title",
		Version: "my version",
	}
	openapi := &openapi3.T{
		Info:  info,
		Paths: &openapi3.Paths{},
	}

	t.Run("not ok - invalid Openapi option", func(t *testing.T) {
		r, err := NewRouter(mAPIRouter, Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{})

		require.Nil(t, r)
		require.EqualError(t, err, fmt.Sprintf("%s: openapi is required", ErrValidatingOAS))
	})

	t.Run("ok - with default context", func(t *testing.T) {
		r, err := NewRouter(mAPIRouter, Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: openapi,
		})

		require.NoError(t, err)
		expected := &Router[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			context:               context.Background(),
			router:                mAPIRouter,
			swaggerSchema:         openapi,
			jsonDocumentationPath: DefaultJSONDocumentationPath,
			yamlDocumentationPath: DefaultYAMLDocumentationPath,
			hostRouters:           make(map[string]*Router[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]),
			rootRouter:            nil, // This will be set below
			defaultRouter:         nil, // This will be set below
		}
		expected.rootRouter = expected    // Set root reference to self
		expected.defaultRouter = expected // Set default router to self
		require.Equal(t, expected, r)
	})

	t.Run("ok - with custom context", func(t *testing.T) {
		type key struct{}
		ctx := context.WithValue(context.Background(), key{}, "value")
		r, err := NewRouter(mAPIRouter, Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: openapi,
			Context: ctx,
		})

		require.NoError(t, err)
		expected := &Router[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			context:               ctx,
			router:                mAPIRouter,
			swaggerSchema:         openapi,
			jsonDocumentationPath: DefaultJSONDocumentationPath,
			yamlDocumentationPath: DefaultYAMLDocumentationPath,
			hostRouters:           make(map[string]*Router[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]),
			rootRouter:            nil, // This will be set below
			defaultRouter:         nil, // This will be set below
		}
		expected.rootRouter = expected    // Set root reference to self
		expected.defaultRouter = expected // Set default router to self
		require.Equal(t, expected, r)
	})

	t.Run("ok - with custom docs paths", func(t *testing.T) {
		type key struct{}
		ctx := context.WithValue(context.Background(), key{}, "value")
		r, err := NewRouter(mAPIRouter, Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi:               openapi,
			Context:               ctx,
			JSONDocumentationPath: "/json/path",
			YAMLDocumentationPath: "/yaml/path",
		})

		require.NoError(t, err)
		expected := &Router[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			context:               ctx,
			router:                mAPIRouter,
			swaggerSchema:         openapi,
			jsonDocumentationPath: "/json/path",
			yamlDocumentationPath: "/yaml/path",
			hostRouters:           make(map[string]*Router[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]),
			rootRouter:            nil, // This will be set below
			defaultRouter:         nil, // This will be set below
		}
		expected.rootRouter = expected    // Set root reference to self
		expected.defaultRouter = expected // Set default router to self
		require.Equal(t, expected, r)
	})

	t.Run("ko - json documentation path does not start with /", func(t *testing.T) {
		type key struct{}
		ctx := context.WithValue(context.Background(), key{}, "value")
		r, err := NewRouter(mAPIRouter, Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi:               openapi,
			Context:               ctx,
			JSONDocumentationPath: "json/path",
			YAMLDocumentationPath: "/yaml/path",
		})

		require.EqualError(t, err, "invalid path json/path. Path should start with '/'")
		require.Nil(t, r)
	})

	t.Run("ko - yaml documentation path does not start with /", func(t *testing.T) {
		type key struct{}
		ctx := context.WithValue(context.Background(), key{}, "value")
		r, err := NewRouter(mAPIRouter, Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi:               openapi,
			Context:               ctx,
			JSONDocumentationPath: "/json/path",
			YAMLDocumentationPath: "yaml/path",
		})

		require.EqualError(t, err, "invalid path yaml/path. Path should start with '/'")
		require.Nil(t, r)
	})
}

func TestGorillaGenerateValidSwagger(t *testing.T) {
	t.Run("not ok - empty openapi info", func(t *testing.T) {
		openapi := &openapi3.T{}

		openapi, err := generateNewValidOpenapi(openapi)
		require.Nil(t, openapi)
		require.EqualError(t, err, "openapi info is required")
	})

	t.Run("not ok - empty info title", func(t *testing.T) {
		openapi := &openapi3.T{
			Info: &openapi3.Info{},
		}

		openapi, err := generateNewValidOpenapi(openapi)
		require.Nil(t, openapi)
		require.EqualError(t, err, "openapi info title is required")
	})

	t.Run("not ok - empty info version", func(t *testing.T) {
		openapi := &openapi3.T{
			Info: &openapi3.Info{
				Title: "title",
			},
		}

		openapi, err := generateNewValidOpenapi(openapi)
		require.Nil(t, openapi)
		require.EqualError(t, err, "openapi info version is required")
	})

	t.Run("ok - custom openapi", func(t *testing.T) {
		openapi := &openapi3.T{
			Info: &openapi3.Info{},
		}

		openapi, err := generateNewValidOpenapi(openapi)
		require.Nil(t, openapi)
		require.EqualError(t, err, "openapi info title is required")
	})

	t.Run("not ok - openapi is required", func(t *testing.T) {
		openapi, err := generateNewValidOpenapi(nil)
		require.Nil(t, openapi)
		require.EqualError(t, err, "openapi is required")
	})

	t.Run("ok", func(t *testing.T) {
		info := &openapi3.Info{
			Title:   "my title",
			Version: "my version",
		}
		openapi := &openapi3.T{
			Info: info,
		}

		openapi, err := generateNewValidOpenapi(openapi)
		require.NoError(t, err)
		require.Equal(t, &openapi3.T{
			OpenAPI: defaultOpenapiVersion,
			Info:    info,
			Paths:   &openapi3.Paths{},
		}, openapi)
	})
}

func TestGorillaGenerateAndExposeSwagger(t *testing.T) {
	t.Run("fails openapi validation", func(t *testing.T) {
		mRouter := mux.NewRouter()
		router, err := NewRouter(gorilla.NewRouter(mRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "title",
					Version: "version",
				},
				Components: &openapi3.Components{
					Schemas: map[string]*openapi3.SchemaRef{
						"&%": {},
					},
				},
			},
		})
		require.NoError(t, err)

		err = router.GenerateAndExposeOpenapi()
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), fmt.Sprintf("%s:", ErrValidatingOAS)))
	})

	t.Run("correctly expose json documentation from loaded openapi file", func(t *testing.T) {
		mRouter := mux.NewRouter()

		openapi, err := openapi3.NewLoader().LoadFromFile("testdata/users_employees.json")
		require.NoError(t, err)

		router, err := NewRouter(gorilla.NewRouter(mRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: openapi,
		})
		require.NoError(t, err)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, DefaultJSONDocumentationPath, nil)
		mRouter.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		require.True(t, strings.Contains(w.Result().Header.Get("content-type"), "application/json"))

		body := readBody(t, w.Result().Body)
		actual, err := os.ReadFile("testdata/users_employees.json")
		require.NoError(t, err)
		require.JSONEq(t, string(actual), body)
	})

	t.Run("correctly expose json documentation from loaded openapi file - custom path", func(t *testing.T) {
		mRouter := mux.NewRouter()

		openapi, err := openapi3.NewLoader().LoadFromFile("testdata/users_employees.json")
		require.NoError(t, err)

		router, err := NewRouter(gorilla.NewRouter(mRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi:               openapi,
			JSONDocumentationPath: "/custom/path",
		})
		require.NoError(t, err)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/custom/path", nil)
		mRouter.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		require.True(t, strings.Contains(w.Result().Header.Get("content-type"), "application/json"))

		body := readBody(t, w.Result().Body)
		actual, err := os.ReadFile("testdata/users_employees.json")
		require.NoError(t, err)
		require.JSONEq(t, string(actual), body)
	})

	t.Run("correctly expose yaml documentation from loaded openapi file", func(t *testing.T) {
		mRouter := mux.NewRouter()

		openapi, err := openapi3.NewLoader().LoadFromFile("testdata/users_employees.json")
		require.NoError(t, err)

		router, err := NewRouter(gorilla.NewRouter(mRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: openapi,
		})
		require.NoError(t, err)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, DefaultYAMLDocumentationPath, nil)
		mRouter.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		require.True(t, strings.Contains(w.Result().Header.Get("content-type"), "text/plain"))

		body := readBody(t, w.Result().Body)
		expected, err := os.ReadFile("testdata/users_employees.yaml")
		require.NoError(t, err)
		require.YAMLEq(t, string(expected), body, string(body))
	})

	t.Run("correctly expose yaml documentation from loaded openapi file - custom path", func(t *testing.T) {
		mRouter := mux.NewRouter()

		openapi, err := openapi3.NewLoader().LoadFromFile("testdata/users_employees.json")
		require.NoError(t, err)

		router, err := NewRouter(gorilla.NewRouter(mRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi:               openapi,
			YAMLDocumentationPath: "/custom/path",
		})
		require.NoError(t, err)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/custom/path", nil)
		mRouter.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		require.True(t, strings.Contains(w.Result().Header.Get("content-type"), "text/plain"))

		body := readBody(t, w.Result().Body)
		expected, err := os.ReadFile("testdata/users_employees.yaml")
		require.NoError(t, err)
		require.YAMLEq(t, string(expected), body, string(body))
	})

	t.Run("ok - subrouter", func(t *testing.T) {
		mRouter := mux.NewRouter()

		router, err := NewRouter(gorilla.NewRouter(mRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "test openapi title",
					Version: "test openapi version",
				},
			},
			JSONDocumentationPath: "/custom/path",
		})
		require.NoError(t, err)

		router.AddRoute(http.MethodGet, "/foo", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}, Definitions{})

		mSubRouter := mRouter.NewRoute().Subrouter()
		subrouter, err := router.SubRouter(gorilla.NewRouter(mSubRouter), SubRouterOptions{
			PathPrefix: "/prefix",
		})
		require.NoError(t, err)

		_, err = subrouter.AddRoute(http.MethodGet, "/taz", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}, Definitions{})
		require.NoError(t, err)

		t.Run("add route with path equal to prefix path", func(t *testing.T) {
			_, err = subrouter.AddRoute(http.MethodGet, "", func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
			}, Definitions{})
			require.NoError(t, err)
		})

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/custom/path", nil)
		mRouter.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		require.True(t, strings.Contains(w.Result().Header.Get("content-type"), "application/json"))

		body := readBody(t, w.Result().Body)
		actual, err := os.ReadFile("testdata/subrouter.json")
		require.NoError(t, err)
		require.JSONEq(t, string(actual), body)

		t.Run("test request /prefix", func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/prefix", nil)
			mRouter.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Result().StatusCode)
		})

		t.Run("test request /prefix/taz", func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/prefix/taz", nil)
			mRouter.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Result().StatusCode)
		})

		t.Run("test request /foo", func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/foo", nil)
			mRouter.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Result().StatusCode)
		})
	})

	t.Run("ok - new router with path prefix", func(t *testing.T) {
		router, err := NewRouter(gorilla.NewRouter(mux.NewRouter()), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "test openapi title",
					Version: "test openapi version",
				},
			},
			JSONDocumentationPath: "/custom/path",
			PathPrefix:            "/prefix",
		})
		require.NoError(t, err)

		router.AddRoute(http.MethodGet, "/foo", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}, Definitions{})

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/prefix/custom/path", nil)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		require.True(t, strings.Contains(w.Result().Header.Get("content-type"), "application/json"))

		body := readBody(t, w.Result().Body)
		actual, err := os.ReadFile("testdata/router_with_prefix.json")
		require.NoError(t, err)
		require.JSONEq(t, string(actual), body, body)
	})
}

func TestGroup(t *testing.T) {
	t.Run("ok - create a group and add routes", func(t *testing.T) {
		mRouter := mux.NewRouter()

		router, err := NewRouter(gorilla.NewRouter(mRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "test openapi title",
					Version: "test openapi version",
				},
			},
			JSONDocumentationPath: "/custom/path",
		})
		require.NoError(t, err)

		router.AddRoute(http.MethodGet, "/foo", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}, Definitions{})

		apiGroup, err := router.Group("/api/v1")
		require.NoError(t, err)

		_, err = apiGroup.AddRoute(http.MethodGet, "/users", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}, Definitions{})
		require.NoError(t, err)

		_, err = apiGroup.AddRoute(http.MethodPost, "/users", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}, Definitions{})
		require.NoError(t, err)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/custom/path", nil)
		mRouter.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		body := readBody(t, w.Result().Body)
		actual, err := os.ReadFile("testdata/group.json")
		require.NoError(t, err)
		require.JSONEq(t, string(actual), body, body)
	})
}

func TestGetRouter(t *testing.T) {
	t.Run("gets gorilla router instance", func(t *testing.T) {
		muxRouter := mux.NewRouter()
		gorillaRouter := gorilla.NewRouter(muxRouter)
		router, err := NewRouter(gorillaRouter, Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "test",
					Version: "1.0",
				},
			},
		})
		require.NoError(t, err)

		muxRouterFromGetter := GetRouter[*mux.Router](router.Router())
		require.Equal(t, muxRouter, muxRouterFromGetter)
	})

	t.Run("panics on wrong type", func(t *testing.T) {
		muxRouter := mux.NewRouter()
		router, err := NewRouter(gorilla.NewRouter(muxRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "test",
					Version: "1.0",
				},
			},
		})
		require.NoError(t, err)

		require.Panics(t, func() {
			_ = GetRouter[string](router.Router())
		})
	})
}

func TestEchoMiddleware(t *testing.T) {
	t.Run("root router Use delegates to underlying router", func(t *testing.T) {
		router, echoRouter := setupEchoMiddlewareTest(t)
		middlewareCalled := false

		router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				middlewareCalled = true
				return next(c)
			}
		})

		router.AddRoute(http.MethodGet, "/test", func(c echo.Context) error {
			return c.String(http.StatusOK, "")
		}, Definitions{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		echoRouter.ServeHTTP(w, r)

		require.True(t, middlewareCalled)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})

	t.Run("middleware is called via Router().Use", func(t *testing.T) {
		router, echoRouter := setupEchoMiddlewareTest(t)
		middlewareCalled := false

		router.Router().Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				middlewareCalled = true
				return next(c)
			}
		})

		router.AddRoute(http.MethodGet, "/test-router-use", func(c echo.Context) error {
			return c.String(http.StatusOK, "")
		}, Definitions{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/test-router-use", nil)
		echoRouter.ServeHTTP(w, r)

		require.True(t, middlewareCalled)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})

	t.Run("middleware is called via AddRoute", func(t *testing.T) {
		router, echoRouter := setupEchoMiddlewareTest(t)
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

		router.AddRoute(http.MethodGet, "/route-mw", func(c echo.Context) error {
			return c.String(http.StatusOK, "")
		}, Definitions{}, mw1, mw2)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/route-mw", nil)
		echoRouter.ServeHTTP(w, r)

		require.True(t, mw1Called)
		require.True(t, mw2Called)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})

	t.Run("multiple middleware are called in order", func(t *testing.T) {
		router, echoRouter := setupEchoMiddlewareTest(t)
		var callOrder []string

		router.Router().Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				callOrder = append(callOrder, "first")
				return next(c)
			}
		})
		router.Router().Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				callOrder = append(callOrder, "second")
				return next(c)
			}
		})

		router.AddRoute(http.MethodGet, "/order", func(c echo.Context) error {
			return c.String(http.StatusOK, "")
		}, Definitions{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/order", nil)
		echoRouter.ServeHTTP(w, r)

		require.Equal(t, []string{"first", "second"}, callOrder)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})
}

func TestEchoNewRouter(t *testing.T) {
	echoRouter := echo.New()
	echoAPIRouter := gecho.NewRouter(echoRouter)

	info := &openapi3.Info{
		Title:   "my title",
		Version: "my version",
	}
	openapi := &openapi3.T{
		Info:  info,
		Paths: &openapi3.Paths{},
	}

	t.Run("not ok - invalid Openapi option", func(t *testing.T) {
		r, err := NewRouter(echoAPIRouter, Options[echo.HandlerFunc, echo.MiddlewareFunc, gecho.Route]{})

		require.Nil(t, r)
		require.EqualError(t, err, fmt.Sprintf("%s: openapi is required", ErrValidatingOAS))
	})

	t.Run("ok - with default context", func(t *testing.T) {
		r, err := NewRouter(echoAPIRouter, Options[echo.HandlerFunc, echo.MiddlewareFunc, gecho.Route]{
			Openapi: openapi,
		})

		require.NoError(t, err)
		expected := &Router[echo.HandlerFunc, echo.MiddlewareFunc, gecho.Route]{
			context:               context.Background(),
			router:                echoAPIRouter,
			swaggerSchema:         openapi,
			jsonDocumentationPath: DefaultJSONDocumentationPath,
			yamlDocumentationPath: DefaultYAMLDocumentationPath,
			hostRouters:           make(map[string]*Router[echo.HandlerFunc, echo.MiddlewareFunc, gecho.Route]),
			rootRouter:            nil, // This will be set below
			defaultRouter:         nil, // This will be set below
		}
		expected.rootRouter = expected    // Set root reference to self
		expected.defaultRouter = expected // Set default router to self
		require.Equal(t, expected, r)
	})
}

func TestEchoGenerateAndExposeSwagger(t *testing.T) {
	t.Run("correctly expose json documentation from loaded openapi file", func(t *testing.T) {
		echoRouter := echo.New()
		router, err := NewRouter(gecho.NewRouter(echoRouter), Options[echo.HandlerFunc, echo.MiddlewareFunc, gecho.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "test openapi title",
					Version: "test openapi version",
				},
			},
		})
		require.NoError(t, err)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, DefaultJSONDocumentationPath, nil)
		echoRouter.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		require.True(t, strings.Contains(w.Result().Header.Get("content-type"), "application/json"))
	})
}

func setupEchoHostRouterTest(t *testing.T) (*echo.Echo, *Router[echo.HandlerFunc, echo.MiddlewareFunc, gecho.Route]) {
	t.Helper()

	info := &openapi3.Info{
		Title:   "Host Routing Test",
		Version: "1.0",
	}
	openapi := &openapi3.T{
		Info:  info,
		Paths: &openapi3.Paths{},
	}

	echoRouter := echo.New()

	router, err := NewRouter(gecho.NewRouter(echoRouter), Options[echo.HandlerFunc, echo.MiddlewareFunc, gecho.Route]{
		Openapi: openapi,
		FrameworkRouterFactory: func() apirouter.Router[echo.HandlerFunc, echo.MiddlewareFunc, gecho.Route] {
			return gecho.NewRouter(echo.New())
		},
	})
	require.NoError(t, err)

	return echoRouter, router
}

func TestEchoHostRouting(t *testing.T) {
	t.Run("create host router", func(t *testing.T) {
		_, router := setupEchoHostRouterTest(t)

		hostRouter, err := router.Host("api.example.com")
		require.NoError(t, err)
		_, err = router.Host("api.example.com") // Test getting existing host
		require.NoError(t, err)
		require.Equal(t, "api.example.com", hostRouter.host)
	})
}

func readBody(t *testing.T, requestBody io.ReadCloser) string {
	t.Helper()

	body, err := io.ReadAll(requestBody)
	require.NoError(t, err)

	return string(body)
}

func setupGorillaHostRouterTest(t *testing.T) (*mux.Router, *Router[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]) {
	t.Helper()

	info := &openapi3.Info{
		Title:   "Host Routing Test",
		Version: "1.0",
	}
	openapi := &openapi3.T{
		Info:  info,
		Paths: &openapi3.Paths{},
	}

	muxRouter := mux.NewRouter()
	gorillaRouter := gorilla.NewRouter(muxRouter)

	router, err := NewRouter(gorillaRouter, Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
		Openapi: openapi,
		FrameworkRouterFactory: func() apirouter.Router[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route] {
			return gorilla.NewRouter(mux.NewRouter())
		},
	})
	require.NoError(t, err)

	return muxRouter, router
}

func TestSetInfo(t *testing.T) {
	t.Run("set info on root router", func(t *testing.T) {
		muxRouter := mux.NewRouter()
		router, err := NewRouter(gorilla.NewRouter(muxRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "initial title",
					Version: "1.0",
				},
			},
		})
		require.NoError(t, err)

		newInfo := &openapi3.Info{
			Title:       "new title",
			Version:     "2.0",
			Description: "new description",
		}

		router.SetInfo(newInfo)

		require.Equal(t, newInfo, router.swaggerSchema.Info)
	})

	t.Run("set info on host router", func(t *testing.T) {
		muxRouter := mux.NewRouter()
		router, err := NewRouter(gorilla.NewRouter(muxRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "initial title",
					Version: "1.0",
				},
			},
			FrameworkRouterFactory: func() apirouter.Router[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route] {
				return gorilla.NewRouter(mux.NewRouter())
			},
		})
		require.NoError(t, err)

		hostRouter, err := router.Host("api.example.com")
		require.NoError(t, err)

		newInfo := &openapi3.Info{
			Title:   "host title",
			Version: "1.0",
		}

		hostRouter.SetInfo(newInfo)

		require.Equal(t, newInfo, hostRouter.swaggerSchema.Info)
		require.NotEqual(t, router.swaggerSchema.Info, hostRouter.swaggerSchema.Info)
	})

	t.Run("set info on subrouter", func(t *testing.T) {
		muxRouter := mux.NewRouter()
		router, err := NewRouter(gorilla.NewRouter(muxRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "initial title",
					Version: "1.0",
				},
			},
		})
		require.NoError(t, err)

		subrouter, err := router.Group("/api")
		require.NoError(t, err)

		newInfo := &openapi3.Info{
			Title:   "subrouter title",
			Version: "1.0",
		}

		subrouter.SetInfo(newInfo)

		require.Equal(t, newInfo, subrouter.swaggerSchema.Info)
		require.Equal(t, newInfo, router.swaggerSchema.Info) // Should be same since subrouter shares schema
	})

	t.Run("validation fails with empty title", func(t *testing.T) {
		muxRouter := mux.NewRouter()
		router, err := NewRouter(gorilla.NewRouter(muxRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "initial title",
					Version: "1.0",
				},
			},
		})
		require.NoError(t, err)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		invalidInfo := &openapi3.Info{
			Title:   "", // Empty title
			Version: "1.0",
		}

		router.SetInfo(invalidInfo)

		err = router.GenerateAndExposeOpenapi()
		require.Error(t, err)
		require.Contains(t, err.Error(), "value of title must be a non-empty string")
	})

	t.Run("validation fails with empty version", func(t *testing.T) {
		muxRouter := mux.NewRouter()
		router, err := NewRouter(gorilla.NewRouter(muxRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "initial title",
					Version: "1.0",
				},
			},
		})
		require.NoError(t, err)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		invalidInfo := &openapi3.Info{
			Title:   "valid title",
			Version: "", // Empty version
		}

		router.SetInfo(invalidInfo)

		err = router.GenerateAndExposeOpenapi()
		require.Error(t, err)
		require.Contains(t, err.Error(), "value of version must be a non-empty string")
	})

	t.Run("nil info is a no-op on root router", func(t *testing.T) {
		muxRouter := mux.NewRouter()
		router, err := NewRouter(gorilla.NewRouter(muxRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "initial title",
					Version: "1.0",
				},
			},
		})
		require.NoError(t, err)

		originalInfo := router.swaggerSchema.Info
		result := router.SetInfo(nil)

		require.Equal(t, originalInfo, router.swaggerSchema.Info)
		require.Equal(t, router, result) // Verify method chaining still works
	})

	t.Run("nil info is a no-op on host router", func(t *testing.T) {
		muxRouter := mux.NewRouter()
		router, err := NewRouter(gorilla.NewRouter(muxRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "initial title",
					Version: "1.0",
				},
			},
			FrameworkRouterFactory: func() apirouter.Router[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route] {
				return gorilla.NewRouter(mux.NewRouter())
			},
		})
		require.NoError(t, err)

		hostRouter, err := router.Host("api.example.com")
		require.NoError(t, err)

		originalInfo := hostRouter.swaggerSchema.Info
		result := hostRouter.SetInfo(nil)

		require.Equal(t, originalInfo, hostRouter.swaggerSchema.Info)
		require.Equal(t, hostRouter, result) // Verify method chaining still works
	})

	t.Run("nil info is a no-op on subrouter", func(t *testing.T) {
		muxRouter := mux.NewRouter()
		router, err := NewRouter(gorilla.NewRouter(muxRouter), Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
			Openapi: &openapi3.T{
				Info: &openapi3.Info{
					Title:   "initial title",
					Version: "1.0",
				},
			},
		})
		require.NoError(t, err)

		subrouter, err := router.Group("/api")
		require.NoError(t, err)

		originalInfo := subrouter.swaggerSchema.Info
		result := subrouter.SetInfo(nil)

		require.Equal(t, originalInfo, subrouter.swaggerSchema.Info)
		require.Equal(t, subrouter, result) // Verify method chaining still works
	})
}

func TestGorillaHostRouting(t *testing.T) {
	t.Run("generate docs on host router", func(t *testing.T) {
		_, router := setupGorillaHostRouterTest(t)

		hostRouter, err := router.Host("api.example.com")
		require.NoError(t, err)

		// Add a route to the host router
		_, err = hostRouter.AddRoute(http.MethodGet, "/host", func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("host"))
		}, Definitions{})
		require.NoError(t, err)

		// Generate docs directly on host router
		err = hostRouter.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/documentation/json", nil)
		req.Host = "api.example.com"
		router.ServeHTTP(w, req) // Use root router's ServeHTTP
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})

	t.Run("create host router", func(t *testing.T) {
		_, router := setupGorillaHostRouterTest(t)

		hostRouter, err := router.Host("api.example.com")
		require.NoError(t, err)
		_, err = router.Host("api.example.com") // Test getting existing host
		require.NoError(t, err)
		require.Equal(t, "api.example.com", hostRouter.host)
	})

	t.Run("host router isolation", func(t *testing.T) {
		_, router := setupGorillaHostRouterTest(t)

		_, err := router.AddRoute(http.MethodGet, "/default", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, Definitions{})
		require.NoError(t, err)

		hostRouter, err := router.Host("api.example.com")
		require.NoError(t, err)
		_, err = hostRouter.AddRoute(http.MethodGet, "/host", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, Definitions{})
		require.NoError(t, err)

		require.NotEqual(t, router.swaggerSchema, hostRouter.swaggerSchema)
		require.Equal(t, 1, len(router.swaggerSchema.Paths.Map()))
		require.Equal(t, 1, len(hostRouter.swaggerSchema.Paths.Map()))
	})

	t.Run("host-based request routing", func(t *testing.T) {
		_, router := setupGorillaHostRouterTest(t)

		_, err := router.AddRoute(http.MethodGet, "/default", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("default"))
		}, Definitions{})
		require.NoError(t, err)

		hostRouter, err := router.Host("api.example.com")
		require.NoError(t, err)
		_, err = hostRouter.AddRoute(http.MethodGet, "/host", func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("host"))
		}, Definitions{})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/default", nil)
		router.ServeHTTP(w, req) // Use root router's ServeHTTP
		require.Equal(t, "default", w.Body.String())

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/host", nil)
		req.Host = "api.example.com"
		router.ServeHTTP(w, req) // Use root router's ServeHTTP
		require.Equal(t, "host", w.Body.String())
	})

	t.Run("host-specific documentation", func(t *testing.T) {
		_, router := setupGorillaHostRouterTest(t)

		_, err := router.Host("api.example.com")
		require.NoError(t, err)

		err = router.GenerateAndExposeOpenapi()
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/documentation/json", nil)
		req.Host = "api.example.com"
		router.ServeHTTP(w, req) // Use root router's ServeHTTP
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})

	t.Run("host router error cases", func(t *testing.T) {
		_, router := setupGorillaHostRouterTest(t)

		// Create a group router
		groupRouter, err := router.Group("/api")
		require.NoError(t, err)

		t.Run("cannot call Host on non-root router", func(t *testing.T) {
			_, err := groupRouter.Host("api.example.com")
			require.Error(t, err)
			require.Equal(t, "Host() can only be called on the root router instance", err.Error())
		})

		t.Run("empty host name", func(t *testing.T) {
			_, err := router.Host("")
			require.Error(t, err)
			require.Equal(t, "Host name cannot be empty", err.Error())
		})

		t.Run("missing framework router factory", func(t *testing.T) {
			info := &openapi3.Info{
				Title:   "Host Routing Test",
				Version: "1.0",
			}
			openapi := &openapi3.T{
				Info:  info,
				Paths: &openapi3.Paths{},
			}
			muxRouter := mux.NewRouter()
			gorillaRouter := gorilla.NewRouter(muxRouter)

			routerNoFactory, err := NewRouter(gorillaRouter, Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]{
				Openapi: openapi,
			})
			require.NoError(t, err)

			_, err = routerNoFactory.Host("api.example.com")
			require.Error(t, err)
			require.Equal(t, "FrameworkRouterFactory is not set in NewRouter Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]", err.Error())
		})
	})

	t.Run("host router middleware", func(t *testing.T) {
		_, router := setupGorillaHostRouterTest(t)

		hostRouter, err := router.Host("api.example.com")
		require.NoError(t, err)

		middlewareCalled := false
		hostRouter.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				middlewareCalled = true
				next.ServeHTTP(w, r)
			})
		})

		_, err = hostRouter.AddRoute(http.MethodGet, "/test", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, Definitions{})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Host = "api.example.com"
		router.ServeHTTP(w, req) // Use root router's ServeHTTP

		require.True(t, middlewareCalled)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})
}
