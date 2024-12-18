package storage

import (
	"context"
	"crypto"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/code-marketplace/api/httpapi"
	"github.com/coder/code-marketplace/api/httpmw"

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

// FileServer will intercept requests for signed extensions payload.
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
//     For windows (maybe mac?) users, the signature must be valid, and this
//     implementation will not work.
func (s *Signature) FileServer() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if s.SigningEnabled() && strings.HasSuffix(r.URL.Path, SigzipFileExtension) {
			// hijack this request, return an empty signature payload
			signed, err := extensionsign.IncludeEmptySignature()
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.ErrorResponse{
					Message:   "Unable to generate empty signature for extension",
					Detail:    err.Error(),
					RequestID: httpmw.RequestID(r),
				})
				return
			}

			rw.Header().Set("Content-Length", strconv.FormatInt(int64(len(signed)), 10))
			rw.WriteHeader(http.StatusOK)
			_, _ = rw.Write(signed)
			return
		}

		s.Storage.FileServer().ServeHTTP(rw, r)
	})
}

func (s *Signature) SigZip(ctx context.Context, vsix []byte, sigManifest []byte) ([]byte, error) {
	signed, err := extensionsign.SignAndZipManifest(s.Signer, vsix, sigManifest)
	if err != nil {
		s.Logger.Error(ctx, "signing manifest", slog.Error(err))
		return nil, xerrors.Errorf("sign and zip manifest: %w", err)
	}
	return signed, nil
}
