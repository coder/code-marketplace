package httpmw

import (
	"net/http"

	"github.com/coder/code-marketplace/buildinfo"
)

// AttachBuildInfo adds a build info header to each HTTP request.
func AttachBuildInfo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Build-Version", buildinfo.Version())
		next.ServeHTTP(rw, r)
	})
}
