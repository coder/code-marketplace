package storage_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

	"github.com/coder/code-marketplace/storage"
	"github.com/coder/code-marketplace/testutil"
)

type testStorage struct {
	storage storage.Storage
	write   func(content []byte, elem ...string)
	exists  func(elem ...string) bool
}
type storageFactory = func(t *testing.T) testStorage

func TestNewStorage(t *testing.T) {
	tests := []struct {
		// error is the expected error, if any
		error string
		// local indicates whether the storage is local.
		local bool
		// name is the name of the test
		name string
		// options are the options to use to create the storage.
		options *storage.Options
		// token is the Artifactory token.
		token string
	}{
		{
			name: "Local",
			options: &storage.Options{
				ExtDir: "/extensions",
			},
			local: true,
		},
		{
			name:  "ArtifactoryWithToken",
			token: "foo",
			options: &storage.Options{
				Artifactory: "coder.com",
				Repo:        "extensions",
			},
		},
		{
			name:  "ArtifactoryWithoutKey",
			error: "environment variable must be set",
			options: &storage.Options{
				Artifactory: "coder.com",
				Repo:        "extensions",
			},
		},
		{
			name:  "ArtifactoryWithoutRepo",
			error: "must provide repository",
			token: "foo",
			options: &storage.Options{
				Artifactory: "coder.com",
			},
		},
		{
			name:  "DirAndArtifactory",
			error: "cannot use both",
			options: &storage.Options{
				ExtDir: "/extensions",
				Repo:   "extensions",
			},
		},
		{
			name:  "DirAndRepo",
			error: "cannot use both",
			options: &storage.Options{
				ExtDir: "/extensions",
				Repo:   "extensions",
			},
		},
		{
			name:    "None",
			error:   "must provide an Artifactory repository or local directory",
			options: &storage.Options{},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(storage.ArtifactoryTokenEnvKey, test.token)
			s, err := storage.NewStorage(context.Background(), test.options)
			if test.error != "" {
				require.Error(t, err)
				require.Regexp(t, test.error, err.Error())
			} else if test.local {
				_, ok := s.(*storage.Local)
				require.True(t, ok)
				require.NoError(t, err)
			} else {
				_, ok := s.(*storage.Artifactory)
				require.True(t, ok)
				require.NoError(t, err)
			}
		})
	}
}

func TestStorage(t *testing.T) {
	t.Parallel()
	factories := []struct {
		name    string
		factory storageFactory
	}{
		{
			name:    "Local",
			factory: localFactory,
		},
		{
			name:    "Artifactory",
			factory: artifactoryFactory,
		},
	}
	for _, sf := range factories {
		t.Run(sf.name, func(t *testing.T) {
			t.Run("AddExtension", func(t *testing.T) {
				testAddExtension(t, sf.factory)
			})
			t.Run("RemoveExtension", func(t *testing.T) {
				testRemoveExtension(t, sf.factory)
			})
			t.Run("FileServer", func(t *testing.T) {
				testFileServer(t, sf.factory)
			})
			t.Run("Manifest", func(t *testing.T) {
				testManifest(t, sf.factory)
			})
			t.Run("WalkExtensions", func(t *testing.T) {
				testWalkExtensions(t, sf.factory)
			})
			t.Run("Versions", func(t *testing.T) {
				testVersions(t, sf.factory)
			})
		})
	}
}

func testFileServer(t *testing.T, factory storageFactory) {
	t.Parallel()

	tests := []struct {
		// content is the expected content when there is no error.
		content string
		// error is the expected error code, if any.
		error int
		// name is the name of the test.
		name string
		// path is the path to request.
		path string
	}{
		{
			name:    "OK",
			content: "baz",
			path:    "/foo/bar",
		},
		{
			name:  "NotFound",
			error: http.StatusNotFound,
			path:  "/qux",
		},
	}

	f := factory(t)
	f.write([]byte("baz"), "foo", "bar")

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", test.path, nil)
			rec := httptest.NewRecorder()

			server := f.storage.FileServer()
			server.ServeHTTP(rec, req)

			resp := rec.Result()
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			if test.error != 0 {
				require.Equal(t, test.error, resp.StatusCode)
			} else {
				require.Equal(t, test.content, string(body))
			}
		})
	}
}

func testManifest(t *testing.T, factory storageFactory) {
	t.Parallel()

	tests := []struct {
		// error is the expected error, if any.
		error error
		// extension contains the expected manifest.
		extension testutil.Extension
		// name is the name of the test.
		name string
		// version is the version to expect in the manifest.  Defaults to the
		// extension's latest version.
		version string
	}{
		{
			name:      "OK",
			extension: testutil.Extensions[0],
		},
		{
			name:      "MissingVersion",
			error:     fs.ErrNotExist,
			extension: testutil.Extensions[0],
			version:   "some-nonexistent-version",
		},
		{
			name:      "MissingExtension",
			error:     fs.ErrNotExist,
			extension: testutil.Extensions[1],
		},
		{
			name:      "MissingPublisher",
			error:     fs.ErrNotExist,
			extension: testutil.Extensions[2],
		},
		{
			name:      "ParseError",
			error:     io.EOF,
			extension: testutil.Extensions[3],
		},
	}

	f := factory(t)
	ext := testutil.Extensions[0]
	manifestBytes := testutil.ConvertExtensionToManifestBytes(t, ext, ext.LatestVersion)
	f.write(manifestBytes, ext.Publisher, ext.Name, ext.LatestVersion, "extension.vsixmanifest")

	ext = testutil.Extensions[3]
	f.write([]byte("invalid"), ext.Publisher, ext.Name, ext.LatestVersion, "extension.vsixmanifest")

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			version := test.version
			if version == "" {
				version = test.extension.LatestVersion
			}
			manifest, err := f.storage.Manifest(context.Background(), test.extension.Publisher, test.extension.Name, version)
			if test.error != nil {
				require.Error(t, err)
				require.True(t, errors.Is(err, test.error))
			} else {
				expected := testutil.ConvertExtensionToManifest(testutil.Extensions[0], version)
				// The storage interface should add the extension asset when it reads the
				// manifest since it is not on the actual manifest on disk.
				expected.Assets.Asset = append(expected.Assets.Asset, storage.VSIXAsset{
					Type:        storage.VSIXAssetType,
					Path:        fmt.Sprintf("%s.%s-%s.vsix", test.extension.Publisher, test.extension.Name, version),
					Addressable: "true",
				})
				require.NoError(t, err)
				require.Equal(t, expected, manifest)
			}
		})
	}
}

type extension struct {
	manifest *storage.VSIXManifest
	versions []string
}

func testWalkExtensions(t *testing.T, factory storageFactory) {
	t.Parallel()

	tests := []struct {
		// error is the expected error, if any.
		error string
		// extensions contains the expected extensions
		extensions []testutil.Extension
		// name is then ame of the test
		name string
		// run is an optional walk callback.
		run func() error
	}{
		{
			name:       "OK",
			extensions: testutil.Extensions,
		},
		{
			name: "NoExtensions",
		},
		{
			name:       "PropagateError",
			error:      "propagate",
			extensions: []testutil.Extension{testutil.Extensions[0]},
			run: func() error {
				return errors.New("propagate")
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := factory(t)
			expected := []extension{}
			for _, ext := range test.extensions {
				versions := make([]string, len(ext.Versions))
				copy(versions, ext.Versions)
				sort.Sort(sort.Reverse(semver.ByVersion(versions)))
				manifest := testutil.ConvertExtensionToManifest(ext, ext.LatestVersion)
				// The storage interface should add the extension asset when it reads the
				// manifest since it is not on the actual manifest on disk.
				manifest.Assets.Asset = append(manifest.Assets.Asset, storage.VSIXAsset{
					Type:        storage.VSIXAssetType,
					Path:        fmt.Sprintf("%s.%s-%s.vsix", ext.Publisher, ext.Name, ext.LatestVersion),
					Addressable: "true",
				})
				expected = append(expected, extension{
					manifest: manifest,
					versions: versions,
				})
				for _, version := range ext.Versions {
					f.write(testutil.ConvertExtensionToManifestBytes(t, ext, version), ext.Publisher, ext.Name, version, "extension.vsixmanifest")
				}
			}
			got := []extension{}
			err := f.storage.WalkExtensions(context.Background(), func(manifest *storage.VSIXManifest, versions []string) error {
				got = append(got, extension{
					manifest: manifest,
					versions: versions,
				})
				if test.run != nil {
					return test.run()
				}
				return nil
			})
			if test.error != "" {
				require.Error(t, err)
				require.Regexp(t, test.error, err.Error())
			} else {
				require.NoError(t, err)
			}
			require.ElementsMatch(t, expected, got)
		})
	}
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
			{
				name:     "Redirect",
				expected: testutil.Extensions[0],
				handler: func(rw http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/redirected" {
						vsix := testutil.CreateVSIXFromExtension(t, testutil.Extensions[0])
						_, err := rw.Write(vsix)
						require.NoError(t, err)
					} else {
						http.Redirect(rw, r, "/redirected", http.StatusFound)
					}
				},
			},
			{
				name:  "InfiniteRedirects",
				error: "stopped after 10 redirects",
				handler: func(rw http.ResponseWriter, r *http.Request) {
					http.Redirect(rw, r, ".", http.StatusFound)
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
			vsix:  testutil.CreateVSIX(t, nil, nil),
		},
		{
			name:  "EmptyManifest",
			error: "EOF",
			vsix:  testutil.CreateVSIX(t, []byte(""), nil),
		},
		{
			name:  "TextFileManifest",
			error: "EOF",
			vsix:  testutil.CreateVSIX(t, []byte("just some random text"), nil),
		},
		{
			name:  "ManifestSyntaxError",
			error: "XML syntax error",
			vsix:  testutil.CreateVSIX(t, []byte("<PackageManifest/PackageManifest>"), nil),
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

func TestReadVSIXPackageJson(t *testing.T) {
	t.Parallel()

	tests := []struct {
		// error is the expected error, if any.
		error string
		// json is the package.json from which to create the VSIX.  Use `vsix` to
		// specify raw bytes instead.
		json *storage.VSIXPackageJSON
		// name is the name of the test.
		name string
		// vsix contains the raw bytes for the VSIX from which to read the manifest.
		// If omitted it will be created from `manifest`.  For non-error cases
		// always use `manifest` instead so the result can be checked.
		vsix []byte
	}{
		{
			name: "OK",
			json: &storage.VSIXPackageJSON{},
		},
		{
			name: "WithBrowser",
			json: &storage.VSIXPackageJSON{
				Browser: "foo",
			},
		},
		{
			name:  "MissingPackageJson",
			error: "not found",
			vsix:  testutil.CreateVSIX(t, nil, nil),
		},
		{
			name:  "EmptyPackageJson",
			error: "EOF",
			vsix:  testutil.CreateVSIX(t, nil, []byte("")),
		},
		{
			name:  "TextFilePackageJson",
			error: "invalid character",
			vsix:  testutil.CreateVSIX(t, nil, []byte("just some random text")),
		},
		{
			name:  "PackageJsonSyntaxError",
			error: "invalid character",
			vsix:  testutil.CreateVSIX(t, nil, []byte("{\"foo\": bar}")),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			vsix := test.vsix
			if vsix == nil {
				vsix = testutil.CreateVSIXFromPackageJSON(t, test.json)
			}
			json, err := storage.ReadVSIXPackageJSON(vsix, "extension/package.json")
			if test.error != "" {
				require.Error(t, err)
				require.Regexp(t, test.error, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, test.json, json)
			}
		})
	}
}

func testAddExtension(t *testing.T, factory storageFactory) {
	t.Parallel()

	tests := []struct {
		// error is the expected error.
		error string
		// extension is the extension to add.  Use `vsix` to specify raw bytes
		// instead.
		extension testutil.Extension
		// name is the name of the test.
		name string
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
			name:      "CopyOverDirectory",
			extension: testutil.Extensions[3],
			error:     "is a directory|found a folder",
		},
	}

	// Put a directory in the way of the vsix.
	f := factory(t)
	ext := testutil.Extensions[3]
	vsixName := fmt.Sprintf("%s.%s-%s.vsix", ext.Publisher, ext.Name, ext.LatestVersion)
	f.write([]byte("foo"), ext.Publisher, ext.Name, ext.LatestVersion, vsixName, "foo")

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			expected := &storage.VSIXManifest{}
			vsix := test.vsix
			if vsix == nil {
				expected = testutil.ConvertExtensionToManifest(test.extension, test.extension.LatestVersion)
				vsix = testutil.CreateVSIXFromManifest(t, expected)
			}
			location, err := f.storage.AddExtension(context.Background(), expected, vsix)
			if test.error != "" {
				require.Error(t, err)
				require.Regexp(t, test.error, err.Error())
			} else {
				require.NoError(t, err)
				// Should make mention of the extension path.
				require.Contains(t, location, test.extension.Publisher)
				require.Contains(t, location, test.extension.Name)
				require.Contains(t, location, test.extension.LatestVersion)
				// There should be a manifest now.
				require.True(t, f.exists(test.extension.Publisher, test.extension.Name, test.extension.LatestVersion, "extension.vsixmanifest"))
			}
		})
	}
}

func testRemoveExtension(t *testing.T, factory storageFactory) {
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
			version:   testutil.Extensions[0].LatestVersion,
		},
		{
			name:      "NoVersionMatch",
			error:     os.ErrNotExist,
			extension: testutil.Extensions[0],
			version:   "does-not-exist",
		},
		{
			name:  "NoPublisherMatch",
			error: os.ErrNotExist,
			// [3]'s publisher does not exist.
			extension: testutil.Extensions[3],
			version:   testutil.Extensions[3].LatestVersion,
		},
		{
			name:  "NoNameMatch",
			error: os.ErrNotExist,
			// [1] shares a publisher with [0] but the extension does not exist.
			extension: testutil.Extensions[1],
			version:   testutil.Extensions[1].LatestVersion,
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

			f := factory(t)
			for _, ext := range []testutil.Extension{testutil.Extensions[0], testutil.Extensions[2]} {
				for _, version := range ext.Versions {
					f.write(testutil.ConvertExtensionToManifestBytes(t, ext, version), ext.Publisher, ext.Name, version, "extension.vsixmanifest")
				}
			}

			err := f.storage.RemoveExtension(context.Background(), test.extension.Publisher, test.extension.Name, test.version)
			if test.error != nil {
				require.Error(t, err)
				require.True(t, errors.Is(err, test.error))
			} else {
				require.NoError(t, err)
				// If a version was specified the parent extension directory should
				// still exist otherwise the whole thing should have been removed.
				if test.version != "" {
					require.True(t, f.exists(test.extension.Publisher, test.extension.Name))
					require.False(t, f.exists(test.extension.Publisher, test.extension.Name, test.version))
				} else {
					require.False(t, f.exists(test.extension.Publisher, test.extension.Name))
				}
			}
		})
	}
}

func testVersions(t *testing.T, factory storageFactory) {
	t.Parallel()

	tests := []struct {
		// error is the expected error type.
		error error
		// extension is the extension with the expected versions.
		extension testutil.Extension
		// name is the name of the test.
		name string
	}{
		{
			name:      "OK",
			extension: testutil.Extensions[0],
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

	f := factory(t)
	ext := testutil.Extensions[0]
	for _, version := range ext.Versions {
		f.write([]byte("stub"), ext.Publisher, ext.Name, version, "extension.vsixmanifest")
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := f.storage.Versions(context.Background(), test.extension.Publisher, test.extension.Name)
			if test.error != nil {
				require.Error(t, err)
				require.True(t, errors.Is(err, test.error))
			} else {
				require.NoError(t, err)
				versions := make([]string, len(test.extension.Versions))
				copy(versions, test.extension.Versions)
				sort.Sort(sort.Reverse(semver.ByVersion(versions)))
				require.Equal(t, versions, got)
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
			require.Equal(t, test.expected, storage.ExtensionIDFromManifest(test.manifest))
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
