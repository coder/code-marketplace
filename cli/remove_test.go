package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/code-marketplace/cli"
	"github.com/coder/code-marketplace/storage"
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
		// error is the expected error, if any.
		error string
		// expected contains the versions should have been deleted, if any.
		expected []storage.Version
		// extension is the extension to remove.  Every version of
		// testutil.Extensions[0] will be added before each test.
		extension testutil.Extension
		// name is the name of the test.
		name string
		// version is the version to remove.
		version string
	}{
		{
			name:      "RemoveOne",
			extension: testutil.Extensions[0],
			version:   "2.0.0",
			expected: []storage.Version{
				{Version: "2.0.0"},
			},
		},
		{
			name:      "RemovePlatforms",
			extension: testutil.Extensions[0],
			version:   testutil.Extensions[0].LatestVersion,
			expected: []storage.Version{
				{Version: "3.0.0"},
				{Version: "3.0.0", TargetPlatform: storage.PlatformAlpineX64},
				{Version: "3.0.0", TargetPlatform: storage.PlatformDarwinX64},
				{Version: "3.0.0", TargetPlatform: storage.PlatformLinuxArm64},
				{Version: "3.0.0", TargetPlatform: storage.PlatformLinuxX64},
				{Version: "3.0.0", TargetPlatform: storage.PlatformWin32X64},
			},
		},
		{
			name:      "All",
			extension: testutil.Extensions[0],
			all:       true,
			expected: []storage.Version{
				{Version: "1.0.0"},
				{Version: "1.0.0", TargetPlatform: storage.PlatformWin32X64},
				{Version: "1.5.2"},
				{Version: "2.0.0"},
				{Version: "2.2.2"},
				{Version: "3.0.0"},
				{Version: "3.0.0", TargetPlatform: storage.PlatformAlpineX64},
				{Version: "3.0.0", TargetPlatform: storage.PlatformDarwinX64},
				{Version: "3.0.0", TargetPlatform: storage.PlatformLinuxArm64},
				{Version: "3.0.0", TargetPlatform: storage.PlatformLinuxX64},
				{Version: "3.0.0", TargetPlatform: storage.PlatformWin32X64},
			},
		},
		{
			name:      "MissingTarget",
			error:     "target a specific version or pass --all",
			extension: testutil.Extensions[0],
		},
		{
			name:      "MissingTargetNoVersions",
			error:     "target a specific version or pass --all",
			extension: testutil.Extensions[1],
		},
		{
			name:      "AllWithVersion",
			error:     "cannot specify both",
			extension: testutil.Extensions[0],
			all:       true,
			version:   testutil.Extensions[0].LatestVersion,
		},
		{
			name:      "NoVersion",
			error:     "does not exist",
			extension: testutil.Extensions[0],
			version:   "does-not-exist",
		},
		{
			name:      "NoVersions",
			error:     "does not exist",
			extension: testutil.Extensions[1],
			version:   testutil.Extensions[1].LatestVersion,
		},
		{
			name:      "AllNoVersions",
			error:     "does not exist",
			extension: testutil.Extensions[1],
			all:       true,
		},
		{
			// Cannot target specific platforms at the moment.  If we wanted this
			// we would likely need to use a `--platform` flag since we already use @
			// to delineate the version.
			name:      "NoPlatformTarget",
			error:     "does not exist",
			extension: testutil.Extensions[0],
			version:   "1.0.0@win32-x64",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			extdir := t.TempDir()
			ext := testutil.Extensions[0]
			for _, version := range ext.Versions {
				manifestPath := filepath.Join(extdir, ext.Publisher, ext.Name, version.String(), "extension.vsixmanifest")
				err := os.MkdirAll(filepath.Dir(manifestPath), 0o755)
				require.NoError(t, err)
				err = os.WriteFile(manifestPath, testutil.ConvertExtensionToManifestBytes(t, ext, version), 0o644)
				require.NoError(t, err)
			}

			id := fmt.Sprintf("%s.%s", test.extension.Publisher, test.extension.Name)
			if test.version != "" {
				id = fmt.Sprintf("%s@%s", id, test.version)
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
				require.NotContains(t, output, "Failed to remove")
			}

			// Should list all the extensions that were able to be removed.
			if len(test.expected) > 0 {
				require.Contains(t, output, fmt.Sprintf("Removing %d version", len(test.expected)))
				for _, version := range test.expected {
					// Should not exist on disk.
					dest := filepath.Join(extdir, test.extension.Publisher, test.extension.Name, version.String())
					_, err := os.Stat(dest)
					require.Error(t, err)
					require.Contains(t, output, fmt.Sprintf("  - %s\n", version))
				}
			}
		})
	}
}
