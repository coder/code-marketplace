package cli

import (
	"context"
	"fmt"

	"cdr.dev/slog"
	"github.com/spf13/cobra"

	"github.com/coder/code-marketplace/storage"
)

func sign() *cobra.Command {
	addFlags, opts := serverFlags()
	cmd := &cobra.Command{
		Use:   "sign",
		Short: "Signs all existing extensions.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()
			logger := cmdLogger(cmd)

			store, err := storage.NewStorage(ctx, opts)
			if err != nil {
				return err
			}

			err = store.WalkExtensions(ctx, func(manifest *storage.VSIXManifest, versions []storage.Version) error {
				logger.Info(ctx, fmt.Sprint("Extension refreshed"))
				for _, asset := range manifest.Assets.Asset {
					if asset.Type == storage.VSIXAssetType {
						// Find the vsix file to reupload
						//store.Open(ctx, asset)
					}
				}

				_, err := store.AddExtension(ctx, manifest, vsixFile)
				if err != nil {
					logger.Error(ctx, "Failed to add extension", slog.Error(err))
				}
				return nil
			})
			if err != nil {
				return err
			}

			return nil
		},
	}
	addFlags(cmd)

	return cmd
}
