package cli_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/code-marketplace/cli"
	"github.com/coder/code-marketplace/storage"
	"github.com/coder/code-marketplace/testutil"
)

func TestAddHelp(t *testing.T) {
	t.Parallel()

	cmd := cli.Root()
	cmd.SetArgs([]string{"add", "--help"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Add an extension", "has help")
}

func TestAdd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		// error is the expected error.
		error string
		// extensions are extensions to add.  Use for success cases.
		extensions []testutil.Extension
		// name is the name of the test.
		name string
		// vsixes contains raw bytes of extensions to add.  Use for failure cases.
		vsixes [][]byte
	}{
		{
			name:       "OK",
			extensions: []testutil.Extension{testutil.Extensions[0]},
		},
		{
			name:   "InvalidVSIX",
			error:  "not a valid zip",
			vsixes: [][]byte{[]byte{}},
		},
		{
			name: "BulkOK",
			extensions: []testutil.Extension{
				testutil.Extensions[0],
				testutil.Extensions[1],
				testutil.Extensions[2],
				testutil.Extensions[3],
			},
		},
		{
			name:  "BulkInvalid",
			error: "Failed to add 2 extensions: 0.vsix, 1.vsix",
			extensions: []testutil.Extension{
				testutil.Extensions[0],
				testutil.Extensions[1],
				testutil.Extensions[2],
				testutil.Extensions[3],
			},
			vsixes: [][]byte{
				[]byte{},
				[]byte("foo"),
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			extdir := t.TempDir()
			count := 0
			create := func(vsix []byte) {
				source := filepath.Join(extdir, fmt.Sprintf("%d.vsix", count))
				err := os.WriteFile(source, vsix, 0o644)
				require.NoError(t, err)
				count++
			}
			for _, vsix := range test.vsixes {
				create(vsix)
			}
			for _, ext := range test.extensions {
				create(testutil.CreateVSIXFromExtension(t, ext))
			}

			// With multiple extensions use bulk add by pointing to the directory
			// otherwise point to the vsix file.  When not using bulk add also test
			// from HTTP.
			sources := []string{extdir}
			if count == 1 {
				sources = []string{filepath.Join(extdir, "0.vsix")}

				handler := func(rw http.ResponseWriter, r *http.Request) {
					var vsix []byte
					if test.vsixes == nil {
						vsix = testutil.CreateVSIXFromExtension(t, test.extensions[0])
					} else {
						vsix = test.vsixes[0]
					}
					_, err := rw.Write(vsix)
					require.NoError(t, err)
				}
				server := httptest.NewServer(http.HandlerFunc(handler))
				defer server.Close()

				sources = append(sources, server.URL)
			}

			for _, source := range sources {
				cmd := cli.Root()
				args := []string{"add", source, "--extensions-dir", extdir}
				cmd.SetArgs(args)
				buf := new(bytes.Buffer)
				cmd.SetOutput(buf)

				err := cmd.Execute()
				output := buf.String()

				if test.error != "" {
					require.Error(t, err)
					require.Regexp(t, test.error, err.Error())
				} else {
					require.NoError(t, err)
				}
				// Should list all the extensions that worked.
				for _, ext := range test.extensions {
					// Should exist on disk.
					dest := filepath.Join(extdir, ext.Publisher, ext.Name, ext.LatestVersion)
					_, err := os.Stat(dest)
					require.NoError(t, err)
					// Should tell you where it went.
					id := storage.ExtensionID(ext.Publisher, ext.Name, ext.LatestVersion)
					require.Contains(t, output, fmt.Sprintf("Unpacked %s to %s", id, dest))
					// Should mention the dependencies and pack.
					require.Contains(t, output, fmt.Sprintf("%s has %d dep", id, len(ext.Dependencies)))
					if len(ext.Pack) > 0 {
						require.Contains(t, output, fmt.Sprintf("%s is in a pack with %d other", id, len(ext.Pack)))
					} else {
						require.Contains(t, output, fmt.Sprintf("%s is not in a pack", id))
					}
				}
			}
		})
	}
}
