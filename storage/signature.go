package storage

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/spf13/afero/mem"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/code-marketplace/extensionsign"
)

var _ Storage = (*Signature)(nil)

const (
	SigzipFileExtension = ".signature.p7s"
	sigManifestName     = ".signature.manifest"
)

func SignatureZipFilename(manifest *VSIXManifest) string {
	return ExtensionVSIXNameFromManifest(manifest) + SigzipFileExtension
}

// Signature is a storage wrapper that can sign extensions on demand.
type Signature struct {
	Logger                 slog.Logger
	IncludeEmptySignatures bool
	Storage
}

func NewSignatureStorage(logger slog.Logger, includeEmptySignatures bool, s Storage) *Signature {
	if includeEmptySignatures {
		logger.Info(context.Background(), "Signature storage enabled, if using VSCode on Windows, this will not work.")
	}
	return &Signature{
		Logger:                 logger,
		IncludeEmptySignatures: includeEmptySignatures,
		Storage:                s,
	}
}

func (s *Signature) SigningEnabled() bool {
	return s.IncludeEmptySignatures
}

// AddExtension includes the signature manifest of the vsix. Signing happens on
// demand, so leave the manifest unsigned. This is safe to do even if
// 'signExtensions' is disabled, as these files lay dormant until signed.
func (s *Signature) AddExtension(ctx context.Context, manifest *VSIXManifest, vsix []byte, extra ...File) (string, error) {
	sigManifest, err := extensionsign.GenerateSignatureManifest(vsix)
	if err != nil {
		return "", xerrors.Errorf("generate signature manifest: %w", err)
	}

	sigManifestJSON, err := json.Marshal(sigManifest)
	if err != nil {
		return "", xerrors.Errorf("encode signature manifest: %w", err)
	}

	return s.Storage.AddExtension(ctx, manifest, vsix, append(extra, File{
		RelativePath: sigManifestName,
		Content:      sigManifestJSON,
	})...)
}

func (s *Signature) Manifest(ctx context.Context, publisher, name string, version Version) (*VSIXManifest, error) {
	manifest, err := s.Storage.Manifest(ctx, publisher, name, version)
	if err != nil {
		return nil, err
	}

	if s.SigningEnabled() {
		for _, asset := range manifest.Assets.Asset {
			if asset.Path == SignatureZipFilename(manifest) {
				// Already signed
				return manifest, nil
			}
		}
		manifest.Assets.Asset = append(manifest.Assets.Asset, VSIXAsset{
			Type:        VSIXSignatureType,
			Path:        SignatureZipFilename(manifest),
			Addressable: "true",
		})
		return manifest, nil
	}
	return manifest, nil
}

// Open will intercept requests for signed extensions payload.
// It does this by looking for 'SigzipFileExtension' or p7s.sig.
//
// The signed payload and signing process is taken from:
// https://github.com/filiptronicek/node-ovsx-sign
//
// Some notes:
//
//   - VSCodium requires a signature to exist, but it does appear to actually read
//     the signature. Meaning the signature could be empty, incorrect, or a
//     picture of cat and it would work. There is no signature verification.
//
//   - VSCode requires a signature payload to exist, but the content is optional
//     for linux users.
//     For windows users, the signature must be valid, and this implementation
//     will not work.
func (s *Signature) Open(ctx context.Context, fp string) (fs.File, error) {
	if s.SigningEnabled() && strings.HasSuffix(filepath.Base(fp), SigzipFileExtension) {
		// hijack this request, sign the sig manifest
		manifest, err := s.Storage.Open(ctx, filepath.Join(filepath.Dir(fp), sigManifestName))
		if err != nil {
			// If this file is missing, it means the extension was added before
			// signatures were handled by the marketplace.
			// TODO: Generate the sig manifest payload and insert it?
			return nil, xerrors.Errorf("open signature manifest: %w", err)
		}
		defer manifest.Close()

		manifestData, err := io.ReadAll(manifest)
		if err != nil {
			return nil, xerrors.Errorf("read signature manifest: %w", err)
		}

		signed, err := s.SigZip(ctx, manifestData)
		if err != nil {
			return nil, xerrors.Errorf("sign and zip manifest: %w", err)
		}

		f := mem.NewFileHandle(mem.CreateFile(fp))
		_, err = f.Write(signed)
		return f, err
	}

	return s.Storage.Open(ctx, fp)
}

// SigZip currently just returns an empty signature.
func (s *Signature) SigZip(ctx context.Context, sigManifest []byte) ([]byte, error) {
	signed, err := extensionsign.IncludeEmptySignature(sigManifest)
	if err != nil {
		s.Logger.Error(ctx, "signing manifest", slog.Error(err))
		return nil, xerrors.Errorf("sign and zip manifest: %w", err)
	}
	return signed, nil
}
