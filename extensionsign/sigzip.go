package extensionsign

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/rand"
	"encoding/json"

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

// SignAndZipManifest signs a manifest and zips it up
// Sign
func SignAndZipManifest(secret crypto.Signer, manifest json.RawMessage) ([]byte, error) {
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

	// Empty file
	_, err = w.Create(".signature.p7s")
	if err != nil {
		return nil, xerrors.Errorf("create empty p7s signature: %w", err)
	}

	// Actual sig
	sigFile, err := w.Create(".signature.sig")
	if err != nil {
		return nil, xerrors.Errorf("create signature: %w", err)
	}

	signature, err := secret.Sign(rand.Reader, manifest, crypto.Hash(0))
	if err != nil {
		return nil, xerrors.Errorf("sign: %w", err)
	}

	_, err = sigFile.Write(signature)
	if err != nil {
		return nil, xerrors.Errorf("write signature: %w", err)
	}

	err = w.Close()
	if err != nil {
		return nil, xerrors.Errorf("close zip: %w", err)
	}

	return buf.Bytes(), nil
}
