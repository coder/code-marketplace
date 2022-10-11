package storage_test

import (
	"context"
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
	return storage.NewLocalStorage(dir, logger)
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

func TestManifest(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		extdir := t.TempDir()
		ext := testutil.Extensions[0]
		expected := testutil.AddExtension(t, ext, extdir, "some-version")

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
			manifest := testutil.AddExtension(t, ext, extdir, version)
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

func TestReadVSIX(t *testing.T) {
	t.Parallel()

	t.Run("HTTP", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			// error is the expected error, if any.
			error string
			// expected is compared with the return VSIX.  It is not checked if an
			// error is expected.
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
						vsix := testutil.CreateVSIXFromExtension(t, test.expected)
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
					require.Equal(t, testutil.CreateVSIXFromExtension(t, test.expected), got)
				}
			})
		}
	})

	t.Run("File", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			// error is the expected error type, if any.
			error error
			// expected is compared with the return VSIX.  It is not checked if an
			// error is expected.
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
					vsix := testutil.CreateVSIXFromExtension(t, testutil.Extensions[0])
					vsixPath := filepath.Join(extdir, "extension.vsix")
					return vsixPath, os.WriteFile(vsixPath, vsix, 0o644)
				},
			},
			{
				name:  "NotFound",
				error: os.ErrNotExist,
				source: func(t *testing.T, extdir string) (string, error) {
					return filepath.Join(extdir, "foo.vsix"), nil
				},
			},
			{
				name:  "Unreadable",
				error: os.ErrPermission,
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
				if test.error != nil {
					require.Error(t, err)
					require.True(t, errors.Is(err, test.error))
				} else {
					require.Equal(t, testutil.CreateVSIXFromExtension(t, test.expected), got)
				}
			})
		}
	})
}

func TestReadVSIXManifest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		// error is the expected error, if any.
		error string
		// manifest is the manifest from which to create the VSIX.  Use `vsix` to
		// specify raw bytes instead.
		manifest *storage.VSIXManifest
		// name is the name of the test.
		name string
		// vsix contains the raw bytes for the VSIX from which to read the manifest.
		// If omitted it will be created from `manifest`.  For non-error cases
		// always use `manifest` instead so the result can be checked.
		vsix []byte
	}{
		{
			name: "OK",
			manifest: &storage.VSIXManifest{
				Metadata: storage.VSIXMetadata{
					Identity: storage.VSIXIdentity{
						Publisher: "foo",
						ID:        "bar",
						Version:   "baz",
					},
				},
			},
		},
		{
			name:  "MissingManifest",
			error: "not found",
			vsix:  testutil.CreateVSIX(t, nil),
		},
		{
			name:  "EmptyManifest",
			error: "EOF",
			vsix:  testutil.CreateVSIX(t, []byte("")),
		},
		{
			name:  "TextFileManifest",
			error: "EOF",
			vsix:  testutil.CreateVSIX(t, []byte("just some random text")),
		},
		{
			name:  "ManifestSyntaxError",
			error: "XML syntax error",
			vsix:  testutil.CreateVSIX(t, []byte("<PackageManifest/PackageManifest>")),
		},
		{
			name:  "ManifestMissingPublisher",
			error: "publisher",
			vsix:  testutil.CreateVSIXFromManifest(t, &storage.VSIXManifest{}),
		},
		{
			name:  "ManifestMissingID",
			error: "ID",
			manifest: &storage.VSIXManifest{
				Metadata: storage.VSIXMetadata{
					Identity: storage.VSIXIdentity{
						Publisher: "foo",
					},
				},
			},
		},
		{
			name:  "ManifestMissingVersion",
			error: "version",
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
			vsix := test.vsix
			if vsix == nil {
				vsix = testutil.CreateVSIXFromManifest(t, test.manifest)
			}
			manifest, err := storage.ReadVSIXManifest(vsix)
			if test.error != "" {
				require.Error(t, err)
				require.Regexp(t, test.error, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, test.manifest, manifest)
			}
		})
	}
}

func TestAddExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		// error is the expected error.
		error string
		// extension is the extension to add.  Use `vsix` to specify raw bytes
		// instead.
		extension testutil.Extension
		// name is the name of the test.
		name string
		// setup is ran before the test and returns the extension directory.
		setup func(extdir string) (string, error)
		// skip indicates whether to skip the test since some failure modes are
		// platform-dependent.
		skip bool
		// vsix contains the raw bytes of the extension to add.  If omitted it will
		// be created from `extension`.  For non-error cases always use `extension`
		// instead so we can check the result.
		vsix []byte
	}{
		{
			name:      "OK",
			extension: testutil.Extensions[0],
		},
		{
			name:      "EmptyDependencies",
			extension: testutil.Extensions[1],
		},
		{
			name:      "NoDependencies",
			extension: testutil.Extensions[2],
		},
		{
			name:  "InvalidZip",
			vsix:  []byte{},
			error: "zip: not a valid zip file",
		},
		{
			name:      "ExtensionDirPerms",
			extension: testutil.Extensions[0],
			error:     "permission denied",
			// It does not appear possible to create a directory that is not writable
			// on Windows?
			skip: runtime.GOOS == "windows",
			setup: func(extdir string) (string, error) {
				// Disallow writing to the extension directory.
				extdir = filepath.Join(extdir, "no-write")
				return extdir, os.MkdirAll(extdir, 0o444)
			},
		},
		{
			name:      "CopyOverDirectory",
			extension: testutil.Extensions[0],
			error:     "is a directory",
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
			if test.skip {
				t.Skip()
			}
			var err error
			extdir := t.TempDir()
			if test.setup != nil {
				extdir, err = test.setup(extdir)
				require.NoError(t, err)
			}
			s := newStorage(t, extdir)
			manifest := &storage.VSIXManifest{}
			vsix := test.vsix
			if vsix == nil {
				manifest = testutil.ConvertExtensionToManifest(test.extension, test.extension.LatestVersion)
				vsix = testutil.CreateVSIXFromManifest(t, manifest)
			}
			location, err := s.AddExtension(context.Background(), manifest, vsix)
			if test.error != "" {
				require.Error(t, err)
				require.Regexp(t, test.error, err.Error())
			} else {
				expected := filepath.Join(extdir, test.extension.Publisher, test.extension.Name, test.extension.LatestVersion)
				require.Equal(t, expected, location)

				_, err := os.Stat(expected)
				require.NoError(t, err)

				vsixName := fmt.Sprintf("%s.%s-%s.vsix", test.extension.Publisher, test.extension.Name, test.extension.LatestVersion)
				_, err = os.Stat(filepath.Join(expected, vsixName))
				require.NoError(t, err)
			}
		})
	}
}

func TestRemoveExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		// error is the expected error type.
		error error
		// extension is the extension to remove.  [0] and [2] of testutil.Extensions
		// will be added with versions a, b, and c before each test.
		extension testutil.Extension
		// name is the name of the test.
		name string
		// version is the version to remove.
		version string
	}{
		{
			name:      "OK",
			extension: testutil.Extensions[0],
			version:   "a",
		},
		{
			name:      "NoVersionMatch",
			error:     os.ErrNotExist,
			extension: testutil.Extensions[0],
			version:   "d",
		},
		{
			name:  "NoPublisherMatch",
			error: os.ErrNotExist,
			// [3]'s publisher does not exist.
			extension: testutil.Extensions[3],
			version:   "a",
		},
		{
			name:  "NoNameMatch",
			error: os.ErrNotExist,
			// [1] shares a publisher with [0] but the extension does not exist.
			extension: testutil.Extensions[1],
			version:   "a",
		},
		{
			name:      "All",
			extension: testutil.Extensions[0],
			version:   "",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			extdir := t.TempDir()
			ext := testutil.Extensions[0]
			testutil.AddExtension(t, ext, extdir, "a")
			testutil.AddExtension(t, ext, extdir, "b")
			testutil.AddExtension(t, ext, extdir, "c")

			ext = testutil.Extensions[2]
			testutil.AddExtension(t, ext, extdir, "a")
			testutil.AddExtension(t, ext, extdir, "b")
			testutil.AddExtension(t, ext, extdir, "c")

			s := newStorage(t, extdir)
			err := s.RemoveExtension(context.Background(), test.extension.Publisher, test.extension.Name, test.version)
			if test.error != nil {
				require.Error(t, err)
				require.True(t, errors.Is(err, test.error))
			} else {
				require.NoError(t, err)
				dir := filepath.Join(extdir, test.extension.Publisher, test.extension.Name)
				// If a version was specified the parent extension directory should
				// still exist otherwise the whole thing should have been removed.
				if test.version != "" {
					_, err := os.Stat(dir)
					require.NoError(t, err)
					dir = filepath.Join(dir, test.version)
				}
				_, err := os.Stat(dir)
				require.Error(t, err)
			}
		})
	}
}

func TestVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		// error is the expected error type.
		error error
		// expected contains the expected versions.  It is not checked if an error
		// is expected.
		expected []string
		// extension is the extension to get versions.  testutil.Extensions[0]
		// will be added with versions a, b, and c before each test.
		extension testutil.Extension
		// name is the name of the test.
		name string
	}{
		{
			name:      "OK",
			extension: testutil.Extensions[0],
			expected:  []string{"c", "b", "a"},
		},
		{
			name: "NoExtension",
			// [1] shares a publisher with [0] but the extension does not exist.
			extension: testutil.Extensions[1],
			error:     os.ErrNotExist,
		},
		{
			name: "NoPublisher",
			// [3]'s publisher does not exist.
			extension: testutil.Extensions[3],
			error:     os.ErrNotExist,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var err error
			extdir := t.TempDir()
			ext := testutil.Extensions[0]
			testutil.AddExtension(t, ext, extdir, "a")
			testutil.AddExtension(t, ext, extdir, "b")
			testutil.AddExtension(t, ext, extdir, "c")

			s := newStorage(t, extdir)
			got, err := s.Versions(context.Background(), test.extension.Publisher, test.extension.Name)
			if test.error != nil {
				require.Error(t, err)
				require.True(t, errors.Is(err, test.error))
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, got)
			}
		})
	}
}

func TestExtensionID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		// expected is the expected id.
		expected string
		// manifest is the manifest from which to build the ID.
		manifest *storage.VSIXManifest
		// name is the name of the test.
		name string
	}{
		{
			name:     "OK",
			expected: "foo.bar-test",
			manifest: &storage.VSIXManifest{
				Metadata: storage.VSIXMetadata{
					Identity: storage.VSIXIdentity{
						Publisher: "foo",
						ID:        "bar",
						Version:   "test",
					},
				},
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.expected, storage.ExtensionID(test.manifest))
		})
	}
}

func TestParseExtensionID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		// error is whether an error is expected.
		error bool
		// expected is the expected parse result (publisher, name, version).  It is
		// ignored if an error is expected.
		expected []string
		// id is the id to parse.
		id string
		// name is the name of the test.
		name string
	}{
		{
			name:     "OK",
			expected: []string{"foo", "bar", "test"},
			id:       "foo.bar-test",
		},
		{
			name:     "VersionWithDots",
			expected: []string{"foo", "bar", "test.test"},
			id:       "foo.bar-test.test",
		},
		{
			name:  "EmptyID",
			error: true,
			id:    "",
		},
		{
			name:  "MissingPublisher",
			error: true,
			id:    ".qux-bar",
		},
		{
			name:  "MissingExtension",
			error: true,
			id:    "foo.-baz",
		},
		{
			name:  "MissingExtensionAndVersion",
			error: true,
			id:    "foo.",
		},
		{
			name:  "MissingPublisherAndVersion",
			error: true,
			id:    ".qux",
		},
		{
			name:  "InvalidID",
			error: true,
			id:    "publisher-version",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			publisher, name, version, err := storage.ParseExtensionID(test.id)
			if test.error {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, []string{publisher, name, version})
			}
		})
	}
}
