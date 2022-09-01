package buildinfo_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/code-marketplace/buildinfo"
)

func TestBuildInfo(t *testing.T) {
	t.Parallel()

	version := buildinfo.Version()
	require.Equal(t, version, "unknown")
}
