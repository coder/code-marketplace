package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/code-marketplace/storage"
	"github.com/coder/code-marketplace/util"
)

func add() *cobra.Command {
	addFlags, opts := serverFlags()

	cmd := &cobra.Command{
		Use:   "add <source>",
		Short: "Add an extension to the marketplace",
		Example: strings.Join([]string{
			"  marketplace add https://domain.tld/extension.vsix --extensions-dir ./extensions",
			"  marketplace add extension.vsix --artifactory http://artifactory.server/artifactory --repo extensions",
			"  marketplace add extension-vsixs/ --extensions-dir ./extensions",
		}, "\n"),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			store, err := storage.NewStorage(ctx, opts)
			if err != nil {
				return err
			}

			// The source might be a local directory with extensions.
			isDir := false
			if !strings.HasPrefix(args[0], "http://") && !strings.HasPrefix(args[0], "https://") {
				stat, err := os.Stat(args[0])
				if err != nil {
					return err
				}
				isDir = stat.IsDir()
			}

			var failed []string
			if isDir {
				files, err := os.ReadDir(args[0])
				if err != nil {
					return err
				}
				for _, file := range files {
					s, err := doAdd(ctx, filepath.Join(args[0], file.Name()), store)
					if err != nil {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Failed to unpack %s: %s\n", file.Name(), err.Error())
						failed = append(failed, file.Name())
					} else {
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), strings.Join(s, "\n"))
					}
				}
			} else {
				s, err := doAdd(ctx, args[0], store)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), strings.Join(s, "\n"))
			}

			if len(failed) > 0 {
				return xerrors.Errorf(
					"Failed to add %s: %s",
					util.Plural(len(failed), "extension", ""),
					strings.Join(failed, ", "))
			}
			return nil
		},
	}
	addFlags(cmd)

	return cmd
}

func doAdd(ctx context.Context, source string, store storage.Storage) ([]string, error) {
	// Read in the extension.  In the future we might support stdin as well.
	vsix, err := storage.ReadVSIX(ctx, source)
	if err != nil {
		return nil, err
	}

	// The manifest is required to know where to place the extension since it
	// is unsafe to rely on the file name or URI.
	manifest, err := storage.ReadVSIXManifest(vsix)
	if err != nil {
		return nil, err
	}

	location, err := store.AddExtension(ctx, manifest, vsix)
	if err != nil {
		return nil, err
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
	id := storage.ExtensionIDFromManifest(manifest)
	summary := []string{
		fmt.Sprintf("Unpacked %s to %s", id, location),
		fmt.Sprintf("  - %s has %s", id, util.Plural(depCount, "dependency", "dependencies")),
	}

	if depCount > 0 {
		for _, id := range deps {
			summary = append(summary, fmt.Sprintf("    - %s", id))
		}
	}

	packCount := len(pack)
	if packCount > 0 {
		summary = append(summary, fmt.Sprintf("  - %s is in a pack with %s", id, util.Plural(packCount, "other extension", "")))
		for _, id := range pack {
			summary = append(summary, fmt.Sprintf("    - %s", id))
		}
	} else {
		summary = append(summary, fmt.Sprintf("  - %s is not in a pack", id))
	}

	return summary, nil
}
