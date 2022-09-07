package httpmw

import (
	"net/http"
	"time"

	"cdr.dev/slog"
	"github.com/coder/code-marketplace/api/httpapi"
)

func Logger(log slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &httpapi.StatusWriter{ResponseWriter: w}

			httplog := log.With(
				slog.F("path", r.URL.Path),
				slog.F("remote_addr", r.RemoteAddr),
				slog.F("client_id", r.Header.Get("x-market-client-id")),
				slog.F("user_id", r.Header.Get("x-market-user-id")),
			)

			next.ServeHTTP(sw, r)

			// Do not log successful health check requests.
			if r.URL.Path == "/healthz" && sw.Status == 200 {
				return
			}

			httplog = httplog.With(
				slog.F("took", time.Since(start)),
				slog.F("status_code", sw.Status),
				slog.F("latency_ms", float64(time.Since(start)/time.Millisecond)),
			)

			if sw.Status >= 400 {
				httplog = httplog.With(
					slog.F("response_body", string(sw.ResponseBody())),
				)
			}

			logLevelFn := httplog.Debug
			if sw.Status >= 400 {
				logLevelFn = httplog.Warn
			}
			if sw.Status >= 500 {
				// Server errors should be treated as an ERROR log level.
				logLevelFn = httplog.Error
			}

			logLevelFn(r.Context(), r.Method)
		})
	}
}
