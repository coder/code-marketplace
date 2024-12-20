package storage

import (
	"context"
	"net/http"
	"strconv"
	"strings"

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
	Logger                 slog.Logger
	IncludeEmptySignatures bool
	Storage
}

func NewSignatureStorage(logger slog.Logger, includeEmptySignatures bool, s Storage) *Signature {
	if includeEmptySignatures {
		logger.Info(context.Background(), "Signature storage enabled, if using VS Code on Windows or macOS, this will not work.")
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
// The signed payload is completely empty. Nothing it actually signed.
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
