package cli

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	cms "github.com/github/smimesign/ietf-cms"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/code-marketplace/extensionsign"
	"github.com/coder/code-marketplace/storage/easyzip"
)

func signature() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "signature",
		Short:   "Commands for debugging and working with signatures.",
		Hidden:  true, // Debugging tools
		Aliases: []string{"sig", "sigs", "signatures"},
	}

	cmd.AddCommand(compareSignatureSigZips(), verifySig())
	return cmd
}

func verifySig() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify <extension.vsix> <signature.p7s>",
		Short: "Decode & verify a signature archive.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := cmdLogger(cmd)
			ctx := cmd.Context()
			extensionVsix := args[0]
			p7sFile := args[1]

			logger.Info(ctx, fmt.Sprintf("Decoding %q", p7sFile))

			data, err := os.ReadFile(p7sFile)
			if err != nil {
				return xerrors.Errorf("read %q: %w", p7sFile, err)
			}

			msg, err := easyzip.GetZipFileReader(data, extensionVsix)
			if err != nil {
				return xerrors.Errorf("get manifest: %w", err)
			}
			msgData, err := io.ReadAll(msg)
			if err != nil {
				return xerrors.Errorf("read manifest: %w", err)
			}

			signed, err := extensionsign.ExtractP7SSig(data)
			if err != nil {
				return xerrors.Errorf("extract p7s: %w", err)
			}

			fmt.Println("----------------Golang Verify----------------")
			valid, err := goVerify(ctx, logger, msgData, signed)
			if err != nil {
				logger.Error(ctx, "go verify", slog.Error(err))
			}
			logger.Info(ctx, fmt.Sprintf("Valid: %t", valid))

			fmt.Println("----------------OpenSSL Verify----------------")
			valid, err = openSSLVerify(ctx, logger, msgData, signed)
			if err != nil {
				logger.Error(ctx, "openssl verify", slog.Error(err))
			}
			logger.Info(ctx, fmt.Sprintf("Valid: %t", valid))

			fmt.Println("----------------vsce-sign Verify----------------")
			valid, err = vsceSignVerify(ctx, logger, extensionVsix, p7sFile)
			if err != nil {
				logger.Error(ctx, "openssl verify", slog.Error(err))
			}
			logger.Info(ctx, fmt.Sprintf("Valid: %t", valid))

			return nil
		},
	}
	return cmd
}

func goVerify(ctx context.Context, logger slog.Logger, message []byte, signature []byte) (bool, error) {
	sd, err := cms.ParseSignedData(signature)
	if err != nil {
		return false, xerrors.Errorf("new signed data: %w", err)
	}

	fmt.Println("Detached:", sd.IsDetached())
	certs, err := sd.GetCertificates()
	if err != nil {
		return false, xerrors.Errorf("get certs: %w", err)
	}
	fmt.Println("Certificates:", len(certs))

	sdData, err := sd.GetData()
	if err != nil {
		return false, xerrors.Errorf("get data: %w", err)
	}
	fmt.Println("Data:", len(sdData))

	var verifyErr error
	var vcerts [][][]*x509.Certificate

	sys, err := x509.SystemCertPool()
	if err != nil {
		return false, xerrors.Errorf("system cert pool: %w", err)
	}
	opts := x509.VerifyOptions{
		Intermediates: sys,
		Roots:         sys,
	}

	if sd.IsDetached() {
		vcerts, verifyErr = sd.VerifyDetached(message, opts)
	} else {
		vcerts, verifyErr = sd.Verify(opts)
	}
	if verifyErr != nil {
		logger.Error(ctx, "verify", slog.Error(verifyErr))
	}

	certChain := dimensions(vcerts)
	fmt.Println(certChain)
	return verifyErr == nil, nil
}

func openSSLVerify(ctx context.Context, logger slog.Logger, message []byte, signature []byte) (bool, error) {
	// openssl cms -verify -in message_from_alice_for_bob.msg -inform DER -CAfile ehealth_root_ca.cer | openssl cms -decrypt -inform DER -recip bob_etk_pair.pem  | openssl cms -inform DER -cmsout -print
	tmpdir := os.TempDir()
	tmpdir = filepath.Join(tmpdir, "verify-sigs")
	defer os.RemoveAll(tmpdir)
	os.MkdirAll(tmpdir, 0755)
	msgPath := filepath.Join(tmpdir, ".signature.manifest")
	err := os.WriteFile(msgPath, message, 0644)
	if err != nil {
		return false, xerrors.Errorf("write message: %w", err)
	}

	sigPath := filepath.Join(tmpdir, ".signature.p7s")
	err = os.WriteFile(sigPath, signature, 0644)
	if err != nil {
		return false, xerrors.Errorf("write signature: %w", err)
	}

	cmd := exec.CommandContext(ctx, "openssl", "smime", "-verify",
		"-in", sigPath, "-content", msgPath, "-inform", "DER",
		"-CAfile", "/home/steven/go/src/github.com/coder/code-marketplace/extensionsign/testdata/cert2.pem",
	)
	output := &strings.Builder{}
	cmd.Stdout = output
	cmd.Stderr = output
	err = cmd.Run()
	fmt.Println(output.String())
	if err != nil {
		return false, xerrors.Errorf("run verify %q: %w", cmd.String(), err)
	}

	return cmd.ProcessState.ExitCode() == 0, nil
}

func vsceSignVerify(ctx context.Context, logger slog.Logger, vsixPath, sigPath string) (bool, error) {
	bin := os.Getenv("VSCE_SIGN_PATH")
	if bin == "" {
		return false, xerrors.Errorf("VSCE_SIGN_PATH not set")
	}

	cmd := exec.CommandContext(ctx, bin, "verify",
		"--package", vsixPath,
		"--signaturearchive", sigPath,
		"-v",
	)
	fmt.Println(cmd.String())
	output := &strings.Builder{}
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	fmt.Println(output.String())
	if err != nil {
		return false, xerrors.Errorf("run verify %q: %w", cmd.String(), err)
	}

	return cmd.ProcessState.ExitCode() == 0, nil
}

func dimensions(chain [][][]*x509.Certificate) string {
	var str strings.Builder
	for _, top := range chain {
		str.WriteString(fmt.Sprintf("Chain, len=%d\n", len(top)))
		for _, second := range top {
			str.WriteString(fmt.Sprintf("  Certs len=%d\n", len(second)))
			for _, cert := range second {
				str.WriteString(fmt.Sprintf("    Cert: %s\n", cert.Subject))
			}
		}
	}
	return str.String()
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
