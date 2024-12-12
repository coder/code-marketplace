package storage

import (
	"context"
	"encoding/json"

	"golang.org/x/xerrors"

	"github.com/coder/code-marketplace/internal/extensionsign"
)

var _ Storage = (*Signature)(nil)

type Signature struct {
	// SignDesignExtensions is a flag that determines if the signature should
	// include the extension payloads.
	signExtensions bool
	Storage
}

func NewSignatureStorage(signExtensions bool, s Storage) *Signature {
	return &Signature{
		signExtensions: signExtensions,
		Storage:        s,
	}
}

// AddExtension includes the signature manifest of the vsix. Signing happens on
// demand, so leave the manifest unsigned. This is safe to do even if
// 'signExtensions' is disabled, as these files lay dormant until signed.
func (s *Signature) AddExtension(ctx context.Context, manifest *VSIXManifest, vsix []byte, extra ...File) (string, error) {
	sigManifest, err := extensionsign.GenerateSignatureManifest(vsix)
	if err != nil {
		return "", xerrors.Errorf("generate signature manifest: %w", err)
	}

	data, err := json.Marshal(sigManifest)
	if err != nil {
		return "", xerrors.Errorf("encode signature manifest: %w", err)
	}

	return s.Storage.AddExtension(ctx, manifest, vsix, append(extra, File{
		RelativePath: ".signature.manifest",
		Content:      data,
	})...)
}

func (s *Signature) Manifest(ctx context.Context, publisher, name string, version Version) (*VSIXManifest, error) {
	manifest, err := s.Storage.Manifest(ctx, publisher, name, version)
	if err != nil {
		return nil, err
	}

	if s.signExtensions {
		manifest.Assets.Asset = append(manifest.Assets.Asset, VSIXAsset{
			Type:        VSIXSignatureType,
			Path:        "extension.sigzip",
			Addressable: "true",
		})
	}

	return manifest, nil
}
