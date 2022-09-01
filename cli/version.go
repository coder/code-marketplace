package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/coder/code-marketplace/buildinfo"
)

func version() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show marketplace version",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), buildinfo.Version())
			return nil
		},
	}
}
