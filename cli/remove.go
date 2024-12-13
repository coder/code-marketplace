package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/code-marketplace/storage"
	"github.com/coder/code-marketplace/util"
)

func remove() *cobra.Command {
	var (
		all bool
	)
	addFlags, opts := serverFlags()

	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove an extension from the marketplace",
		Example: strings.Join([]string{
			"  marketplace remove publisher.extension@1.0.0 --extensions-dir ./extensions",
			"  marketplace remove publisher.extension --all --artifactory http://artifactory.server/artifactory --repo extensions",
		}, "\n"),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			store, err := storage.NewStorage(ctx, opts)
			if err != nil {
				return err
			}

			targetId := args[0]
			publisher, name, versionStr, err := storage.ParseExtensionID(targetId)
			if err != nil {
				return err
			}

			version := storage.Version{Version: versionStr}
			if version.Version != "" && all {
				return xerrors.Errorf("cannot specify both --all and version %s", version)
			}

			allVersions, err := store.Versions(ctx, publisher, name)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}

			versionCount := len(allVersions)
			if version.Version == "" && !all {
				return xerrors.Errorf(
					"use %s@<version> to target a specific version or pass --all to delete %s of %s",
					targetId,
					util.Plural(versionCount, "version", ""),
					targetId,
				)
			}

			// TODO: Allow deleting by platform as well?
			var toDelete []storage.Version
			if all {
				toDelete = allVersions
			} else {
				for _, sv := range allVersions {
					if version.Version == sv.Version {
						toDelete = append(toDelete, sv)
					}
				}
			}
			if len(toDelete) == 0 {
				return xerrors.Errorf("%s does not exist", targetId)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Removing %s...\n", util.Plural(len(toDelete), "version", ""))
			var failed []string
			for _, delete := range toDelete {
				err = store.RemoveExtension(ctx, publisher, name, delete)
				if err != nil {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s (%s)\n", delete, err)
					failed = append(failed, delete.String())
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", delete)
				}
			}

			if len(failed) > 0 {
				return xerrors.Errorf(
					"Failed to remove %s: %s",
					util.Plural(len(failed), "version", ""),
					strings.Join(failed, ", "))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Whether to delete all versions of the extension.")
	addFlags(cmd)

	return cmd
}
