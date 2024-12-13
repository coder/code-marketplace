package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/code-marketplace/extensionsign"
)

func signature() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "signature",
		Short:   "Commands for debugging and working with signatures.",
		Hidden:  true, // Debugging tools
		Aliases: []string{"sig", "sigs", "signatures"},
	}
	cmd.AddCommand(compareSignatureSigZips())
	return cmd
}

func compareSignatureSigZips() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "compare",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			decode := func(path string) (extensionsign.SignatureManifest, error) {
				data, err := os.ReadFile(path)
				if err != nil {
					return extensionsign.SignatureManifest{}, xerrors.Errorf("read %q: %w", args[0], err)
				}

				sig, err := extensionsign.ExtractSignatureManifest(data)
				if err != nil {
					return extensionsign.SignatureManifest{}, xerrors.Errorf("unmarshal %q: %w", path, err)
				}
				return sig, nil
			}

			a, err := decode(args[0])
			if err != nil {
				return err
			}
			b, err := decode(args[1])
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(os.Stdout, "Signature A:%s\n", a)
			_, _ = fmt.Fprintf(os.Stdout, "Signature B:%s\n", b)
			err = a.Equal(b)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(os.Stdout, "Signatures are equal\n")
			return nil
		},
	}
	return cmd
}
