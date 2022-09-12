package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"cdr.dev/slog"
	"github.com/coder/code-marketplace/api/httpapi"
	"github.com/coder/code-marketplace/api/httpmw"
	"github.com/coder/code-marketplace/database"
)

// QueryRequest implements an untyped object.  It is the data sent to the API to
// query for extensions.
// https://github.com/microsoft/vscode/blob/a69f95fdf3dc27511517eef5ff62b21c7a418015/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L338-L342
type QueryRequest struct {
	Filters []database.Filter `json:"filters"`
	Flags   database.Flag     `json:"flags"`
}

// QueryResponse implements IRawGalleryQueryResult.  This is the response sent
// to extension queries.
// https://github.com/microsoft/vscode/blob/29234f0219bdbf649d6107b18651a1038d6357ac/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L81-L92
type QueryResponse struct {
	Results []QueryResult `json:"results"`
}

// QueryResult implements IRawGalleryQueryResult.results.
// https://github.com/microsoft/vscode/blob/29234f0219bdbf649d6107b18651a1038d6357ac/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L82-L91
type QueryResult struct {
	Extensions []*database.Extension `json:"extensions"`
	Metadata   []ResultMetadata      `json:"resultMetadata"`
}

// ResultMetadata implements IRawGalleryQueryResult.resultMetadata.
// https://github.com/microsoft/vscode/blob/29234f0219bdbf649d6107b18651a1038d6357ac/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L84-L90
type ResultMetadata struct {
	Type  string               `json:"metadataType"`
	Items []ResultMetadataItem `json:"metadataItems"`
}

// ResultMetadataItem implements IRawGalleryQueryResult.metadataItems.
// https://github.com/microsoft/vscode/blob/29234f0219bdbf649d6107b18651a1038d6357ac/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L86-L89
type ResultMetadataItem struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Options struct {
	Database database.Database
	// TODO: Abstract file storage for use with storage services like jFrog.
	ExtDir string
	Logger slog.Logger
	// Set to <0 to disable.
	RateLimit int
}

type API struct {
	Database database.Database
	Handler  http.Handler
	Logger   slog.Logger
}

// New creates a new API server.
func New(options *Options) *API {
	if options.RateLimit == 0 {
		options.RateLimit = 512
	}

	r := chi.NewRouter()

	cors := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"POST", "GET", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
		MaxAge:           300,
	})

	r.Use(
		cors.Handler,
		httpmw.RateLimitPerMinute(options.RateLimit),
		middleware.GetHead,
		httpmw.AttachRequestID,
		httpmw.Recover(options.Logger),
		httpmw.AttachBuildInfo,
		httpmw.Logger(options.Logger),
	)

	api := &API{
		Database: options.Database,
		Handler:  r,
		Logger:   options.Logger,
	}

	r.Get("/", func(rw http.ResponseWriter, r *http.Request) {
		httpapi.WriteBytes(rw, http.StatusOK, []byte("Marketplace is running"))
	})

	r.Get("/healthz", func(rw http.ResponseWriter, r *http.Request) {
		httpapi.WriteBytes(rw, http.StatusOK, []byte("API server running"))
	})

	// TODO: Read API version header and output a warning if it has changed since
	// that could indicate something needs to be updated.
	r.Post("/api/extensionquery", api.extensionQuery)

	// Endpoint for getting an extension's files or the extension zip.
	options.Logger.Info(context.Background(), "Serving files", slog.F("dir", options.ExtDir))
	r.Mount("/files", http.StripPrefix("/files", http.FileServer(http.Dir(options.ExtDir))))

	// VS Code can use the files in the response to get file paths but it will
	// sometimes ignore that and use use requests to /assets with hardcoded
	// types to get files.
	r.Get("/assets/{publisher}/{extension}/{version}/{type}", api.assetRedirect)

	// This is the "download manually" URL, which like /assets is hardcoded and
	// ignores the VSIX asset URL provided to VS Code in the response.
	r.Get("/publishers/{publisher}/vsextensions/{extension}/{version}/{type}", api.assetRedirect)

	return api
}

func (api *API) extensionQuery(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var query QueryRequest
	if r.ContentLength <= 0 {
		query = QueryRequest{}
	} else {
		err := json.NewDecoder(r.Body).Decode(&query)
		if err != nil {
			httpapi.Write(rw, http.StatusBadRequest, httpapi.ErrorResponse{
				Message:   "Unable to read query",
				Detail:    "Check that the posted data is valid JSON",
				RequestID: httpmw.RequestID(r),
			})
			return
		}
	}

	// Validate query sizes.
	if len(query.Filters) == 0 {
		query.Filters = append(query.Filters, database.Filter{})
	} else if len(query.Filters) > 1 {
		// VS Code always seems to use one filter.
		httpapi.Write(rw, http.StatusBadRequest, httpapi.ErrorResponse{
			Message:   "Too many filters",
			Detail:    "Check that you only have one filter",
			RequestID: httpmw.RequestID(r),
		})
	}
	for _, filter := range query.Filters {
		if filter.PageSize < 0 || filter.PageSize > 50 {
			httpapi.Write(rw, http.StatusBadRequest, httpapi.ErrorResponse{
				Message:   "Invalid page size",
				Detail:    "Check that the page size is between zero and fifty",
				RequestID: httpmw.RequestID(r),
			})
		}
	}

	baseURL := httpapi.RequestBaseURL(r, "/")

	// Each filter gets its own entry in the results.
	results := []QueryResult{}
	for _, filter := range query.Filters {
		extensions, count, err := api.Database.GetExtensions(ctx, filter, query.Flags, baseURL)
		if err != nil {
			api.Logger.Error(ctx, "Unable to execute query", slog.Error(err))
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.ErrorResponse{
				Message:   "Internal server error while executing query",
				Detail:    "Contact an administrator with the request ID",
				RequestID: httpmw.RequestID(r),
			})
			return
		}

		api.Logger.Debug(ctx, "Got extensions for filter",
			slog.F("filter", filter),
			slog.F("count", count))

		results = append(results, QueryResult{
			Extensions: extensions,
			Metadata: []ResultMetadata{{
				Type: "ResultCount",
				Items: []ResultMetadataItem{{
					Count: count,
					Name:  "TotalCount",
				}},
			}},
		})
	}

	httpapi.Write(rw, http.StatusOK, QueryResponse{Results: results})
}

func (api *API) assetRedirect(rw http.ResponseWriter, r *http.Request) {
	// TODO: Asset URIs can contain a targetPlatform query variable.
	baseURL := httpapi.RequestBaseURL(r, "/")
	assetType := chi.URLParam(r, "type")
	if assetType == "vspackage" {
		assetType = database.ExtensionAssetType
	}
	url, err := api.Database.GetExtensionAssetPath(r.Context(), &database.Asset{
		Extension: chi.URLParam(r, "extension"),
		Publisher: chi.URLParam(r, "publisher"),
		Type:      assetType,
		Version:   chi.URLParam(r, "version"),
	}, baseURL)
	if err != nil && os.IsNotExist(err) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.ErrorResponse{
			Message:   "Extension asset does not exist",
			Detail:    "Please check the asset path",
			RequestID: httpmw.RequestID(r),
		})
		return
	} else if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.ErrorResponse{
			Message:   "Unable to read extension",
			Detail:    "Contact an administrator with the request ID",
			RequestID: httpmw.RequestID(r),
		})
		return
	}

	http.Redirect(rw, r, url, http.StatusMovedPermanently)
}
