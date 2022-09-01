package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/code-marketplace/cli"
)

func TestRoot(t *testing.T) {
	t.Parallel()

	cmd := cli.Root()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Code extension marketplace", "has help")
}
