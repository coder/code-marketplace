package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/code-marketplace/api/httpapi"
	"github.com/coder/code-marketplace/api/httpmw"
)

func TestRecover(t *testing.T) {
	t.Parallel()

	handler := func(isPanic, hijack bool) http.HandlerFunc {
		return func(rw http.ResponseWriter, r *http.Request) {
			if isPanic {
				panic("Oh no!")
			}

			rw.WriteHeader(http.StatusOK)
		}
	}

	cases := []struct {
		Name   string
		Code   int
		Panic  bool
		Hijack bool
	}{
		{
			Name:   "OK",
			Code:   http.StatusOK,
			Panic:  false,
			Hijack: false,
		},
		{
			Name:   "Panic",
			Code:   http.StatusInternalServerError,
			Panic:  true,
			Hijack: false,
		},
		{
			Name:   "Hijack",
			Code:   0,
			Panic:  true,
			Hijack: true,
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			var (
				log = slogtest.Make(t, nil)
				rtr = chi.NewRouter()
				r   = httptest.NewRequest("GET", "/", nil)
				sw  = &httpapi.StatusWriter{
					ResponseWriter: httptest.NewRecorder(),
					Hijacked:       c.Hijack,
				}
			)

			rtr.Use(httpmw.AttachRequestID, httpmw.Recover(log))
			rtr.Get("/", handler(c.Panic, c.Hijack))
			rtr.ServeHTTP(sw, r)

			require.Equal(t, c.Code, sw.Status)
		})
	}
}
