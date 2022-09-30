package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/code-marketplace/storage"
)

func remove() *cobra.Command {
	var (
		extdir string
		all    bool
	)

	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove an extension from the marketplace",
		Example: strings.Join([]string{
			"  marketplace remove publisher.extension-1.0.0 --extensions-dir ./extensions",
			"  marketplace remove publisher.extension --all --extensions-dir ./extensions",
		}, "\n"),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			verbose, err := cmd.Flags().GetBool("verbose")
			if err != nil {
				return err
			}
			logger := slog.Make(sloghuman.Sink(cmd.ErrOrStderr()))
			if verbose {
				logger = logger.Leveled(slog.LevelDebug)
			}

			extdir, err = filepath.Abs(extdir)
			if err != nil {
				return err
			}

			// Always local storage for now.
			store := &storage.Local{
				ExtDir: extdir,
				Logger: logger,
			}

			removed, err := store.RemoveExtension(ctx, args[0], all)
			if err != nil {
				return err
			}

			removedCount := len(removed)
			pluralVersions := "versions"
			if removedCount == 1 {
				pluralVersions = "version"
			}
			summary := []string{
				fmt.Sprintf("Removed %d %s", removedCount, pluralVersions),
			}
			for _, id := range removed {
				summary = append(summary, fmt.Sprintf("  - %s", id))
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), strings.Join(summary, "\n"))
			return err
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Whether to delete all versions of the extension.")
	cmd.Flags().StringVar(&extdir, "extensions-dir", "", "The path to extensions.")
	_ = cmd.MarkFlagRequired("extensions-dir")

	return cmd
}
