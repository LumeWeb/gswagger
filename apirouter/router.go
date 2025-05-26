package apirouter

type Router[HandlerFunc any, MiddlewareFunc any, Route any] interface {
	AddRoute(method string, path string, handler HandlerFunc, middleware ...MiddlewareFunc) Route
	SwaggerHandler(contentType string, blob []byte) HandlerFunc
	TransformPathToOasPath(path string) string
	Router() any
	Group(pathPrefix string) Router[HandlerFunc, MiddlewareFunc, Route]
	Host(host string) Router[HandlerFunc, MiddlewareFunc, Route]
	Use(middleware ...MiddlewareFunc)
}
