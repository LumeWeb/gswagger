package fiber

import (
	"github.com/gofiber/fiber/v2"
	"go.lumeweb.com/gswagger/apirouter"
)

type HandlerFunc = fiber.Handler
type Route = fiber.Router

type fiberRouter struct {
	router fiber.Router
}

func (r fiberRouter) Router() any {
	return r.router
}

func NewRouter(router fiber.Router) apirouter.Router[HandlerFunc, Route] {
	return fiberRouter{
		router: router,
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
	return apirouter.TransformPathParamsWithColon(path)
}
