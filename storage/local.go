package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"cdr.dev/slog"
)

// Local implements Storage.  It stores extensions locally on disk by both
// copying the VSIX and extracting said VSIX to a tree structure in the form of
// publisher/extension/version to easily serve individual assets via HTTP.
type Local struct {
	listCache      []extension
	listDuration   time.Duration
	listExpiration time.Time
	listMutex      sync.Mutex
	extdir         string
	logger         slog.Logger
}

type LocalOptions struct {
	// How long to cache the list of extensions with their manifests.  Zero means 
	// no cache.
	ListCacheDuration time.Duration
	ExtDir            string
}

func NewLocalStorage(options *LocalOptions, logger slog.Logger) (*Local, error) {
	extdir, err := filepath.Abs(options.ExtDir)
	if err != nil {
		return nil, err
	}
	return &Local{
		// TODO: Eject the cache when adding/removing extensions and/or add a
		// command to eject the cache?
		extdir:       extdir,
		listDuration: options.ListCacheDuration,
		logger:       logger,
	}, nil
}

func (s *Local) list(ctx context.Context) []extension {
	var list []extension
	publishers, err := s.getDirNames(ctx, s.extdir)
	if err != nil {
		s.logger.Error(ctx, "Error reading publisher", slog.Error(err))
	}
	for _, publisher := range publishers {
		ctx := slog.With(ctx, slog.F("publisher", publisher))
		dir := filepath.Join(s.extdir, publisher)

		extensions, err := s.getDirNames(ctx, dir)
		if err != nil {
			s.logger.Error(ctx, "Error reading extensions", slog.Error(err))
		}
		for _, name := range extensions {
			ctx := slog.With(ctx, slog.F("extension", name))
			versions, err := s.Versions(ctx, publisher, name)
			if err != nil {
				s.logger.Error(ctx, "Error reading versions", slog.Error(err))
			}
			if len(versions) == 0 {
				continue
			}

			// The manifest from the latest version is used for filtering.
			manifest, err := s.Manifest(ctx, publisher, name, versions[0])
			if err != nil {
				s.logger.Error(ctx, "Unable to read extension manifest", slog.Error(err))
				continue
			}

			list = append(list, extension{
				manifest,
				name,
				publisher,
				versions,
			})
		}
	}
	return list
}

func (s *Local) AddExtension(ctx context.Context, manifest *VSIXManifest, vsix []byte) (string, error) {
	// Extract the zip to the correct path.
	identity := manifest.Metadata.Identity
	dir := filepath.Join(s.extdir, identity.Publisher, identity.ID, Version{
		Version:        identity.Version,
		TargetPlatform: identity.TargetPlatform,
	}.String())
	err := ExtractZip(vsix, func(name string, r io.Reader) error {
		path := filepath.Join(dir, name)
		err := os.MkdirAll(filepath.Dir(path), 0o755)
		if err != nil {
			return err
		}
		w, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		defer w.Close()
		_, err = io.Copy(w, r)
		return err
	})
	if err != nil {
		return "", err
	}

	// Copy the VSIX itself as well.
	vsixPath := filepath.Join(dir, fmt.Sprintf("%s.vsix", ExtensionVSIXNameFromManifest(manifest)))
	err = os.WriteFile(vsixPath, vsix, 0o644)
	if err != nil {
		return "", err
	}

	return dir, nil
}

func (s *Local) FileServer() http.Handler {
	return http.FileServer(http.Dir(s.extdir))
}

func (s *Local) Manifest(ctx context.Context, publisher, name string, version Version) (*VSIXManifest, error) {
	reader, err := os.Open(filepath.Join(s.extdir, publisher, name, version.String(), "extension.vsixmanifest"))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// If the manifest is returned with an error that means it exists but is
	// invalid.  We will still return it as a best-effort.
	manifest, err := parseVSIXManifest(reader)
	if manifest == nil && err != nil {
		return nil, err
	} else if err != nil {
		s.logger.Error(ctx, "Extension has invalid manifest", slog.Error(err))
	}

	manifest.Assets.Asset = append(manifest.Assets.Asset, VSIXAsset{
		Type:        VSIXAssetType,
		Path:        fmt.Sprintf("%s.vsix", ExtensionVSIXNameFromManifest(manifest)),
		Addressable: "true",
	})

	return manifest, nil
}

func (s *Local) RemoveExtension(ctx context.Context, publisher, name string, version Version) error {
	dir := filepath.Join(s.extdir, publisher, name, version.String())
	// RemoveAll() will not error if the directory does not exist so check first
	// as this function should error when removing versions that do not exist.
	_, err := os.Stat(dir)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func (s *Local) Versions(ctx context.Context, publisher, name string) ([]Version, error) {
	dir := filepath.Join(s.extdir, publisher, name)
	versionDirs, err := s.getDirNames(ctx, dir)
	var versions []Version
	for _, versionDir := range versionDirs {
		versions = append(versions, VersionFromString(versionDir))
	}
	// Return anything we did get even if there was an error.
	sort.Sort(ByVersion(versions))
	return versions, err
}

func (s *Local) listWithCache(ctx context.Context) []extension {
	s.listMutex.Lock()
	defer s.listMutex.Unlock()
	if s.listCache == nil || time.Now().After(s.listExpiration) {
		s.listExpiration = time.Now().Add(s.listDuration)
		s.listCache = s.list(ctx)
	}
	return s.listCache
}

func (s *Local) WalkExtensions(ctx context.Context, fn func(manifest *VSIXManifest, versions []Version) error) error {
	// Walking through directories on disk and parsing manifest files takes several
	// minutes with many extensions installed, so if we already did that within
	// a specified duration, just load extensions from the cache instead.
	for _, extension := range s.listWithCache(ctx) {
		if err := fn(extension.manifest, extension.versions); err != nil {
			return err
		}
	}
	return nil
}

// getDirNames get the names of directories in the provided directory.  If an
// error is occured it will be returned along with any directories that were
// able to be read.
func (s *Local) getDirNames(ctx context.Context, dir string) ([]string, error) {
	files, err := os.ReadDir(dir)
	names := []string{}
	for _, file := range files {
		if file.IsDir() {
			names = append(names, file.Name())
		}
	}
	return names, err
}
