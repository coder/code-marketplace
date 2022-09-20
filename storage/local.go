package storage

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/mod/semver"

	"cdr.dev/slog"
)

// Local implements Storage.  It stores extensions locally on disk.
type Local struct {
	ExtDir string
	Logger slog.Logger
}

func (s *Local) FileServer() http.Handler {
	return http.FileServer(http.Dir(s.ExtDir))
}

func (s *Local) Manifest(ctx context.Context, publisher, extension, version string) (*VSIXManifest, error) {
	reader, err := os.Open(filepath.Join(s.ExtDir, publisher, extension, version, "extension.vsixmanifest"))
	if err != nil {
		return nil, err
	}

	return parseVSIXManifest(reader)
}

func (s *Local) WalkExtensions(ctx context.Context, fn func(manifest *VSIXManifest, versions []string) error) error {
	for _, publisher := range s.getDirNames(ctx, s.ExtDir) {
		ctx := slog.With(ctx, slog.F("publisher", publisher))
		dir := filepath.Join(s.ExtDir, publisher)

		for _, extension := range s.getDirNames(ctx, dir) {
			ctx := slog.With(ctx, slog.F("extension", extension))
			dir := filepath.Join(s.ExtDir, publisher, extension)

			versions := s.getDirNames(ctx, dir)
			sort.Sort(sort.Reverse(semver.ByVersion(versions)))
			if len(versions) == 0 {
				continue
			}

			// The manifest from the latest version is used for filtering.
			manifest, err := s.Manifest(ctx, publisher, extension, versions[0])
			if err != nil {
				s.Logger.Error(ctx, "Unable to read extension manifest", slog.Error(err))
				continue
			}

			if err = fn(manifest, versions); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Local) getDirNames(ctx context.Context, dir string) []string {
	files, err := os.ReadDir(dir)
	names := []string{}
	if err != nil {
		s.Logger.Error(ctx, "Error while reading publisher", slog.Error(err))
		// No return since ReadDir may still have returned files.
	}
	for _, file := range files {
		if file.IsDir() {
			names = append(names, file.Name())
		}
	}
	return names
}
