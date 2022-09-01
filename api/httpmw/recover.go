package httpmw

import (
	"context"
	"net/http"
	"runtime/debug"

	"cdr.dev/slog"
	"github.com/coder/code-marketplace/api/httpapi"
)

func Recover(log slog.Logger) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			defer func() {
				err := recover()
				if err != nil {
					log.Warn(context.Background(),
						"panic serving http request (recovered)",
						slog.F("panic", err),
						slog.F("stack", string(debug.Stack())),
					)

					var hijacked bool
					if sw, ok := rw.(*httpapi.StatusWriter); ok {
						hijacked = sw.Hijacked
					}

					if !hijacked {
						httpapi.Write(rw, http.StatusInternalServerError, httpapi.ErrorResponse{
							Message:   "An internal server error occurred.",
							Detail:    "Application recovered from a panic",
							RequestID: RequestID(r),
						})
					}
				}
			}()

			h.ServeHTTP(rw, r)
		})
	}
}
