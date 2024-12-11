package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func Root() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "code-marketplace",
		SilenceErrors: true,
		SilenceUsage:  true,
		Long:          "Code extension marketplace",
		Example: strings.Join([]string{
			"  code-marketplace server --extensions-dir ./extensions",
		}, "\n"),
	}

	cmd.AddCommand(add(), remove(), server(), version(), signature())

	cmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")

	return cmd
}
