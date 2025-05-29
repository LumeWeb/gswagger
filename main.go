package swagger

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/ghodss/yaml"
	"go.lumeweb.com/gswagger/apirouter"
)

var (
	ErrGenerateOAS       = errors.New("fail to generate openapi")
	ErrValidatingOAS     = errors.New("fails to validate openapi")
	ErrGenerateSwagger   = ErrGenerateOAS
	ErrValidatingSwagger = ErrValidatingOAS
)

func GetRouter[T any, H any, M any, R any](r apirouter.Router[H, M, R]) T {
	if r == nil || r.Router() == nil {
		var zero T
		return zero // Return zero value for the type T
	}
	return r.Router().(T)
}

const (
	// DefaultJSONDocumentationPath is the path of the openapi documentation in json format.
	DefaultJSONDocumentationPath = "/documentation/json"
	// DefaultYAMLDocumentationPath is the path of the openapi documentation in yaml format.
	DefaultYAMLDocumentationPath = "/documentation/yaml"
	defaultOpenapiVersion        = "3.0.0"
)

// Router provides API routing with integrated OpenAPI schema generation.
// Supports multiple router implementations (gorilla/mux, fiber, echo) with:
// - Host-specific routing with isolated schemas
// - Route grouping by path prefixes
// - Middleware chaining
// - Automatic OpenAPI documentation generation
type SubRouterOptions struct {
	PathPrefix string
}

// Router wraps framework routers while maintaining OpenAPI documentation.
//
// Type parameters:
//   - HandlerFunc: Framework-specific handler function type
//   - MiddlewareFunc: Framework-specific middleware function type
//   - Route: Framework-specific route type
type Router[HandlerFunc any, MiddlewareFunc any, Route any] struct {
	router apirouter.Router[HandlerFunc, MiddlewareFunc, Route]

	swaggerSchema *openapi3.T
	context       context.Context

	jsonDocumentationPath string
	yamlDocumentationPath string

	pathPrefix string

	host string

	rootRouter *Router[HandlerFunc, MiddlewareFunc, Route]

	hostRouters map[string]*Router[HandlerFunc, MiddlewareFunc, Route]

	defaultRouter *Router[HandlerFunc, MiddlewareFunc, Route]

	frameworkRouterFactory func() apirouter.Router[HandlerFunc, MiddlewareFunc, Route]

	isSubrouter bool
}

// Router returns the underlying router implementation for the current context (default, group, or host)
// Router returns the underlying framework-specific router instance.
// This allows accessing framework-specific functionality when needed.
func (r *Router[HandlerFunc, MiddlewareFunc, Route]) Router() apirouter.Router[HandlerFunc, MiddlewareFunc, Route] {
	return r.router
}

// Use adds middleware to the router that will be executed for all routes
// registered on this router instance. Middleware executes in the order they
// are added.
func (r *Router[HandlerFunc, MiddlewareFunc, Route]) Use(middleware ...MiddlewareFunc) {
	r.router.Use(middleware...)
}

// SubRouter creates a new router with the given path prefix.
// The new router shares the same OpenAPI schema and documentation paths as the parent,
// but routes are prefixed with the specified path.
func (r *Router[HandlerFunc, MiddlewareFunc, Route]) SubRouter(router apirouter.Router[HandlerFunc, MiddlewareFunc, Route], opts SubRouterOptions) (*Router[HandlerFunc, MiddlewareFunc, Route], error) {
	if r.rootRouter == nil {
		return nil, errors.New("SubRouter() can only be called on a router with rootRouter set")
	}

	return r.Group(opts.PathPrefix)
}

// Group creates a new router group with prefix and optional group-level middleware.
// Routes added to the returned router will inherit the parent's host and append the path prefix.
// Group creates a new router group with the given path prefix.
// Routes added to the group will have their paths prefixed with group's path.
// The group inherits the parent's host and shares the root OpenAPI schema.
// Returns an error if pathPrefix is invalid.
func (r *Router[HandlerFunc, MiddlewareFunc, Route]) Group(pathPrefix string) (*Router[HandlerFunc, MiddlewareFunc, Route], error) {
	apiGroupRouter := r.router.Group(pathPrefix)
	return &Router[HandlerFunc, MiddlewareFunc, Route]{
		router:                apiGroupRouter,
		swaggerSchema:         r.rootRouter.swaggerSchema,          // Share the root schema
		context:               r.rootRouter.context,                // Share the root context
		jsonDocumentationPath: r.rootRouter.jsonDocumentationPath,  // Share doc paths
		yamlDocumentationPath: r.rootRouter.yamlDocumentationPath,  // Share doc paths
		pathPrefix:            path.Join(r.pathPrefix, pathPrefix), // Append prefix
		host:                  r.host,                              // Inherit host from parent
		rootRouter:            r.rootRouter,                        // Reference the root router
		isSubrouter:           true,
	}, nil
}

// Host creates a new router instance configured for a specific host.
// This method must be called on the root router instance.
// Routes added to the returned router will only match requests for the specified host
// and will be documented with a server object for that host.
// Host creates a new router instance configured for a specific host.
// The host router maintains its own isolated OpenAPI schema while sharing
// documentation paths and context with the root router.
// Must be called on the root router instance.
// Returns an error if:
// - Called on non-root router
// - Host is empty
// - FrameworkRouterFactory is not set
func (r *Router[HandlerFunc, MiddlewareFunc, Route]) Host(host string) (*Router[HandlerFunc, MiddlewareFunc, Route], error) {
	if r.rootRouter != r {
		return nil, errors.New("Host() can only be called on the root router instance")
	}
	if host == "" {
		return nil, errors.New("Host name cannot be empty")
	}

	if existingRouter, ok := r.hostRouters[host]; ok {
		return existingRouter, nil
	}

	if r.frameworkRouterFactory == nil {
		return nil, errors.New("FrameworkRouterFactory is not set in NewRouter Options[gorilla.HandlerFunc, mux.MiddlewareFunc, gorilla.Route]")
	}
	newFrameworkRouter := r.frameworkRouterFactory()

	hostSchema := &openapi3.T{
		Info:    r.swaggerSchema.Info,
		OpenAPI: r.swaggerSchema.OpenAPI,
		Paths:   &openapi3.Paths{},
	}

	hostRouter := &Router[HandlerFunc, MiddlewareFunc, Route]{
		router:                newFrameworkRouter,
		swaggerSchema:         hostSchema,
		context:               r.context,
		jsonDocumentationPath: r.jsonDocumentationPath,
		yamlDocumentationPath: r.yamlDocumentationPath,
		pathPrefix:            "",
		host:                  host,
		rootRouter:            r,
	}

	r.hostRouters[host] = hostRouter

	return hostRouter, nil
}

// SwaggerSchema sets the OpenAPI schema for the router instance.
// This allows modifying the schema after router creation, particularly useful
// for host-specific routers where you want to customize the schema.
// Returns the router instance for method chaining.
func (r *Router[HandlerFunc, MiddlewareFunc, Route]) SwaggerSchema(schema *openapi3.T) *Router[HandlerFunc, MiddlewareFunc, Route] {
	r.swaggerSchema = schema
	return r
}

// SetInfo sets the OpenAPI Info struct (title, version, description etc) for the router.
// This allows modifying the API metadata after router creation.
// Returns the router instance for method chaining.
// If info is nil, the method is a no-op and returns the router unchanged.
func (r *Router[HandlerFunc, MiddlewareFunc, Route]) SetInfo(info *openapi3.Info) *Router[HandlerFunc, MiddlewareFunc, Route] {
	if info == nil {
		return r
	}
	r.swaggerSchema.Info = info
	return r
}

type Options[HandlerFunc any, MiddlewareFunc any, Route any] struct {
	Context context.Context
	Openapi *openapi3.T
	// JSONDocumentationPath is the path exposed by json endpoint. Default to /documentation/json.
	JSONDocumentationPath string
	// YAMLDocumentationPath is the path exposed by yaml endpoint. Default to /documentation/yaml.
	YAMLDocumentationPath string
	// Add path prefix to add to every router path.
	PathPrefix string
	// FrameworkRouterFactory is a function that creates a new instance of the underlying framework router.
	// This is required when using the Host() method to manage multiple host-specific routers.
	FrameworkRouterFactory func() apirouter.Router[HandlerFunc, MiddlewareFunc, Route]
}

func NewRouter[HandlerFunc, MiddlewareFunc, Route any](frameworkRouter apirouter.Router[HandlerFunc, MiddlewareFunc, Route], options Options[HandlerFunc, MiddlewareFunc, Route]) (*Router[HandlerFunc, MiddlewareFunc, Route], error) {
	openapi, err := generateNewValidOpenapi(options.Openapi)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrValidatingOAS, err)
	}

	var ctx = options.Context
	if options.Context == nil {
		ctx = context.Background()
	}

	yamlDocumentationPath := DefaultYAMLDocumentationPath
	if options.YAMLDocumentationPath != "" {
		if err := isValidDocumentationPath(options.YAMLDocumentationPath); err != nil {
			return nil, err
		}
		yamlDocumentationPath = options.YAMLDocumentationPath
	}

	jsonDocumentationPath := DefaultJSONDocumentationPath
	if options.JSONDocumentationPath != "" {
		if err := isValidDocumentationPath(options.JSONDocumentationPath); err != nil {
			return nil, err
		}
		jsonDocumentationPath = options.JSONDocumentationPath
	}

	defaultFrameworkRouterWithPrefix := frameworkRouter
	if options.PathPrefix != "" {
		defaultFrameworkRouterWithPrefix = frameworkRouter.Group(options.PathPrefix)
	}

	root := &Router[HandlerFunc, MiddlewareFunc, Route]{
		router:                 defaultFrameworkRouterWithPrefix,
		swaggerSchema:          openapi,
		context:                ctx,
		yamlDocumentationPath:  yamlDocumentationPath,
		jsonDocumentationPath:  jsonDocumentationPath,
		pathPrefix:             options.PathPrefix,
		host:                   "",
		rootRouter:             nil,
		hostRouters:            make(map[string]*Router[HandlerFunc, MiddlewareFunc, Route]),
		defaultRouter:          nil,
		frameworkRouterFactory: options.FrameworkRouterFactory,
	}
	root.rootRouter = root
	root.defaultRouter = root

	return root, nil
}

func (r *Router[HandlerFunc, MiddlewareFunc, Route]) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if r.rootRouter != r {
		http.Error(w, "Internal Server Error: ServeHTTP called on non-root router", http.StatusInternalServerError)
		return
	}

	host := req.Host
	var handlerRouter *Router[HandlerFunc, MiddlewareFunc, Route]

	if hostRouter, ok := r.hostRouters[host]; ok && hostRouter != nil {
		handlerRouter = hostRouter
	} else {
		handlerRouter = r.defaultRouter
	}

	if handlerRouter == nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	if req.URL.Path == handlerRouter.jsonDocumentationPath || req.URL.Path == handlerRouter.yamlDocumentationPath {
		if handler, ok := handlerRouter.router.Router().(http.Handler); ok {
			handler.ServeHTTP(w, req)
			return
		}
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if handler, ok := handlerRouter.router.Router().(http.Handler); ok {
		handler.ServeHTTP(w, req)
		return
	}

	http.Error(w, "Not Found", http.StatusNotFound)
}

// AddRoute adds a route with OpenAPI schema inferred from Definitions.
// Automatically handles path parameters, request bodies, and responses.
// The route is added to both the router and OpenAPI schema.
// Returns the framework-specific route object and any validation errors.

func generateNewValidOpenapi(openapi *openapi3.T) (*openapi3.T, error) {
	if openapi == nil {
		return nil, fmt.Errorf("openapi is required")
	}
	if openapi.OpenAPI == "" {
		openapi.OpenAPI = defaultOpenapiVersion
	}
	if openapi.Paths == nil {
		openapi.Paths = &openapi3.Paths{}
	}

	if openapi.Info == nil {
		return nil, fmt.Errorf("openapi info is required")
	}
	if openapi.Info.Title == "" {
		return nil, fmt.Errorf("openapi info title is required")
	}
	if openapi.Info.Version == "" {
		return nil, fmt.Errorf("openapi info version is required")
	}

	return openapi, nil
}

func (r *Router[HandlerFunc, MiddlewareFunc, Route]) GenerateAndExposeOpenapi() error {
	if r.host != "" {
		if err := r.swaggerSchema.Validate(r.context); err != nil {
			return fmt.Errorf("%w for host %s: %s", ErrValidatingOAS, r.host, err)
		}

		jsonDocumentationPath := path.Join(r.pathPrefix, r.jsonDocumentationPath)
		yamlDocumentationPath := path.Join(r.pathPrefix, r.yamlDocumentationPath)

		jsonSwagger, err := r.swaggerSchema.MarshalJSON()
		if err != nil {
			return fmt.Errorf("%w json marshal for host %s: %s", ErrGenerateOAS, r.host, err)
		}
		r.router.AddRoute(http.MethodGet, jsonDocumentationPath, r.router.SwaggerHandler("application/json", jsonSwagger))

		yamlSwagger, err := yaml.JSONToYAML(jsonSwagger)
		if err != nil {
			return fmt.Errorf("%w yaml marshal for host %s: %s", ErrGenerateOAS, r.host, err)
		}
		r.router.AddRoute(http.MethodGet, yamlDocumentationPath, r.router.SwaggerHandler("text/plain", yamlSwagger))
		return nil
	}

	if r.defaultRouter != nil {
		if err := r.defaultRouter.swaggerSchema.Validate(r.context); err != nil {
			return fmt.Errorf("%w: %s", ErrValidatingOAS, err)
		}

		jsonSwagger, err := r.defaultRouter.swaggerSchema.MarshalJSON()
		if err != nil {
			return fmt.Errorf("%w json marshal: %s", ErrGenerateOAS, err)
		}
		r.defaultRouter.router.AddRoute(http.MethodGet, r.jsonDocumentationPath, r.defaultRouter.router.SwaggerHandler("application/json", jsonSwagger))

		yamlSwagger, err := yaml.JSONToYAML(jsonSwagger)
		if err != nil {
			return fmt.Errorf("%w yaml marshal: %s", ErrGenerateOAS, err)
		}
		r.defaultRouter.router.AddRoute(http.MethodGet, r.yamlDocumentationPath, r.defaultRouter.router.SwaggerHandler("text/plain", yamlSwagger))
	}

	for host, hostRouter := range r.hostRouters {
		if err := hostRouter.swaggerSchema.Validate(r.context); err != nil {
			return fmt.Errorf("%w for host %s: %s", ErrValidatingOAS, host, err)
		}

		jsonSwagger, err := hostRouter.swaggerSchema.MarshalJSON()
		if err != nil {
			return fmt.Errorf("%w json marshal for host %s: %s", ErrGenerateOAS, host, err)
		}
		hostRouter.router.AddRoute(http.MethodGet, hostRouter.jsonDocumentationPath, hostRouter.router.SwaggerHandler("application/json", jsonSwagger))

		yamlSwagger, err := yaml.JSONToYAML(jsonSwagger)
		if err != nil {
			return fmt.Errorf("%w yaml marshal for host %s: %s", ErrGenerateOAS, host, err)
		}
		hostRouter.router.AddRoute(http.MethodGet, hostRouter.yamlDocumentationPath, hostRouter.router.SwaggerHandler("text/plain", yamlSwagger))
	}

	return nil
}

func isValidDocumentationPath(path string) error {
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("invalid path %s. Path should start with '/'", path)
	}
	return nil
}
