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

			// Read in the extension.  In the future we might support stdin as well.
			vsix, err := storage.ReadVSIX(ctx, args[0])
			if err != nil {
				return err
			}

			// The manifest is required to know where to place the extension since it
			// is unsafe to rely on the file name or URI.
			manifest, err := storage.ReadVSIXManifest(vsix)
			if err != nil {
				return err
			}

			// Always local storage for now.
			store := storage.NewLocalStorage(extdir, logger)
			location, err := store.AddExtension(ctx, manifest, vsix)
			if err != nil {
				return err
			}

			deps := []string{}
			pack := []string{}
			for _, prop := range manifest.Metadata.Properties.Property {
				if prop.Value == "" {
					continue
				}
				switch prop.ID {
				case storage.DependencyPropertyType:
					deps = append(deps, strings.Split(prop.Value, ",")...)
				case storage.PackPropertyType:
					pack = append(pack, strings.Split(prop.Value, ",")...)
				}
			}

			depCount := len(deps)
			id := storage.ExtensionID(manifest)
			summary := []string{
				fmt.Sprintf("Unpacked %s to %s", id, location),
				fmt.Sprintf("%s has %s", id, util.Plural(depCount, "dependency", "dependencies")),
			}

			if depCount > 0 {
				for _, id := range deps {
					summary = append(summary, fmt.Sprintf("  - %s", id))
				}
			}

			packCount := len(pack)
			if packCount > 0 {
				summary = append(summary, fmt.Sprintf("%s is in a pack with %s", id, util.Plural(packCount, "other extension", "")))
				for _, id := range pack {
					summary = append(summary, fmt.Sprintf("  - %s", id))
				}
			} else {
				summary = append(summary, fmt.Sprintf("%s is not in a pack", id))
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), strings.Join(summary, "\n"))
			return err
		},
	}

	cmd.Flags().StringVar(&extdir, "extensions-dir", "", "The path to extensions.")
	_ = cmd.MarkFlagRequired("extensions-dir")

	return cmd
}
