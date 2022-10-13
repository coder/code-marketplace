package util_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/code-marketplace/util"
)

func TestPlural(t *testing.T) {
	t.Parallel()

	require.Equal(t, "0 versions", util.Plural(0, "version", ""))
	require.Equal(t, "1 version", util.Plural(1, "version", ""))
	require.Equal(t, "2 versions", util.Plural(2, "version", ""))

	require.Equal(t, "0 dependencies", util.Plural(0, "dependency", "dependencies"))
	require.Equal(t, "1 dependency", util.Plural(1, "dependency", "dependencies"))
	require.Equal(t, "2 dependencies", util.Plural(2, "dependency", "dependencies"))
}

func TestContains(t *testing.T) {
	t.Parallel()

	require.True(t, util.Contains([]string{"foo", "bar"}, "foo"))
	require.True(t, util.Contains([]string{"foo", "bar"}, "bar"))
	require.False(t, util.Contains([]string{"foo", "bar"}, "baz"))
	require.False(t, util.Contains([]string{"foo", "bar"}, "foobar"))
}
