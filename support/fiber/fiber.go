package fiber

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.lumeweb.com/gswagger/apirouter"
)

type HandlerFunc = fiber.Handler
type Route = fiber.Router

var _ apirouter.Router[HandlerFunc, HandlerFunc, Route] = (*fiberRouter)(nil)

type fiberRouter struct {
	router     fiber.Router // Can be *fiber.App or fiber.Router (from Group)
	pathPrefix string
}

func (r fiberRouter) Router() any {
	return r.router
}

func NewRouter(router fiber.Router) apirouter.Router[HandlerFunc, HandlerFunc, Route] {
	return fiberRouter{
		router: router,
	}
}

func (r fiberRouter) Group(pathPrefix string) apirouter.Router[HandlerFunc, HandlerFunc, Route] {
	// Ensure path prefix starts with / and doesn't end with /
	cleanPrefix := strings.TrimPrefix(pathPrefix, "/")
	cleanPrefix = strings.TrimSuffix(cleanPrefix, "/")

	fiberGroup := r.router.Group("/" + cleanPrefix)
	return fiberRouter{
		router:     fiberGroup,
		pathPrefix: "/" + cleanPrefix,
	}
}

func (r fiberRouter) AddRoute(method string, path string, handler HandlerFunc, middleware ...HandlerFunc) Route {
	handlers := make([]HandlerFunc, 0, len(middleware)+1)
	handlers = append(handlers, middleware...)
	handlers = append(handlers, handler)
	return r.router.Add(method, path, handlers...)
}

func (r fiberRouter) SwaggerHandler(contentType string, blob []byte) HandlerFunc {
	return func(c *fiber.Ctx) error {
		c.Set("Content-Type", contentType)
		return c.Send(blob)
	}
}

func (r fiberRouter) Use(middleware ...HandlerFunc) {
	useMiddleware(r.router, middleware...)
}

func (r fiberRouter) TransformPathToOasPath(path string) string {
	// If this is a subrouter, the path is relative to the prefix.
	// We need to prepend the prefix for the OpenAPI path.
	if r.pathPrefix != "" {
		return apirouter.TransformPathParamsWithColon(r.pathPrefix + path)
	}
	return apirouter.TransformPathParamsWithColon(path)
}

func useMiddleware(router fiber.Router, middleware ...HandlerFunc) fiber.Router {
	if len(middleware) > 0 {
		for _, mw := range middleware {
			router.Use(mw)
		}
	}

	return router
}
