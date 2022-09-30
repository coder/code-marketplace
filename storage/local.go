package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

func (s *Local) AddExtension(ctx context.Context, source string) (string, error) {
	vsixBytes, err := readVSIX(ctx, source)
	if err != nil {
		return "", err
	}

	mr, err := GetZipFileReader(vsixBytes, "extension.vsixmanifest")
	if err != nil {
		return "", err
	}
	defer mr.Close()

	manifest, err := parseVSIXManifest(mr)
	if err != nil {
		return "", err
	}

	err = validateManifest(manifest)
	if err != nil {
		return "", err
	}

	// Extract the zip to the correct path.
	identity := manifest.Metadata.Identity
	dir := filepath.Join(s.ExtDir, identity.Publisher, identity.ID, identity.Version)
	err = ExtractZip(vsixBytes, func(name string) (io.Writer, error) {
		path := filepath.Join(dir, name)
		err := os.MkdirAll(filepath.Dir(path), 0o755)
		if err != nil {
			return nil, err
		}
		return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	})
	if err != nil {
		return "", err
	}

	// Copy the VSIX itself as well.
	vsixName := fmt.Sprintf("%s.%s-%s.vsix", identity.Publisher, identity.ID, identity.Version)
	dst, err := os.OpenFile(filepath.Join(dir, vsixName), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(dst, bytes.NewReader(vsixBytes))
	if err != nil {
		return "", err
	}

	return dir, nil
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
