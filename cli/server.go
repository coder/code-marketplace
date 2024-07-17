package cli

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/code-marketplace/api"
	"github.com/coder/code-marketplace/database"
	"github.com/coder/code-marketplace/storage"
)

func server() *cobra.Command {
	var (
		address           string
		artifactory       string
		extdir            string
		repo              string
		listcacheduration time.Duration
		maxpagesize       int
	)

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start the Code extension marketplace",
		Example: strings.Join([]string{
			"  marketplace server --extensions-dir ./extensions",
			"  marketplace server --artifactory http://artifactory.server/artifactory --repo extensions",
		}, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			notifyCtx, notifyStop := signal.NotifyContext(ctx, interruptSignals...)
			defer notifyStop()

			verbose, err := cmd.Flags().GetBool("verbose")
			if err != nil {
				return err
			}
			logger := slog.Make(sloghuman.Sink(cmd.ErrOrStderr()))
			if verbose {
				logger = logger.Leveled(slog.LevelDebug)
			}

			store, err := storage.NewStorage(ctx, &storage.Options{
				Artifactory:       artifactory,
				ExtDir:            extdir,
				Logger:            logger,
				Repo:              repo,
				ListCacheDuration: listcacheduration,
			})
			if err != nil {
				return err
			}

			// A separate listener is required to get the resulting address (as
			// opposed to using http.ListenAndServe()).
			listener, err := net.Listen("tcp", address)
			if err != nil {
				return xerrors.Errorf("listen %q: %w", address, err)
			}
			defer listener.Close()
			tcpAddr, valid := listener.Addr().(*net.TCPAddr)
			if !valid {
				return xerrors.New("must be listening on tcp")
			}
			logger.Info(ctx, "Started API server", slog.F("address", tcpAddr))

			// Always no database for now.
			database := &database.NoDB{
				Storage: store,
				Logger:  logger,
			}

			// Start the API server.
			mapi := api.New(&api.Options{
				Database:    database,
				Storage:     store,
				Logger:      logger,
				MaxPageSize: maxpagesize,
			})
			server := &http.Server{
				Handler: mapi.Handler,
				BaseContext: func(_ net.Listener) context.Context {
					return ctx
				},
			}
			eg := errgroup.Group{}
			eg.Go(func() error {
				return server.Serve(listener)
			})
			errCh := make(chan error, 1)
			go func() {
				select {
				case errCh <- eg.Wait():
				default:
				}
			}()

			// Wait for an interrupt or error.
			var exitErr error
			select {
			case <-notifyCtx.Done():
				exitErr = notifyCtx.Err()
				logger.Info(ctx, "Interrupt caught, gracefully exiting...")
			case exitErr = <-errCh:
			}
			if exitErr != nil && !errors.Is(exitErr, context.Canceled) {
				logger.Error(ctx, "Unexpected error, shutting down server...", slog.Error(exitErr))
			}

			// Shut down the server.
			logger.Info(ctx, "Shutting down API server...")
			cancel() // Cancel in-flight requests since Shutdown() will not do this.
			timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = server.Shutdown(timeout)
			if err != nil {
				logger.Error(ctx, "API server shutdown took longer than 5s", slog.Error(err))
			} else {
				logger.Info(ctx, "Gracefully shut down API server\n")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&extdir, "extensions-dir", "", "The path to extensions.")
	cmd.Flags().IntVar(&maxpagesize, "max-page-size", 50, "The maximum number of pages to request")
	cmd.Flags().StringVar(&artifactory, "artifactory", "", "Artifactory server URL.")
	cmd.Flags().StringVar(&repo, "repo", "", "Artifactory repository.")
	cmd.Flags().StringVar(&address, "address", "127.0.0.1:3001", "The address on which to serve the marketplace API.")
	cmd.Flags().DurationVar(&listcacheduration, "list-cache-duration", time.Minute, "The duration of the extension cache.")

	return cmd
}
