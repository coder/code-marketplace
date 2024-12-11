package extensionsign

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"encoding/json"

	"golang.org/x/xerrors"

	"github.com/coder/code-marketplace/storage"
)

func ExtractSignatureManifest(zip []byte) (SignatureManifest, error) {
	r, err := storage.GetZipFileReader(zip, ".signature.manifest")
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

// SignAndZip signs a manifest and zips it up
// Should be a PCKS8 key
// TODO: Support other key types
func SignAndZip(key ed25519.PrivateKey, manifest SignatureManifest) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	manFile, err := w.Create(".signature.manifest")
	if err != nil {
		return nil, xerrors.Errorf("create manifest: %w", err)
	}

	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return nil, xerrors.Errorf("encode manifest: %w", err)
	}

	_, err = manFile.Write(manifestData)
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

	signature := ed25519.Sign(key, manifestData)

	//signature, err := key.Sign(rand.Reader, manifestData, crypto.SHA512)
	//if err != nil {
	//	return nil, xerrors.Errorf("sign: %w", err)
	//}

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
