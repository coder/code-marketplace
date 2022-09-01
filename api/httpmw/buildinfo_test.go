package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/coder/code-marketplace/api/httpmw"
	"github.com/coder/code-marketplace/buildinfo"
)

func TestBuildInfo(t *testing.T) {
	t.Parallel()

	rtr := chi.NewRouter()
	rtr.Use(httpmw.AttachBuildInfo)
	rtr.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r := httptest.NewRequest("GET", "/", nil)
	rw := httptest.NewRecorder()
	rtr.ServeHTTP(rw, r)

	res := rw.Result()
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, buildinfo.Version(), res.Header.Get("Build-Version"))
}
