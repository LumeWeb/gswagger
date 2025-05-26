package fiber

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.lumeweb.com/gswagger/apirouter"
)

type HandlerFunc = fiber.Handler
type Route = fiber.Router

var _ apirouter.Router[HandlerFunc, Route] = (*fiberRouter)(nil)

type fiberRouter struct {
	router     fiber.Router // Can be *fiber.App or fiber.Router (from Group)
	pathPrefix string
}

func (r fiberRouter) Router() any {
	return r.router
}

func NewRouter(router fiber.Router) apirouter.Router[HandlerFunc, Route] {
	return fiberRouter{
		router: router,
	}
}

func (r fiberRouter) Group(pathPrefix string) apirouter.Router[HandlerFunc, Route] {
	// Ensure path prefix starts with / and doesn't end with /
	cleanPrefix := strings.TrimPrefix(pathPrefix, "/")
	cleanPrefix = strings.TrimSuffix(cleanPrefix, "/")
	
	fiberGroup := r.router.Group("/" + cleanPrefix)
	return fiberRouter{
		router:     fiberGroup,
		pathPrefix: "/" + cleanPrefix,
	}
}

func (r fiberRouter) AddRoute(method string, path string, handler HandlerFunc) Route {
	return r.router.Add(method, path, handler)
}

func (r fiberRouter) SwaggerHandler(contentType string, blob []byte) HandlerFunc {
	return func(c *fiber.Ctx) error {
		c.Set("Content-Type", contentType)
		return c.Send(blob)
	}
}

func (r fiberRouter) TransformPathToOasPath(path string) string {
	// If this is a subrouter, the path is relative to the prefix.
	// We need to prepend the prefix for the OpenAPI path.
	if r.pathPrefix != "" {
		return apirouter.TransformPathParamsWithColon(r.pathPrefix + path)
	}
	return apirouter.TransformPathParamsWithColon(path)
}
