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
	"runtime"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/code-marketplace/storage"
	"github.com/coder/code-marketplace/testutil"
)

func newStorage(t *testing.T, dir string) storage.Storage {
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	return storage.NewLocalStorage(context.Background(), dir, logger)
}

func TestFileServer(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "foo")
	err := os.WriteFile(file, []byte("bar"), 0o644)
	require.NoError(t, err)

	s := newStorage(t, dir)
	server := s.FileServer()

	req := httptest.NewRequest("GET", "/foo", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, "bar", string(body))
}

// addExtension adds the provided test extension to the provided directory.
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

		s := newStorage(t, extdir)
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

		s := newStorage(t, extdir)
		_, err = s.Manifest(context.Background(), "foo", "bar", "baz")
		require.Error(t, err)
	})

	t.Run("Missing", func(t *testing.T) {
		t.Parallel()

		s := newStorage(t, t.TempDir())
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

		s := newStorage(t, t.TempDir())
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

		s := newStorage(t, extdir)
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
		s := newStorage(t, extdir)
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

// createVSIX returns the bytes for a VSIX file containing the provided raw
// manifest bytes and an icon.
func createVSIX(t *testing.T, manifestBytes []byte) []byte {
	files := []file{{"icon.png", []byte("fake icon")}}
	if manifestBytes != nil {
		files = append(files, file{"extension.vsixmanifest", manifestBytes})
	}
	buf := bytes.NewBuffer(nil)
	zw := zip.NewWriter(buf)
	for _, file := range files {
		fw, err := zw.Create(file.name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(file.body))
		require.NoError(t, err)
	}
	err := zw.Close()
	require.NoError(t, err)
	return buf.Bytes()
}

// createVSIXFromManifest returns the bytes for a VSIX file containing the
// provided manifest and an icon.
func createVSIXFromManifest(t *testing.T, manifest *storage.VSIXManifest) []byte {
	manifestBytes, err := xml.Marshal(manifest)
	require.NoError(t, err)
	return createVSIX(t, manifestBytes)
}

// createVSIXFromExtension returns the bytes for a VSIX file containing the
// manifest for the provided test extension and an icon.
func createVSIXFromExtension(t *testing.T, ext testutil.Extension) []byte {
	return createVSIXFromManifest(t, testutil.ConvertExtensionToManifest(ext, ext.LatestVersion))
}

func TestReadVSIX(t *testing.T) {
	t.Parallel()

	t.Run("HTTP", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			// error is the expected error if any.
			error string
			// expected is compared with the return VSIX.  It is not checked if
			// `error` is expected.
			expected testutil.Extension
			// handler is the handler for the HTTP server returning the VSIX.  By
			// default it returns the `expected` extension.
			handler http.HandlerFunc
			// name is the name of the test.
			name string
		}{
			{
				name:     "OK",
				expected: testutil.Extensions[0],
			},
			{
				name:  "InternalError",
				error: strconv.Itoa(http.StatusInternalServerError),
				handler: func(rw http.ResponseWriter, r *http.Request) {
					http.Error(rw, "something went wrong", http.StatusInternalServerError)
				},
			},
			{
				name:  "ReadError",
				error: "unexpected EOF",
				handler: func(rw http.ResponseWriter, r *http.Request) {
					rw.Header().Set("Content-Length", "1")
				},
			},
		}

		for _, test := range tests {
			test := test
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				handler := test.handler
				if handler == nil {
					handler = func(rw http.ResponseWriter, r *http.Request) {
						vsix := createVSIXFromExtension(t, test.expected)
						_, err := rw.Write(vsix)
						require.NoError(t, err)
					}
				}

				server := httptest.NewServer(http.HandlerFunc(handler))
				defer server.Close()

				got, err := storage.ReadVSIX(context.Background(), server.URL)
				if test.error != "" {
					require.Error(t, err)
					require.Regexp(t, test.error, err.Error())
				} else {
					require.Equal(t, createVSIXFromExtension(t, test.expected), got)
				}
			})
		}
	})

	t.Run("File", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			// error is the expected error if any.  It is not checked if `errorType`
			// is expected.
			error string
			// errorType is the expected type of error.
			errorType error
			// expected is compared with the return VSIX.  It is not checked if
			// `error` or `errorType` are expected.
			expected testutil.Extension
			// name is the name of the test.
			name string
			// skip indicates whether to skip the test since some failure modes are
			// platform-dependent.
			skip bool
			// source sets up the extension on disk and returns the path to that
			// extension.
			source func(t *testing.T, extdir string) (string, error)
		}{
			{
				name:     "OK",
				expected: testutil.Extensions[0],
				source: func(t *testing.T, extdir string) (string, error) {
					vsix := createVSIXFromExtension(t, testutil.Extensions[0])
					vsixPath := filepath.Join(extdir, "extension.vsix")
					return vsixPath, os.WriteFile(vsixPath, vsix, 0o644)
				},
			},
			{
				name:      "NotFound",
				errorType: os.ErrNotExist,
				source: func(t *testing.T, extdir string) (string, error) {
					return filepath.Join(extdir, "foo.vsix"), nil
				},
			},
			{
				name:  "Unreadable",
				error: "permission denied",
				// It does not appear possible to create a file that is not readable on
				// Windows?
				skip: runtime.GOOS == "windows",
				source: func(t *testing.T, extdir string) (string, error) {
					vsixPath := filepath.Join(extdir, "extension.vsix")
					return vsixPath, os.WriteFile(vsixPath, []byte{}, 0o222)
				},
			},
		}

		for _, test := range tests {
			test := test
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()
				if test.skip {
					t.Skip()
				}

				extdir := t.TempDir()
				source, err := test.source(t, extdir)
				require.NoError(t, err)

				got, err := storage.ReadVSIX(context.Background(), source)
				if test.errorType != nil {
					require.Error(t, err)
					require.True(t, errors.Is(err, test.errorType))
				} else if test.error != "" {
					require.Error(t, err)
					require.Regexp(t, test.error, err.Error())
				} else {
					require.Equal(t, createVSIXFromExtension(t, test.expected), got)
				}
			})
		}
	})
}

func TestAddExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		// error is the expected error.
		error string
		// expected is the expected extension.  It is not checked if `error` is
		// expected.
		expected testutil.Extension
		// name is the name of the test.
		name string
		// setup is ran before the test.
		setup func(extdir string) (string, error)
		// skip indicates whether to skip the test since some failure modes are
		// platform-dependent.
		skip bool
		// vsix is the extension to add.  If missing it will be created from
		// `expected`.
		vsix []byte
	}{
		{
			name:     "OK",
			expected: testutil.Extensions[0],
		},
		{
			name:     "EmptyDependencies",
			expected: testutil.Extensions[1],
		},
		{
			name:     "NoDependencies",
			expected: testutil.Extensions[2],
		},
		{
			name:  "InvalidZip",
			error: "zip: not a valid zip file",
			vsix:  []byte{},
		},
		{
			error: "not found",
			name:  "MissingManifest",
			vsix:  createVSIX(t, nil),
		},
		{
			error: "EOF",
			name:  "EmptyManifest",
			vsix:  createVSIX(t, []byte("")),
		},
		{
			error: "EOF",
			name:  "TextFileManifest",
			vsix:  createVSIX(t, []byte("just some random text")),
		},
		{
			error: "XML syntax error",
			name:  "ManifestSyntaxError",
			vsix:  createVSIX(t, []byte("<PackageManifest/PackageManifest>")),
		},
		{
			error: "publisher",
			name:  "ManifestMissingPublisher",
			vsix:  createVSIXFromManifest(t, &storage.VSIXManifest{}),
		},
		{
			error: "ID",
			name:  "ManifestMissingID",
			vsix: createVSIXFromManifest(t, &storage.VSIXManifest{
				Metadata: storage.VSIXMetadata{
					Identity: storage.VSIXIdentity{
						Publisher: "foo",
					},
				},
			}),
		},
		{
			error: "version",
			name:  "ManifestMissingVersion",
			vsix: createVSIXFromManifest(t, &storage.VSIXManifest{
				Metadata: storage.VSIXMetadata{
					Identity: storage.VSIXIdentity{
						Publisher: "foo",
						ID:        "bar",
					},
				},
			}),
		},
		{
			name:  "ExtensionDirPerms",
			error: "permission denied",
			// It does not appear possible to create a directory that is not
			// writable on Windows?
			skip: runtime.GOOS == "windows",
			vsix: createVSIXFromExtension(t, testutil.Extensions[0]),
			setup: func(extdir string) (string, error) {
				// Disallow writing to the extension directory.
				extdir = filepath.Join(extdir, "no-write")
				return extdir, os.MkdirAll(extdir, 0o444)
			},
		},
		{
			name:  "CopyOverDirectory",
			error: "is a directory",
			vsix:  createVSIXFromExtension(t, testutil.Extensions[0]),
			setup: func(extdir string) (string, error) {
				// Put a directory in the way of the vsix.
				ext := testutil.Extensions[0]
				vsixName := fmt.Sprintf("%s.%s-%s.vsix", ext.Publisher, ext.Name, ext.LatestVersion)
				vsixPath := filepath.Join(extdir, ext.Publisher, ext.Name, ext.LatestVersion, vsixName)
				return extdir, os.MkdirAll(vsixPath, 0o755)
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var err error
			extdir := t.TempDir()
			if test.setup != nil {
				extdir, err = test.setup(extdir)
				require.NoError(t, err)
			}
			s := newStorage(t, extdir)
			vsix := test.vsix
			if vsix == nil {
				vsix = createVSIXFromExtension(t, test.expected)
			}
			got, err := s.AddExtension(context.Background(), vsix)
			if test.error != "" {
				require.Error(t, err)
				require.Regexp(t, test.error, err.Error())
			} else {
				expected := filepath.Join(extdir, test.expected.Publisher, test.expected.Name, test.expected.LatestVersion)
				require.Equal(t, expected, got.Location)
				_, err := os.Stat(expected)
				require.NoError(t, err)

				vsixName := fmt.Sprintf("%s.%s-%s.vsix", test.expected.Publisher, test.expected.Name, test.expected.LatestVersion)
				_, err = os.Stat(filepath.Join(expected, vsixName))
				require.NoError(t, err)

				require.Equal(t, test.expected.Dependencies, got.Dependencies)
				require.Equal(t, test.expected.Pack, got.Pack)
			}
		})
	}
}

func TestRemoveExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		all      bool
		error    string
		expected []string
		name     string
		remove   string
	}{
		{
			name:     "OK",
			expected: []string{"a"},
			remove:   fmt.Sprintf("%s.%s-a", testutil.Extensions[0].Publisher, testutil.Extensions[0].Name),
		},
		{
			name:   "NoVersionMatch",
			error:  "does not exist",
			remove: fmt.Sprintf("%s.%s-d", testutil.Extensions[0].Publisher, testutil.Extensions[0].Name),
		},
		{
			name:   "NoPublisherMatch",
			error:  "does not exist",
			remove: "test-test.test-test",
		},
		{
			name:   "NoExtensionMatch",
			error:  "does not exist",
			remove: "foo.test-test",
		},
		{
			name:   "MultipleDots",
			error:  "does not exist",
			remove: "foo.bar-test.test",
		},
		{
			name:   "EmptyID",
			error:  "invalid ID",
			remove: "",
		},
		{
			name:   "MissingPublisher",
			error:  "invalid ID",
			remove: ".qux-bar",
		},
		{
			name:   "MissingExtension",
			error:  "invalid ID",
			remove: "foo.-baz",
		},
		{
			name:   "MissingExtensionAndVersion",
			error:  "invalid ID",
			remove: "foo.",
		},
		{
			name:   "MissingPublisherAndVersion",
			error:  "invalid ID",
			remove: ".qux",
		},
		{
			name:   "InvalidID",
			error:  "invalid ID",
			remove: "publisher-version",
		},
		{
			name:   "MissingVersion",
			error:  "target a specific version or pass --all",
			remove: fmt.Sprintf("%s.%s", testutil.Extensions[0].Publisher, testutil.Extensions[0].Name),
		},
		{
			name:     "All",
			expected: []string{"a", "b", "c"},
			all:      true,
			remove:   fmt.Sprintf("%s.%s", testutil.Extensions[0].Publisher, testutil.Extensions[0].Name),
		},
		{
			name:   "AllWithVersion",
			error:  "cannot specify both",
			all:    true,
			remove: fmt.Sprintf("%s.%s-a", testutil.Extensions[0].Publisher, testutil.Extensions[0].Name),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			extdir := t.TempDir()
			ext := testutil.Extensions[0]
			addExtension(t, ext, extdir, "a")
			addExtension(t, ext, extdir, "b")
			addExtension(t, ext, extdir, "c")

			ext = testutil.Extensions[1]
			addExtension(t, ext, extdir, "a")
			addExtension(t, ext, extdir, "b")
			addExtension(t, ext, extdir, "c")

			s := newStorage(t, extdir)
			removed, err := s.RemoveExtension(context.Background(), test.remove, test.all)
			if test.error != "" {
				require.Error(t, err)
				require.Regexp(t, test.error, err.Error())
			} else {
				require.NoError(t, err)
			}
			require.ElementsMatch(t, test.expected, removed)
			dir := filepath.Join(extdir, testutil.Extensions[0].Publisher, testutil.Extensions[0].Name)
			for _, version := range test.expected {
				_, err := os.Stat(filepath.Join(dir, version))
				require.Error(t, err)
			}
		})
	}
}
