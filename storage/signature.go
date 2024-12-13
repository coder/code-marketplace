package storage

import (
	"context"
	"crypto"
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
	// Signer if provided, will be used to sign extensions. If not provided,
	// no extensions will be signed.
	Signer crypto.Signer
	Logger slog.Logger
	// SaveSigZips is a flag that will save the signed extension to disk.
	// This is useful for debugging, but the server will never use this file.
	saveSigZips bool
	Storage
}

func NewSignatureStorage(logger slog.Logger, signer crypto.Signer, s Storage) *Signature {
	return &Signature{
		Signer:  signer,
		Storage: s,
	}
}

func (s *Signature) SaveSigZips() {
	if !s.saveSigZips {
		s.Logger.Info(context.Background(), "extension signatures will be saved to disk, do not use this in production")
	}
	s.saveSigZips = true
}

func (s *Signature) SigningEnabled() bool {
	return s.Signer != nil
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

	if s.SigningEnabled() && s.saveSigZips {
		signed, err := s.SigZip(ctx, vsix, sigManifestJSON)
		if err != nil {
			s.Logger.Error(ctx, "signing manifest", slog.Error(err))
			return "", xerrors.Errorf("sign and zip manifest: %w", err)
		}
		extra = append(extra, File{
			RelativePath: SignatureZipFilename(manifest),
			Content:      signed,
		})
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
//   - VSCode requires a signature payload to exist, but the context appear
//     to be somewhat optional.
//     Following another open source implementation, it appears the '.signature.p7s'
//     file must exist, but it can be empty.
//     The signature is stored in a '.signature.sig' file, although it is unclear
//     is VSCode ever reads this file.
//     TODO: Properly implement the p7s file, and diverge from the other open
//     source implementation. Ideally this marketplace would match Microsoft's
//     marketplace API.
func (s *Signature) Open(ctx context.Context, fp string) (fs.File, error) {
	if s.SigningEnabled() && strings.HasSuffix(filepath.Base(fp), SigzipFileExtension) {
		base := filepath.Base(fp)
		vsixPath := strings.TrimSuffix(base, SigzipFileExtension)

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

		vsix, err := s.Storage.Open(ctx, filepath.Join(filepath.Dir(fp), vsixPath+".vsix"))
		if err != nil {
			// If this file is missing, it means the extension was added before
			// signatures were handled by the marketplace.
			// TODO: Generate the sig manifest payload and insert it?
			return nil, xerrors.Errorf("open signature manifest: %w", err)
		}
		defer vsix.Close()

		vsixData, err := io.ReadAll(vsix)
		if err != nil {
			return nil, xerrors.Errorf("read signature manifest: %w", err)
		}

		// TODO: Fetch the VSIX payload from the storage
		signed, err := s.SigZip(ctx, vsixData, manifestData)
		if err != nil {
			return nil, xerrors.Errorf("sign and zip manifest: %w", err)
		}

		f := mem.NewFileHandle(mem.CreateFile(fp))
		_, err = f.Write(signed)
		return f, err
	}

	return s.Storage.Open(ctx, fp)
}

func (s *Signature) SigZip(ctx context.Context, vsix []byte, sigManifest []byte) ([]byte, error) {
	signed, err := extensionsign.SignAndZipManifest(s.Signer, vsix, sigManifest)
	if err != nil {
		s.Logger.Error(ctx, "signing manifest", slog.Error(err))
		return nil, xerrors.Errorf("sign and zip manifest: %w", err)
	}
	return signed, nil
}
