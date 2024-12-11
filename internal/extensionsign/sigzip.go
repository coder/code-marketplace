package extensionsign

import (
	"archive/zip"
	"bytes"
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

func Zip(manifest SignatureManifest) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	manFile, err := w.Create(".signature.manifest")
	if err != nil {
		return nil, xerrors.Errorf("create manifest: %w", err)
	}

	err = json.NewEncoder(manFile).Encode(manifest)
	if err != nil {
		return nil, xerrors.Errorf("encode manifest: %w", err)
	}

	err = w.Close()
	if err != nil {
		return nil, xerrors.Errorf("close zip: %w", err)
	}

	return buf.Bytes(), nil
}
