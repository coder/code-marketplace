package api_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/code-marketplace/api"
)

func TestServer(t *testing.T) {
	t.Parallel()

	api := api.New(&api.Options{
		Logger: slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		ExtDir: filepath.Join(t.TempDir(), "extensions"),
	})

	server := httptest.NewServer(api.Handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/non-existent")
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	resp, err = http.Get(server.URL)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get(server.URL + "/healthz")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
