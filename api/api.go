package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"cdr.dev/slog"
	"github.com/coder/code-marketplace/api/httpmw"
)

type Options struct {
	ExtDir string
	Logger slog.Logger
	// Set to <0 to disable.
	RateLimit int
}

type API struct {
	Handler http.Handler
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

	r.Get("/", func(rw http.ResponseWriter, r *http.Request) {
		httpapi.WriteBytes(rw, http.StatusOK, []byte("Marketplace is running"))
	})

	r.Get("/healthz", func(rw http.ResponseWriter, r *http.Request) {
		httpapi.WriteBytes(rw, http.StatusOK, []byte("API server running"))
	})

	return &API{
		Handler: r,
	}
}
