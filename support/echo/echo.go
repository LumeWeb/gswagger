package echo

import (
	"github.com/labstack/echo/v4"
	"go.lumeweb.com/gswagger/apirouter"
	"net/http"
	"reflect"
	"strings"
)

type Route = *echo.Route

var _ apirouter.Router[echo.HandlerFunc, echo.MiddlewareFunc, Route] = (*echoRouter)(nil)

type echoRouter struct {
	router *echo.Echo
	group  *echo.Group
}

func (r echoRouter) AddRoute(method string, _path string, handler echo.HandlerFunc, middleware ...echo.MiddlewareFunc) Route {
	if len(middleware) > 0 {
		if r.group != nil {
			return r.group.Add(method, _path, handler, middleware...)
		}
		return r.router.Add(method, _path, handler, middleware...)
	}

	if r.group != nil {
		return r.group.Add(method, _path, handler)
	}
	return r.router.Add(method, _path, handler)
}

func (r echoRouter) SwaggerHandler(contentType string, blob []byte) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Add("Content-Type", contentType)
		return c.JSONBlob(http.StatusOK, blob)
	}
}

func (r echoRouter) TransformPathToOasPath(path string) string {
	// Echo handles path prefixes internally, so we don't need to prepend them here
	return apirouter.TransformPathParamsWithColon(path)
}

func (r echoRouter) Router(group bool) any {
	if r.group != nil && group {
		return r.group
	}
	return r.router
}

func NewRouter(router *echo.Echo) apirouter.Router[echo.HandlerFunc, echo.MiddlewareFunc, Route] {
	return echoRouter{
		router: router,
	}
}

func (r echoRouter) Group(pathPrefix string) apirouter.Router[echo.HandlerFunc, echo.MiddlewareFunc, Route] {
	var cleanPrefix string
	if pathPrefix != "" {
		cleanPrefix = strings.TrimPrefix(pathPrefix, "/")
		cleanPrefix = strings.TrimSuffix(cleanPrefix, "/")
		cleanPrefix = "/" + cleanPrefix
	}

	var newGroup *echo.Group
	if r.group != nil {
		newGroup = r.group.Group(cleanPrefix)
	} else {
		newGroup = r.router.Group(cleanPrefix)
	}

	return echoRouter{
		router: r.router,
		group:  newGroup,
	}
}

func (r echoRouter) Host(host string) apirouter.Router[echo.HandlerFunc, echo.MiddlewareFunc, Route] {
	// Echo doesn't natively support host-based routing, so we'll use a middleware
	// to filter requests by host
	hostRouter := r.router.Group("")
	hostRouter.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().Host != host {
				return echo.ErrNotFound
			}
			return next(c)
		}
	})
	return echoRouter{
		router: r.router,
		group:  hostRouter,
	}
}

func (r echoRouter) HasRoute(req *http.Request) (bool, string) {
	router := r.router.Router()
	c := r.router.AcquireContext()
	defer r.router.ReleaseContext(c)
	c.Reset(req, nil)

	router.Find(req.Method, echo.GetPath(req), c)

	handler := c.Handler()
	if handler == nil {
		return false, ""
	}

	// Get the pointer values (uintptr) for comparison
	handlerPtr404 := reflect.ValueOf(echo.NotFoundHandler).Pointer()
	handlerPtr405 := reflect.ValueOf(echo.MethodNotAllowedHandler).Pointer()
	handlerPtr := reflect.ValueOf(handler).Pointer()

	// Compare the pointer values
	if handlerPtr == handlerPtr404 || handlerPtr == handlerPtr405 {
		return false, ""
	}

	return true, ""
}

func (r echoRouter) Use(middleware ...echo.MiddlewareFunc) {
	if r.group != nil {
		r.group.Use(middleware...)
	} else {
		r.router.Use(middleware...)
	}
}
