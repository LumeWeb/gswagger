package gorilla

import (
	"go.lumeweb.com/gswagger/apirouter"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// HandlerFunc is the http type handler used by gorilla/mux
type HandlerFunc func(w http.ResponseWriter, req *http.Request)
type Route = *mux.Route

var _ apirouter.Router[HandlerFunc, mux.MiddlewareFunc, Route] = (*gorillaRouter)(nil)

func NewRouter(router *mux.Router) apirouter.Router[HandlerFunc, mux.MiddlewareFunc, Route] {
	return gorillaRouter{
		router:  router,
		inGroup: false,
	}
}

func (r gorillaRouter) Use(middleware ...mux.MiddlewareFunc) {
	r.router.Use(middleware...)
}

type gorillaRouter struct {
	router     *mux.Router // Can be main router or subrouter
	pathPrefix string
	inGroup    bool // Flag to prevent infinite recursion
}

func (r gorillaRouter) AddRoute(method string, path string, handler HandlerFunc, middleware ...mux.MiddlewareFunc) Route {
	// Use group API consistently for middleware handling
	if len(middleware) > 0 && !r.inGroup {
		group := r.Group("")
		group.Use(middleware...)
		return group.AddRoute(method, path, handler)
	}
	return r.router.HandleFunc(path, handler).Methods(method)
}

func (r gorillaRouter) SwaggerHandler(contentType string, blob []byte) HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		w.Write(blob)
	}
}

func (r gorillaRouter) TransformPathToOasPath(path string) string {
	return path
}

func (r gorillaRouter) Router() any {
	return r.router
}

func (r gorillaRouter) Group(pathPrefix string) apirouter.Router[HandlerFunc, mux.MiddlewareFunc, Route] {
	// Ensure path prefix starts with / and doesn't end with /
	cleanPrefix := strings.TrimPrefix(pathPrefix, "/")
	cleanPrefix = strings.TrimSuffix(cleanPrefix, "/")

	fullPrefix := "/" + cleanPrefix
	if r.pathPrefix != "" {
		fullPrefix = r.pathPrefix + fullPrefix
	}

	// Create the subrouter using NewRoute().PathPrefix()
	muxSubrouter := r.router.NewRoute().PathPrefix(fullPrefix).Subrouter()
	muxSubrouter.StrictSlash(true)
	return gorillaRouter{
		router:     muxSubrouter,
		pathPrefix: fullPrefix,
		inGroup:    true, // Mark as being in a group
	}
}

func (r gorillaRouter) Host(host string) apirouter.Router[HandlerFunc, mux.MiddlewareFunc, Route] {
	hostRouter := r.router.Host(host).Subrouter()
	return gorillaRouter{
		router: hostRouter,
	}
}
