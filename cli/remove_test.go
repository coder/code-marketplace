package cli_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/code-marketplace/cli"
	"github.com/coder/code-marketplace/testutil"
)

func TestRemoveHelp(t *testing.T) {
	t.Parallel()

	cmd := cli.Root()
	cmd.SetArgs([]string{"remove", "--help"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Remove an extension", "has help")
}

func TestRemove(t *testing.T) {
	t.Parallel()

	tests := []struct {
		// all means to pass --all.
		all bool
		// error is the expected error.
		error string
		// extension is the extension to remove.  testutil.Extensions[0] will be
		// added with versions a, b, and c before each test.
		extension testutil.Extension
		// name is the name of the test.
		name string
		// version is the version to remove.
		version string
	}{
		{
			name:      "RemoveOne",
			extension: testutil.Extensions[0],
			version:   "a",
		},
		{
			name:      "All",
			extension: testutil.Extensions[0],
			all:       true,
		},
		{
			name:      "MissingTarget",
			error:     "target a specific version or pass --all",
			extension: testutil.Extensions[0],
		},
		{
			name:      "MissingTargetNoVersions",
			error:     "has no versions",
			extension: testutil.Extensions[1],
		},
		{
			name:      "AllWithVersion",
			error:     "cannot specify both",
			extension: testutil.Extensions[0],
			all:       true,
			version:   "a",
		},
		{
			name:      "NoVersion",
			error:     "does not exist",
			extension: testutil.Extensions[0],
			version:   "d",
		},
		{
			name:      "NoVersions",
			error:     "does not exist",
			extension: testutil.Extensions[1],
			version:   "a",
		},
		{
			name:      "AllNoVersions",
			error:     "has no versions",
			extension: testutil.Extensions[1],
			all:       true,
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

			id := fmt.Sprintf("%s.%s", test.extension.Publisher, test.extension.Name)
			if test.version != "" {
				id = fmt.Sprintf("%s-%s", id, test.version)
			}

			cmd := cli.Root()
			args := []string{"remove", id, "--extensions-dir", extdir}
			if test.all {
				args = append(args, "--all")
			}
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
				if test.all {
					require.Contains(t, output, "Removed 3 versions")
					require.Contains(t, output, "  - a")
					require.Contains(t, output, "  - b")
					require.Contains(t, output, "  - c")
				} else {
					require.Contains(t, output, fmt.Sprintf("Removed %s", test.version))
				}
			}
		})
	}
}
