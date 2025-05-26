package gorilla

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/gswagger/apirouter"
)

func TestGorillaMuxRouter(t *testing.T) {
	muxRouter := mux.NewRouter()
	ar := NewRouter(muxRouter)

	t.Run("middleware is applied via Use", func(t *testing.T) {
		middlewareCalled := false
		middleware := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				middlewareCalled = true
				next.ServeHTTP(w, r)
			})
		}

		ar.Use(middleware)
		ar.AddRoute(http.MethodGet, "/middleware", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/middleware", nil)
		muxRouter.ServeHTTP(w, r)

		require.True(t, middlewareCalled)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)
	})

	t.Run("middleware is applied via AddRoute", func(t *testing.T) {
		// Create a fresh router for this test to avoid middleware leaking
		testRouter := mux.NewRouter()
		testAR := NewRouter(testRouter)

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

		testAR.AddRoute(http.MethodGet, "/route-mw", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, mw1, mw2)

		// Test middleware is called for the intended route
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/route-mw", nil)
		testRouter.ServeHTTP(w, r)

		require.True(t, mw1Called)
		require.True(t, mw2Called)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)

		// Verify middleware isn't called for other routes
		mw1Called = false
		mw2Called = false
		w = httptest.NewRecorder()
		r = httptest.NewRequest(http.MethodGet, "/other-route", nil)
		testRouter.ServeHTTP(w, r)
		require.False(t, mw1Called)
		require.False(t, mw2Called)
	})

	t.Run("create a new api router", func(t *testing.T) {
		require.Implements(t, (*apirouter.Router[HandlerFunc, mux.MiddlewareFunc, Route])(nil), ar)
	})

	t.Run("add new route", func(t *testing.T) {
		route := ar.AddRoute(http.MethodGet, "/foo", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(200)
			w.Write(nil)
		})
		require.IsType(t, route, &mux.Route{})

		t.Run("router exposes correctly api", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/foo", nil)

			muxRouter.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Result().StatusCode)
		})

		t.Run("router exposes api only to the specific method", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/foo", nil)

			muxRouter.ServeHTTP(w, r)

			require.Equal(t, http.StatusMethodNotAllowed, w.Result().StatusCode)
		})
	})

	t.Run("create openapi handler", func(t *testing.T) {
		handlerFunc := ar.SwaggerHandler("text/html", []byte("some data"))
		muxRouter.HandleFunc("/oas", handlerFunc).Methods(http.MethodGet)

		t.Run("responds correctly to the API", func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/oas", nil)

			muxRouter.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Result().StatusCode)
			require.Equal(t, "text/html", w.Result().Header.Get("Content-Type"))

			body, err := io.ReadAll(w.Result().Body)
			require.NoError(t, err)
			require.Equal(t, "some data", string(body))
		})
	})
}
