package echo

import (
	"go.lumeweb.com/gswagger/apirouter"

	"net/http"

	"github.com/labstack/echo/v4"
)

type Route = *echo.Route

var _ apirouter.Router[echo.HandlerFunc, Route] = (*echoRouter)(nil)

type echoRouter struct {
	router *echo.Echo
	group  *echo.Group
}

func (r echoRouter) AddRoute(method string, path string, handler echo.HandlerFunc) Route {
	return r.router.Add(method, path, handler)
}

func (r echoRouter) SwaggerHandler(contentType string, blob []byte) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Add("Content-Type", contentType)
		return c.JSONBlob(http.StatusOK, blob)
	}
}

func (r echoRouter) TransformPathToOasPath(path string) string {
	return apirouter.TransformPathParamsWithColon(path)
}

func (r echoRouter) Router() any {
	if r.group != nil {
		return r.group
	}
	return r.router
}

func NewRouter(router *echo.Echo) apirouter.Router[echo.HandlerFunc, Route] {
	return echoRouter{
		router: router,
	}
}

func (r echoRouter) Group(pathPrefix string) apirouter.Router[echo.HandlerFunc, Route] {
	echoGroup := r.router.Group(pathPrefix)
	return echoRouter{
		router: r.router,
		group:  echoGroup,
	}
}
