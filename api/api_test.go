package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/code-marketplace/api"
	"github.com/coder/code-marketplace/api/httpapi"
	"github.com/coder/code-marketplace/database"
	"github.com/coder/code-marketplace/testutil"
)

func TestAPI(t *testing.T) {
	t.Parallel()

	exts := []*database.Extension{}
	for i := 0; i < 10; i++ {
		exts = append(exts, &database.Extension{
			ID: fmt.Sprintf("extension-%d", i),
		})
	}

	cases := []struct {
		Name     string
		Path     string
		Request  any
		Response any
		Status   int
		Method   string
	}{
		{
			Name:   "Root",
			Path:   "/",
			Status: http.StatusOK,
		},
		{
			Name:   "404",
			Path:   "/non-existent",
			Status: http.StatusNotFound,
		},
		{
			Name:   "Healthz",
			Path:   "/healthz",
			Status: http.StatusOK,
		},
		{
			Name:    "MalformedQuery",
			Path:    "/api/extensionquery",
			Status:  http.StatusBadRequest,
			Request: "foo",
			Response: &httpapi.ErrorResponse{
				Message: "Unable to read query",
				Detail:  "Check that the posted data is valid JSON",
			},
		},
		{
			Name:   "EmptyPayload",
			Path:   "/api/extensionquery",
			Status: http.StatusOK,
			Response: &api.QueryResponse{
				Results: []api.QueryResult{{
					Metadata: []api.ResultMetadata{{
						Type: "ResultCount",
						Items: []api.ResultMetadataItem{{
							Count: 0,
							Name:  "TotalCount",
						}},
					}},
				}},
			},
		},
		{
			Name:   "NoCriteria",
			Path:   "/api/extensionquery",
			Status: http.StatusOK,
			Request: &api.QueryRequest{
				Filters: []database.Filter{},
			},
			Response: &api.QueryResponse{
				Results: []api.QueryResult{{
					Metadata: []api.ResultMetadata{{
						Type: "ResultCount",
						Items: []api.ResultMetadataItem{{
							Count: 0,
							Name:  "TotalCount",
						}},
					}},
				}},
			},
		},
		{
			Name:   "ManyQueries",
			Path:   "/api/extensionquery",
			Status: http.StatusBadRequest,
			Request: &api.QueryRequest{
				Filters: make([]database.Filter, 2),
			},
			Response: &httpapi.ErrorResponse{
				Message: "Too many filters",
				Detail:  "Check that you only have one filter",
			},
		},
		{
			Name:   "HugePages",
			Path:   "/api/extensionquery",
			Status: http.StatusBadRequest,
			Request: &api.QueryRequest{
				Filters: []database.Filter{{
					PageSize: 500,
				}},
			},
			Response: &httpapi.ErrorResponse{
				Message: "The page size must be between 0 and 200",
				Detail:  "Contact an administrator to increase the page size",
			},
		},
		{
			Name:   "DBError",
			Path:   "/api/extensionquery",
			Status: http.StatusInternalServerError,
			Request: &api.QueryRequest{
				// testDB is configured to error if this flag is set.
				Flags: database.Unpublished,
			},
			Response: &httpapi.ErrorResponse{
				Message: "Internal server error while executing query",
				Detail:  "Contact an administrator with the request ID",
			},
		},
		{
			Name:   "GetExtensions",
			Path:   "/api/extensionquery",
			Status: http.StatusOK,
			Request: &api.QueryRequest{
				Filters: []database.Filter{{
					Criteria: []database.Criteria{{
						Type:  database.Target,
						Value: "Microsoft.VisualStudio.Code",
					}},
					PageNumber: 1,
					PageSize:   50,
				}},
			},
			Response: &api.QueryResponse{
				Results: []api.QueryResult{{
					Extensions: exts,
					Metadata: []api.ResultMetadata{{
						Type: "ResultCount",
						Items: []api.ResultMetadataItem{{
							Count: len(exts),
							Name:  "TotalCount",
						}},
					}},
				}},
			},
		},
		{
			Name:     "FileAPI",
			Path:     "/files/exists",
			Status:   http.StatusOK,
			Response: "foobar",
		},
		{
			Name:   "FileAPI",
			Path:   "/files/nonexistent",
			Status: http.StatusNotFound,
		},
		{
			Name:   "AssetError",
			Path:   "/assets/error/extension/version/type",
			Status: http.StatusInternalServerError,
			Response: &httpapi.ErrorResponse{
				Message: "Unable to read extension",
				Detail:  "Contact an administrator with the request ID",
			},
		},
		{
			Name:   "AssetNotExist",
			Path:   "/assets/notexist/extension/version/type",
			Status: http.StatusNotFound,
			Response: &httpapi.ErrorResponse{
				Message: "Extension asset does not exist",
				Detail:  "Please check the asset path",
			},
		},
		{
			Name:     "AssetOK",
			Path:     "/assets/publisher/extension/version/type",
			Status:   http.StatusMovedPermanently,
			Response: "/files/publisher/extension/version/foo",
		},
		{
			Name:     "AssetOKPlatform",
			Path:     "/assets/publisher/extension/version@linux-x64/type",
			Status:   http.StatusMovedPermanently,
			Response: "/files/publisher/extension/version@linux-x64/foo",
		},
		{
			Name:     "AssetOKPlatformQuery",
			Path:     "/assets/publisher/extension/version/type?targetPlatform=linux-x64",
			Status:   http.StatusMovedPermanently,
			Response: "/files/publisher/extension/version@linux-x64/foo",
		},
		{
			Name:     "AssetOKDuplicatedPlatformQuery",
			Path:     "/assets/publisher/extension/version@darwin-x64/type?targetPlatform=linux-x64",
			Status:   http.StatusMovedPermanently,
			Response: "/files/publisher/extension/version@darwin-x64/foo",
		},
		// Old vspackage path, for backwards compatibility.
		{
			Name:   "DownloadNotExist",
			Path:   "/publishers/notexist/vsextensions/extension/version/vspackage",
			Status: http.StatusNotFound,
			Response: &httpapi.ErrorResponse{
				Message: "Extension asset does not exist",
				Detail:  "Please check the asset path",
			},
		},
		{
			Name:     "DownloadOK",
			Path:     "/publishers/publisher/vsextensions/extension/version/vspackage",
			Status:   http.StatusMovedPermanently,
			Response: "/files/publisher/extension/version/extension.vsix",
		},
		// The vspackage path currently generated by VS Code.
		{
			Name:   "APIDownloadNotExist",
			Path:   "/api/publishers/notexist/vsextensions/extension/version/vspackage",
			Status: http.StatusNotFound,
			Response: &httpapi.ErrorResponse{
				Message: "Extension asset does not exist",
				Detail:  "Please check the asset path",
			},
			Method: http.MethodGet,
		},
		{
			Name:     "APIDownloadOK",
			Path:     "/api/publishers/publisher/vsextensions/extension/version/vspackage",
			Status:   http.StatusMovedPermanently,
			Response: "/files/publisher/extension/version/extension.vsix",
			Method:   http.MethodGet,
		},
		{
			Name:   "Item",
			Path:   "/item",
			Status: http.StatusOK,
		},
		{
			Name:   "WebExtensionStat",
			Path:   "/api/itemName/vscodevim.vim/version/1.23.1/statType/1/vscodewebextension",
			Status: http.StatusOK,
		},
		{
			Name:   "ExtensionStat",
			Path:   "/api/publishers/vscodevim/extensions/vim/1.23.1/stats?statType=1",
			Status: http.StatusOK,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
			apiServer := api.New(&api.Options{
				Database:    testutil.NewMockDB(exts),
				Storage:     testutil.NewMockStorage(),
				Logger:      logger,
				MaxPageSize: api.MaxPageSizeDefault,
			})

			server := httptest.NewServer(apiServer.Handler)
			defer server.Close()

			url := server.URL + c.Path

			// Do not follow redirects.
			client := &http.Client{
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}

			// Most /api calls are POSTs, the rest are GETs.
			var method = c.Method
			if method == "" {
				if strings.HasPrefix(c.Path, "/api") {
					method = http.MethodPost
				} else {
					method = http.MethodGet
				}
			}

			var resp *http.Response
			var err error
			if method == http.MethodPost {
				var body []byte
				if str, ok := c.Request.(string); ok {
					body = []byte(str)
				} else if c.Request != nil {
					body, err = json.Marshal(c.Request)
					require.NoError(t, err)
				}
				resp, err = client.Post(url, "application/json", bytes.NewReader(body))
			} else if method == http.MethodGet {
				resp, err = client.Get(url)
			} else {
				t.Fatal(method + " is not handled in the test yet, please add it now")
			}
			require.NoError(t, err)
			require.Equal(t, c.Status, resp.StatusCode)

			if c.Response != nil {
				// Copy the request ID so the objects can match.
				if a, aok := c.Response.(*httpapi.ErrorResponse); aok {
					var body httpapi.ErrorResponse
					err := json.NewDecoder(resp.Body).Decode(&body)
					require.NoError(t, err)
					a.RequestID = body.RequestID
					require.Equal(t, c.Response, &body)
				} else if c.Status == http.StatusMovedPermanently {
					require.Equal(t, c.Response, resp.Header.Get("Location"))
				} else if a, aok := c.Response.(string); aok {
					b, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					require.Equal(t, a, string(b))
				} else {
					var body api.QueryResponse
					err := json.NewDecoder(resp.Body).Decode(&body)
					require.NoError(t, err)
					require.Equal(t, c.Response, &body)
				}
			}
		})
	}
}
