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
	"github.com/coder/code-marketplace/util"
)

func add() *cobra.Command {
	var (
		extdir string
	)

	cmd := &cobra.Command{
		Use:   "add <source>",
		Short: "Add an extension to the marketplace",
		Example: strings.Join([]string{
			"  marketplace add https://domain.tld/extension.vsix --extensions-dir ./extensions",
			"  marketplace add extension.vsix --extensions-dir ./extensions",
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

			ext, err := store.AddExtension(ctx, args[0])
			if err != nil {
				return err
			}

			depCount := len(ext.Dependencies)
			summary := []string{
				fmt.Sprintf("Unpacked %s to %s", ext.ID, ext.Location),
				fmt.Sprintf("%s has %s", ext.ID, util.Plural(depCount, "dependency", "dependencies")),
			}

			if depCount > 0 {
				for _, id := range ext.Dependencies {
					summary = append(summary, fmt.Sprintf("  - %s", id))
				}
			}

			packCount := len(ext.Pack)
			if packCount > 0 {
				summary = append(summary, fmt.Sprintf("%s is in a pack with %s", ext.ID, util.Plural(packCount, "other extension", "")))
				for _, id := range ext.Pack {
					summary = append(summary, fmt.Sprintf("  - %s", id))
				}
			} else {
				summary = append(summary, fmt.Sprintf("%s is not in a pack", ext.ID))
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), strings.Join(summary, "\n"))
			return err
		},
	}

	cmd.Flags().StringVar(&extdir, "extensions-dir", "", "The path to extensions.")
	_ = cmd.MarkFlagRequired("extensions-dir")

	return cmd
}
