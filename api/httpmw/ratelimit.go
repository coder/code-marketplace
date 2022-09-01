package httpmw

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"

	"github.com/coder/code-marketplace/api/httpapi"
)

// RateLimitPerMinute returns a handler that limits requests per-minute based
// on IP and endpoint.
func RateLimitPerMinute(count int) func(http.Handler) http.Handler {
	// -1 is no rate limit
	if count <= 0 {
		return func(handler http.Handler) http.Handler {
			return handler
		}
	}
	return httprate.Limit(
		count,
		1*time.Minute,
		httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
			return httprate.KeyByIP(r)
		}, httprate.KeyByEndpoint),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(w, http.StatusTooManyRequests, httpapi.ErrorResponse{
				Message: "You have been rate limited!",
				Detail:  "Please wait a minute then try again.",
			})
		}),
	)
}
