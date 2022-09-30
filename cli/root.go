package cli

import (
	"github.com/spf13/cobra"
	"strings"
)

func Root() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "marketplace",
		SilenceErrors: true,
		SilenceUsage:  true,
		Long:          "Code extension marketplace",
		Example: strings.Join([]string{
			"  marketplace server --extensions-dir ./extensions",
		}, "\n"),
	}

	cmd.AddCommand(add(), remove(), server(), version())

	cmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")

	return cmd
}
