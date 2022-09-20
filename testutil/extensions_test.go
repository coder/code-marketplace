package testutil_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/code-marketplace/testutil"
)

func TestConvert(t *testing.T) {
	ext := testutil.Extensions[0]
	manifest := testutil.ConvertExtensionToManifest(ext, "a")
	require.Equal(t, manifest.Metadata.Identity.ID, ext.Name)
	require.Equal(t, manifest.Metadata.Identity.Publisher, ext.Publisher)
	require.Equal(t, manifest.Metadata.Identity.Version, "a")
}
