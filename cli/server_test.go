package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/code-marketplace/cli"
)

func TestServer(t *testing.T) {
	t.Parallel()

	cmd := cli.Root()
	cmd.SetArgs([]string{"server", "--help"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Start the Code", "has help")
}
