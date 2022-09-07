package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/code-marketplace/api/httpapi"
)

type TestResponse struct {
	Message string
}

func TestWrite(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		message := TestResponse{Message: "foo"}
		rw := httptest.NewRecorder()
		httpapi.Write(rw, http.StatusOK, message)
		require.Equal(t, http.StatusOK, rw.Code)

		var m TestResponse
		err := json.NewDecoder(rw.Body).Decode(&m)
		require.NoError(t, err)
		require.Equal(t, message, m)
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()

		message := httpapi.ErrorResponse{Message: "foo", Detail: "bar", RequestID: uuid.New()}
		rw := httptest.NewRecorder()
		httpapi.Write(rw, http.StatusMethodNotAllowed, message)
		require.Equal(t, http.StatusMethodNotAllowed, rw.Code)

		var m httpapi.ErrorResponse
		err := json.NewDecoder(rw.Body).Decode(&m)
		require.NoError(t, err)
		require.Equal(t, message, m)
	})

	t.Run("Malformed", func(t *testing.T) {
		t.Parallel()

		rw := httptest.NewRecorder()
		httpapi.Write(rw, http.StatusMethodNotAllowed, "no")
		// This will still be the original code since it was already set.
		require.Equal(t, http.StatusMethodNotAllowed, rw.Code)

		var m httpapi.ErrorResponse
		err := json.NewDecoder(rw.Body).Decode(&m)
		require.Error(t, err)
	})
}

func TestBaseURL(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest("GET", "/", nil)
	url, err := url.Parse("http://example.com/foo")
	require.NoError(t, err)
	require.Equal(t, *url, httpapi.RequestBaseURL(r, "/foo"))

	r.Header.Set(httpapi.XForwardedHostHeader, "foo.bar")
	r.Header.Set(httpapi.XForwardedProtoHeader, "qux")

	url, err = url.Parse("qux://foo.bar")
	require.NoError(t, err)
	require.Equal(t, *url, httpapi.RequestBaseURL(r, ""))

	url, err = url.Parse("qux://foo.bar")
	require.NoError(t, err)
	require.Equal(t, *url, httpapi.RequestBaseURL(r, "/"))

	r.Header.Set(httpapi.ForwardedHeader, "by=idk;for=idk;host=fred.thud;proto=baz")

	url, err = url.Parse("baz://fred.thud/quirk/bling")
	require.NoError(t, err)
	require.Equal(t, *url, httpapi.RequestBaseURL(r, "/quirk/bling"))
}
