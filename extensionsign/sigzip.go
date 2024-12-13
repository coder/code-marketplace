package extensionsign

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"io"

	"golang.org/x/xerrors"

	"github.com/coder/code-marketplace/storage/easyzip"
)

func ExtractSignatureManifest(zip []byte) (SignatureManifest, error) {
	r, err := easyzip.GetZipFileReader(zip, ".signature.manifest")
	if err != nil {
		return SignatureManifest{}, xerrors.Errorf("get manifest: %w", err)
	}

	defer r.Close()
	var manifest SignatureManifest
	err = json.NewDecoder(r).Decode(&manifest)
	if err != nil {
		return SignatureManifest{}, xerrors.Errorf("decode manifest: %w", err)
	}
	return manifest, nil
}

func ExtractP7SSig(zip []byte) ([]byte, error) {
	r, err := easyzip.GetZipFileReader(zip, ".signature.p7s")
	if err != nil {
		return nil, xerrors.Errorf("get p7s: %w", err)
	}

	defer r.Close()
	return io.ReadAll(r)
}

// SignAndZipManifest signs a manifest and zips it up
func SignAndZipManifest(certs []*x509.Certificate, secret crypto.Signer, vsixData []byte, manifest json.RawMessage) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	manFile, err := w.Create(".signature.manifest")
	if err != nil {
		return nil, xerrors.Errorf("create manifest: %w", err)
	}

	_, err = manFile.Write(manifest)
	if err != nil {
		return nil, xerrors.Errorf("write manifest: %w", err)
	}

	p7sFile, err := w.Create(".signature.p7s")
	if err != nil {
		return nil, xerrors.Errorf("create empty p7s signature: %w", err)
	}

	signature, err := SigningAlgorithm(vsixData, certs, secret)
	if err != nil {
		return nil, xerrors.Errorf("sign: %w", err)
	}

	_, err = p7sFile.Write(signature)
	if err != nil {
		return nil, xerrors.Errorf("write signature: %w", err)
	}

	err = w.Close()
	if err != nil {
		return nil, xerrors.Errorf("close zip: %w", err)
	}

	return buf.Bytes(), nil
}
