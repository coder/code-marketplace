package extensionsign

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/xerrors"

	"github.com/coder/code-marketplace/storage/easyzip"
)

// SignatureManifest should be serialized to JSON before being signed.
type SignatureManifest struct {
	Package File
	// Entries is base64(filepath) -> File
	Entries map[string]File
}

func (a SignatureManifest) String() string {
	return fmt.Sprintf("Package %q with Entries: %d", a.Package.Digests.SHA256, len(a.Entries))
}

// Equal is helpful for debugging to know if two manifests are equal.
// They can change if any file is removed/added/edited to an extension.
func (a SignatureManifest) Equal(b SignatureManifest) error {
	var errs []error
	if err := a.Package.Equal(b.Package); err != nil {
		errs = append(errs, xerrors.Errorf("package: %w", err))
	}

	if len(a.Entries) != len(b.Entries) {
		errs = append(errs, xerrors.Errorf("entry count mismatch: %d != %d", len(a.Entries), len(b.Entries)))
	}

	for k, v := range a.Entries {
		if _, ok := b.Entries[k]; !ok {
			errs = append(errs, xerrors.Errorf("entry %q not found in second set", k))
			continue
		}
		if err := v.Equal(b.Entries[k]); err != nil {
			errs = append(errs, xerrors.Errorf("entry %q: %w", k, err))
		}
	}
	return errors.Join(errs...)
}

type File struct {
	Size    int64   `json:"size"`
	Digests Digests `json:"digests"`
}

func (f File) Equal(b File) error {
	if f.Size != b.Size {
		return xerrors.Errorf("size mismatch: %d != %d", f.Size, b.Size)
	}
	if f.Digests.SHA256 != b.Digests.SHA256 {
		return xerrors.Errorf("sha256 mismatch: %s != %s", f.Digests.SHA256, b.Digests.SHA256)
	}
	return nil
}

func FileManifest(file io.Reader) (File, error) {
	hash := sha256.New()

	n, err := io.Copy(hash, file)
	if err != nil {
		return File{}, xerrors.Errorf("hash file: %w", err)
	}

	return File{
		Size: n,
		Digests: Digests{
			SHA256: base64.StdEncoding.EncodeToString(hash.Sum(nil)),
		},
	}, nil
}

type Digests struct {
	SHA256 string `json:"sha256"`
}

// GenerateSignatureManifest generates a signature manifest for a VSIX file.
// It does not sign the manifest. The manifest is the base64 encoded file path
// followed by the sha256 hash of the file, and it's size.
func GenerateSignatureManifest(vsixFile []byte) (SignatureManifest, error) {
	pkgManifest, err := FileManifest(bytes.NewReader(vsixFile))
	if err != nil {
		return SignatureManifest{}, xerrors.Errorf("package manifest: %w", err)
	}

	manifest := SignatureManifest{
		Package: pkgManifest,
		Entries: make(map[string]File),
	}

	err = easyzip.ExtractZip(vsixFile, func(name string, reader io.Reader) error {
		fm, err := FileManifest(reader)
		if err != nil {
			return xerrors.Errorf("file %q: %w", name, err)
		}
		manifest.Entries[base64.StdEncoding.EncodeToString([]byte(name))] = fm
		return nil
	})

	if err != nil {
		return SignatureManifest{}, err
	}

	return manifest, nil
}
