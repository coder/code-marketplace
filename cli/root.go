package cli

import (
	"github.com/spf13/cobra"
)

func Root() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "marketplace",
		SilenceErrors: true,
		SilenceUsage:  true,
		Long:          "Code extension marketplace",
		Example:       "  marketplace server --extensions-dir /path/to/extensions",
	}

	cmd.AddCommand(add(), server(), version())

	cmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")

	return cmd
}
