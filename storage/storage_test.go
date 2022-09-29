package storage_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"

	"github.com/coder/code-marketplace/storage"
	"github.com/coder/code-marketplace/testutil"
)

func TestFileServer(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "foo")
	err := os.WriteFile(file, []byte("bar"), 0o644)
	require.NoError(t, err)

	server := (&storage.Local{ExtDir: dir}).FileServer()

	req := httptest.NewRequest("GET", "/foo", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, "bar", string(body))
}

func addExtension(t *testing.T, ext testutil.Extension, extdir, version string) *storage.VSIXManifest {
	dir := filepath.Join(extdir, ext.Publisher, ext.Name, version)
	err := os.MkdirAll(dir, 0o755)
	require.NoError(t, err)

	manifest := testutil.ConvertExtensionToManifest(ext, version)
	rawManifest, err := xml.Marshal(manifest)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "extension.vsixmanifest"), rawManifest, 0o644)
	require.NoError(t, err)

	// The storage interface should add the extension asset when it reads the
	// manifest since it is not on the actual manifest on disk.
	manifest.Assets.Asset = append(manifest.Assets.Asset, storage.VSIXAsset{
		Type:        storage.VSIXAssetType,
		Path:        fmt.Sprintf("%s.%s-%s.vsix", ext.Publisher, ext.Name, version),
		Addressable: "true",
	})

	return manifest
}

func TestManifest(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		extdir := t.TempDir()
		ext := testutil.Extensions[0]
		expected := addExtension(t, ext, extdir, "some-version")

		s := &storage.Local{ExtDir: extdir}
		manifest, err := s.Manifest(context.Background(), ext.Publisher, ext.Name, "some-version")
		require.NoError(t, err)
		require.Equal(t, expected, manifest)
	})

	t.Run("ParseError", func(t *testing.T) {
		t.Parallel()

		extdir := t.TempDir()
		dir := filepath.Join(extdir, "foo/bar/baz")
		err := os.MkdirAll(dir, 0o755)
		require.NoError(t, err)

		file := filepath.Join(dir, "extension.vsixmanifest")
		err = os.WriteFile(file, []byte("invalid"), 0o644)
		require.NoError(t, err)

		s := &storage.Local{ExtDir: extdir}

		_, err = s.Manifest(context.Background(), "foo", "bar", "baz")
		require.Error(t, err)
	})

	t.Run("Missing", func(t *testing.T) {
		t.Parallel()

		extdir := t.TempDir()
		s := &storage.Local{
			ExtDir: extdir,
		}
		_, err := s.Manifest(context.Background(), "foo", "bar", "baz")
		require.Error(t, err)
	})
}

type extension struct {
	manifest *storage.VSIXManifest
	versions []string
}

func TestWalkExtensions(t *testing.T) {
	t.Parallel()

	expected := []extension{}
	extdir := t.TempDir()
	for _, ext := range testutil.Extensions {
		var latestManifest *storage.VSIXManifest
		for _, version := range ext.Versions {
			manifest := addExtension(t, ext, extdir, version)
			if ext.LatestVersion == version {
				latestManifest = manifest
			}
		}

		// The versions should be sorted when walking.
		versions := make([]string, len(ext.Versions))
		copied := copy(versions, ext.Versions)
		require.Equal(t, len(ext.Versions), copied)
		sort.Sort(sort.Reverse(semver.ByVersion(versions)))

		expected = append(expected, extension{
			manifest: latestManifest,
			versions: versions,
		})
	}

	t.Run("NoExtensions", func(t *testing.T) {
		t.Parallel()

		s := &storage.Local{ExtDir: t.TempDir()}
		called := false
		err := s.WalkExtensions(context.Background(), func(manifest *storage.VSIXManifest, versions []string) error {
			called = true
			return nil
		})
		require.NoError(t, err)
		require.False(t, called)
	})

	t.Run("PropagateError", func(t *testing.T) {
		t.Parallel()

		s := &storage.Local{ExtDir: extdir}
		ran := 0
		expected := errors.New("error")
		err := s.WalkExtensions(context.Background(), func(manifest *storage.VSIXManifest, versions []string) error {
			ran++
			return expected
		})
		require.Equal(t, expected, err)
		require.Equal(t, 1, ran)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		got := []extension{}
		s := &storage.Local{ExtDir: extdir}
		err := s.WalkExtensions(context.Background(), func(manifest *storage.VSIXManifest, versions []string) error {
			got = append(got, extension{
				manifest: manifest,
				versions: versions,
			})
			return nil
		})
		require.NoError(t, err)
		require.ElementsMatch(t, expected, got)
	})
}

type file struct {
	name string
	body []byte
}

func createVsix(manifest *storage.VSIXManifest) ([]byte, error) {
	manifestBytes, err := xml.Marshal(manifest)
	if err != nil {
		return nil, err
	}

	files := []file{{"icon.png", []byte("fake icon")}}
	if manifest != nil {
		files = append(files, file{"extension.vsixmanifest", manifestBytes})
	}

	buf := bytes.NewBuffer(nil)
	zw := zip.NewWriter(buf)
	for _, file := range files {
		fw, err := zw.Create(file.name)
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write([]byte(file.body)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func requireExtension(t *testing.T, ext testutil.Extension, extdir string, got string) {
	expected := filepath.Join(extdir, ext.Publisher, ext.Name, ext.LatestVersion)
	require.Equal(t, expected, got)

	_, err := os.Stat(expected)
	require.NoError(t, err)

	vsixName := fmt.Sprintf("%s.%s-%s.vsix", ext.Publisher, ext.Name, ext.LatestVersion)
	_, err = os.Stat(filepath.Join(expected, vsixName))
	require.NoError(t, err)
}

func TestAddExtension(t *testing.T) {
	t.Parallel()

	t.Run("HTTPOK", func(t *testing.T) {
		t.Parallel()

		ext := testutil.Extensions[0]
		vsix, err := createVsix(testutil.ConvertExtensionToManifest(ext, ext.LatestVersion))
		require.NoError(t, err)

		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			_, err := rw.Write(vsix)
			require.NoError(t, err)
		}))
		defer server.Close()

		extdir := t.TempDir()
		s := &storage.Local{ExtDir: extdir}

		got, err := s.AddExtension(context.Background(), server.URL)
		require.NoError(t, err)
		requireExtension(t, ext, extdir, got)
	})

	t.Run("HTTPError", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			http.Error(rw, "something went wrong", http.StatusInternalServerError)
		}))
		defer server.Close()

		s := &storage.Local{ExtDir: t.TempDir()}

		_, err := s.AddExtension(context.Background(), server.URL)
		require.Error(t, err)

		// The error should mention the status code.
		require.Contains(t, err.Error(), strconv.Itoa(http.StatusInternalServerError))
	})

	t.Run("FileOK", func(t *testing.T) {
		t.Parallel()

		ext := testutil.Extensions[0]
		vsix, err := createVsix(testutil.ConvertExtensionToManifest(ext, ext.LatestVersion))
		require.NoError(t, err)

		extdir := t.TempDir()
		vsixPath := filepath.Join(extdir, "extension.vsix")
		err = os.WriteFile(vsixPath, vsix, 0o644)
		require.NoError(t, err)

		s := &storage.Local{ExtDir: extdir}

		got, err := s.AddExtension(context.Background(), vsixPath)
		require.NoError(t, err)
		requireExtension(t, ext, extdir, got)
	})

	t.Run("FileError", func(t *testing.T) {
		t.Parallel()

		extdir := t.TempDir()
		s := &storage.Local{ExtDir: extdir}
		_, err := s.AddExtension(context.Background(), filepath.Join(extdir, "foo.vsix"))
		require.Error(t, err)
	})

	t.Run("InvalidManifest", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			error    string
			manifest *storage.VSIXManifest
			name     string
		}{
			{
				error:    "not found",
				name:     "Missing",
				manifest: nil,
			},
			{
				error:    "publisher",
				name:     "MissingPublisher",
				manifest: &storage.VSIXManifest{},
			},
			{
				error: "ID",
				name:  "MissingID",
				manifest: &storage.VSIXManifest{
					Metadata: storage.VSIXMetadata{
						Identity: storage.VSIXIdentity{
							Publisher: "foo",
						},
					},
				},
			},
			{
				error: "version",
				name:  "MissingVersion",
				manifest: &storage.VSIXManifest{
					Metadata: storage.VSIXMetadata{
						Identity: storage.VSIXIdentity{
							Publisher: "foo",
							ID:        "bar",
						},
					},
				},
			},
		}

		for _, test := range tests {
			test := test
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				vsix, err := createVsix(test.manifest)
				require.NoError(t, err)

				extdir := t.TempDir()
				vsixPath := filepath.Join(extdir, "extension.vsix")
				err = os.WriteFile(vsixPath, vsix, 0o644)
				require.NoError(t, err)

				s := &storage.Local{ExtDir: extdir}

				_, err = s.AddExtension(context.Background(), vsixPath)
				require.Error(t, err)

				// The error should mention what is wrong.
				require.Contains(t, err.Error(), test.error)
			})
		}
	})
}
