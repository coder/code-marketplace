package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/code-marketplace/util"
)

// Local implements Storage.  It stores extensions locally on disk.
type Local struct {
	ExtDir string
	Logger slog.Logger
}

func NewLocalStorage(_ context.Context, extdir string, logger slog.Logger) *Local {
	return &Local{
		ExtDir: extdir,
		Logger: logger,
	}
}
func (s *Local) AddExtension(ctx context.Context, vsix []byte) (*Extension, error) {
	mr, err := GetZipFileReader(vsix, "extension.vsixmanifest")
	if err != nil {
		return nil, err
	}
	defer mr.Close()

	manifest, err := parseVSIXManifest(mr)
	if err != nil {
		return nil, err
	}

	err = validateManifest(manifest)
	if err != nil {
		return nil, err
	}

	// Extract the zip to the correct path.
	identity := manifest.Metadata.Identity
	dir := filepath.Join(s.ExtDir, identity.Publisher, identity.ID, identity.Version)
	err = ExtractZip(vsix, func(name string) (io.WriteCloser, error) {
		path := filepath.Join(dir, name)
		err := os.MkdirAll(filepath.Dir(path), 0o755)
		if err != nil {
			return nil, err
		}
		return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	})
	if err != nil {
		return nil, err
	}

	// Copy the VSIX itself as well.
	id := extensionID(manifest)
	vsixName := fmt.Sprintf("%s.vsix", id)
	dst, err := os.OpenFile(filepath.Join(dir, vsixName), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	defer dst.Close()
	_, err = io.Copy(dst, bytes.NewReader(vsix))
	if err != nil {
		return nil, err
	}

	ext := &Extension{ID: id, Location: dir}
	for _, prop := range manifest.Metadata.Properties.Property {
		if prop.Value == "" {
			continue
		}
		switch prop.ID {
		case DependencyPropertyType:
			ext.Dependencies = append(ext.Dependencies, strings.Split(prop.Value, ",")...)
		case PackPropertyType:
			ext.Pack = append(ext.Pack, strings.Split(prop.Value, ",")...)
		}
	}

	return ext, nil
}

func (s *Local) RemoveExtension(ctx context.Context, id string, all bool) ([]string, error) {
	re := regexp.MustCompile(`^([^.]+)\.([^-]+)-?(.*)$`)
	match := re.FindAllStringSubmatch(id, -1)
	if match == nil {
		return nil, xerrors.Errorf("expected ID in the format <publisher>.<name> or <publisher>.<name>-<version> but got invalid ID \"%s\"", id)
	}

	// Get the directory to delete.
	publisher := match[0][1]
	extension := match[0][2]
	version := match[0][3]
	dir := filepath.Join(s.ExtDir, publisher, extension)
	if !all {
		dir = filepath.Join(dir, version)
	}

	// We could avoid an error if extensions already do not exist but since we are
	// explicitly being asked to remove an extension the extension not being there
	// to be removed could be considered an error.
	_, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, xerrors.Errorf("%s does not exist", id)
		}
		return nil, err
	}

	allVersions := s.getDirNames(ctx, dir)
	versionCount := len(allVersions)

	// TODO: Probably should use a custom error instance since knowledge of --all
	// is weird here.
	if version != "" && all {
		return nil, xerrors.Errorf("cannot specify both --all and version %s", version)
	} else if version == "" && !all {
		return nil, xerrors.Errorf(
			"use %s-<version> to target a specific version or pass --all to delete %s of %s",
			id,
			util.Plural(versionCount, "version", ""),
			id,
		)
	}

	err = os.RemoveAll(dir)
	if err != nil {
		return nil, err
	}

	var versions []string
	if all {
		versions = allVersions
	} else {
		versions = []string{version}
	}
	sort.Sort(sort.Reverse(semver.ByVersion(versions)))
	return versions, nil
}

func (s *Local) FileServer() http.Handler {
	return http.FileServer(http.Dir(s.ExtDir))
}

func (s *Local) Manifest(ctx context.Context, publisher, extension, version string) (*VSIXManifest, error) {
	reader, err := os.Open(filepath.Join(s.ExtDir, publisher, extension, version, "extension.vsixmanifest"))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	manifest, err := parseVSIXManifest(reader)
	if err != nil {
		return nil, err
	}

	// The extension asset is not stored in the manifest.  Since we always store
	// it next to the manifest using the publisher.name-version format we can set
	// that as the path.
	manifest.Assets.Asset = append(manifest.Assets.Asset, VSIXAsset{
		Type:        VSIXAssetType,
		Path:        fmt.Sprintf("%s.vsix", extensionID(manifest)),
		Addressable: "true",
	})

	return manifest, nil
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
