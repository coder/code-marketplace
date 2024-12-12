package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"github.com/spf13/afero/mem"
	"golang.org/x/xerrors"

	"github.com/coder/code-marketplace/extensionsign"
)

var _ Storage = (*Signature)(nil)

const (
	sigzipFilename  = "extension.sigzip"
	sigManifestName = ".signature.manifest"
)

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
		RelativePath: sigManifestName,
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
			Path:        sigzipFilename,
			Addressable: "true",
		})
	}
	return manifest, nil
}

func (s *Signature) Open(ctx context.Context, fp string) (fs.File, error) {
	if s.signExtensions && filepath.Base(fp) == "p7s.sig" {
		// This file must exist, and it is always empty
		return mem.NewFileHandle(mem.CreateFile("p7s.sig")), nil
	}
	if s.signExtensions && filepath.Base(fp) == sigzipFilename {
		// hijack this request, sign the sig manifest
		manifest, err := s.Storage.Open(ctx, filepath.Join(filepath.Dir(fp), sigManifestName))
		if err != nil {
			fmt.Println(err)
			return nil, xerrors.Errorf("open signature manifest: %w", err)
		}
		defer manifest.Close()

		key, _ := extensionsign.GenerateKey()
		manifestData, err := io.ReadAll(manifest)
		if err != nil {
			return nil, xerrors.Errorf("read signature manifest: %w", err)
		}

		signed, err := extensionsign.SignAndZipManifest(key, manifestData)
		if err != nil {
			return nil, xerrors.Errorf("sign and zip manifest: %w", err)
		}

		f := mem.NewFileHandle(mem.CreateFile(sigzipFilename))
		_, err = f.Write(signed)
		return f, err
	}

	return s.Storage.Open(ctx, fp)
}
