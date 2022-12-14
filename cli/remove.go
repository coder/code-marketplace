package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/code-marketplace/storage"
	"github.com/coder/code-marketplace/util"
)

func remove() *cobra.Command {
	var (
		all         bool
		artifactory string
		extdir      string
		repo        string
	)

	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove an extension from the marketplace",
		Example: strings.Join([]string{
			"  marketplace remove publisher.extension-1.0.0 --extensions-dir ./extensions",
			"  marketplace remove publisher.extension --all --artifactory http://artifactory.server/artifactory --repo extensions",
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

			store, err := storage.NewStorage(ctx, &storage.Options{
				Artifactory: artifactory,
				ExtDir:      extdir,
				Logger:      logger,
				Repo:        repo,
			})
			if err != nil {
				return err
			}

			id := args[0]
			publisher, name, version, err := storage.ParseExtensionID(id)
			if err != nil {
				return err
			}

			if version != "" && all {
				return xerrors.Errorf("cannot specify both --all and version %s", version)
			}

			allVersions, err := store.Versions(ctx, publisher, name)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}

			versionCount := len(allVersions)
			if !all && version != "" && !util.Contains(allVersions, version) {
				return xerrors.Errorf("%s does not exist", id)
			} else if versionCount == 0 {
				return xerrors.Errorf("%s.%s has no versions to delete", publisher, name)
			} else if version == "" && !all {
				return xerrors.Errorf(
					"use %s-<version> to target a specific version or pass --all to delete %s of %s",
					id,
					util.Plural(versionCount, "version", ""),
					id,
				)
			}
			err = store.RemoveExtension(ctx, publisher, name, version)
			if err != nil {
				return err
			}

			summary := []string{}
			if all {
				removedCount := len(allVersions)
				summary = append(summary, fmt.Sprintf("Removed %s", util.Plural(removedCount, "version", "")))
				for _, version := range allVersions {
					summary = append(summary, fmt.Sprintf("  - %s", version))
				}
			} else {
				summary = append(summary, fmt.Sprintf("Removed %s", version))
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), strings.Join(summary, "\n"))
			return err
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Whether to delete all versions of the extension.")
	cmd.Flags().StringVar(&extdir, "extensions-dir", "", "The path to extensions.")
	cmd.Flags().StringVar(&artifactory, "artifactory", "", "Artifactory server URL.")
	cmd.Flags().StringVar(&repo, "repo", "", "Artifactory repository.")

	return cmd
}
