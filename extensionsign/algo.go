package extensionsign

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"

	cms "github.com/github/smimesign/ietf-cms"
	"golang.org/x/xerrors"
)

var SigningAlgorithm = OpenSSLSign

func CMSAlgo(data []byte, certs []*x509.Certificate, signer crypto.Signer) (result []byte, err error) {
	return cms.SignDetached(data, certs, signer)
}

// openssl smime -sign -signer <cert> -inkey <key> -binary -in .signature.manifest -outform der -out openssl.p7s
func OpenSSLSign(data []byte, certs []*x509.Certificate, signer crypto.Signer) (result []byte, err error) {
	tmpdir := os.TempDir()
	tmpdir = filepath.Join(tmpdir, "sign-sigs")
	defer os.RemoveAll(tmpdir)

	err = os.MkdirAll(tmpdir, 0755)
	if err != nil {
		return nil, xerrors.Errorf("create temp dir: %w", err)
	}

	certPath := filepath.Join(tmpdir, "certs.pem")
	certFile, err := os.OpenFile(certPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, xerrors.Errorf("open cert file: %w", err)
	}

	for _, cert := range certs {
		if len(cert.Raw) == 0 {
			return nil, xerrors.Errorf("empty certificate")
		}
		err = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
		if err != nil {
			return nil, err
		}
	}

	keyPath := "/home/steven/go/src/github.com/coder/code-marketplace/extensionsign/testdata/key2.pem"
	//keyFile, err := os.Open(keyPath)
	//if err != nil {
	//	return nil, err
	//}
	//pem.Encode(keyFile, &pem.Block{Type: "PRIVATE KEY", )

	msgPath := filepath.Join(tmpdir, ".signature.manifest")
	messageFile, err := os.OpenFile(msgPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	_, err = messageFile.Write(data)
	if err != nil {
		return nil, xerrors.Errorf("write message: %w", err)
	}

	signed := filepath.Join(tmpdir, "openssl.p7s")
	cmd := exec.CommandContext(context.Background(), "openssl", "cms", "-sign",
		"-signer", certPath,
		"-inkey", keyPath,
		"-binary",
		"-in", msgPath,
		"-outform", "der",
		"-out", signed,
	)

	err = cmd.Run()
	if err != nil {
		return nil, xerrors.Errorf("run openssl: %w", err)
	}

	return os.ReadFile(signed)
}
