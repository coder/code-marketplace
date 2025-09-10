package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/code-marketplace/api/httpmw"
)

func TestCors(t *testing.T) {
	t.Parallel()

	methods := []string{
		http.MethodOptions,
		http.MethodHead,
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}

	tests := []struct {
		name           string
		origin         string
		allowedOrigin  string
		headers        string
		allowedHeaders string
	}{
		{
			name:          "LocalHTTP",
			origin:        "http://localhost:3000",
			allowedOrigin: "*",
		},
		{
			name:          "LocalHTTPS",
			origin:        "https://localhost:3000",
			allowedOrigin: "*",
		},
		{
			name:          "HTTP",
			origin:        "http://code-server.domain.tld",
			allowedOrigin: "*",
		},
		{
			name:          "HTTPS",
			origin:        "https://code-server.domain.tld",
			allowedOrigin: "*",
		},
		{
			// VS Code appears to use this origin.
			name:          "VSCode",
			origin:        "vscode-file://vscode-app",
			allowedOrigin: "*",
		},
		{
			name:          "NoOrigin",
			allowedOrigin: "",
		},
		{
			name:           "Headers",
			origin:         "foobar",
			allowedOrigin:  "*",
			headers:        "X-TEST,X-TEST2",
			allowedHeaders: "X-Test, X-Test2",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			for _, method := range methods {
				method := method
				t.Run(method, func(t *testing.T) {
					t.Parallel()

					r := httptest.NewRequest(method, "http://dev.coder.com", nil)
					if test.origin != "" {
						r.Header.Set(httpmw.OriginHeader, test.origin)
					}

					// OPTIONS requests need to know what method will be requested, or
					// go-chi/cors will error.  Both request headers and methods should be
					// ignored for regular requests even if they are set, although that is
					// not tested here.
					if method == http.MethodOptions {
						r.Header.Set(httpmw.AccessControlRequestMethodHeader, http.MethodGet)
						if test.headers != "" {
							r.Header.Set(httpmw.AccessControlRequestHeadersHeader, test.headers)
						}
					}

					rw := httptest.NewRecorder()
					handler := httpmw.Cors()(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
						rw.WriteHeader(http.StatusNoContent)
					}))
					handler.ServeHTTP(rw, r)

					// Should always set some kind of allowed origin, if allowed.
					require.Equal(t, test.allowedOrigin, rw.Header().Get(httpmw.AccessControlAllowOriginHeader))

					// OPTIONS should echo back the request method and headers (if there
					// is an origin header set) and we should never get to our handler as
					// the middleware short-circuits with a 200.
					if method == http.MethodOptions && test.origin != "" {
						require.Equal(t, http.MethodGet, rw.Header().Get(httpmw.AccessControlAllowMethodsHeader))
						require.Equal(t, test.allowedHeaders, rw.Header().Get(httpmw.AccessControlAllowHeadersHeader))
						require.Equal(t, http.StatusOK, rw.Code)
					} else {
						require.Equal(t, "", rw.Header().Get(httpmw.AccessControlAllowMethodsHeader))
						require.Equal(t, "", rw.Header().Get(httpmw.AccessControlAllowHeadersHeader))
						require.Equal(t, http.StatusNoContent, rw.Code)
					}
				})
			}
		})
	}
}
