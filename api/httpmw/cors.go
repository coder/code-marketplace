package httpmw

import (
	"net/http"

	"github.com/go-chi/cors"
)

const (
	// Server headers.
	AccessControlAllowOriginHeader      = "Access-Control-Allow-Origin"
	AccessControlAllowCredentialsHeader = "Access-Control-Allow-Credentials"
	AccessControlAllowMethodsHeader     = "Access-Control-Allow-Methods"
	AccessControlAllowHeadersHeader     = "Access-Control-Allow-Headers"
	VaryHeader                          = "Vary"

	// Client headers.
	OriginHeader                      = "Origin"
	AccessControlRequestMethodHeader  = "Access-Control-Request-Method"
	AccessControlRequestHeadersHeader = "Access-Control-Request-Headers"
)

func Cors() func(next http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{
			http.MethodHead,
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
		},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}
