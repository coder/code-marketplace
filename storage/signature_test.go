package storage_test

import (
	"crypto"
	"crypto/x509"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/code-marketplace/extensionsign"
	"github.com/coder/code-marketplace/storage"
)

func expectSignature(manifest *storage.VSIXManifest) {
	manifest.Assets.Asset = append(manifest.Assets.Asset, storage.VSIXAsset{
		Type:        storage.VSIXSignatureType,
		Path:        storage.SignatureZipFilename(manifest),
		Addressable: "true",
	})
}

//nolint:revive // test control flag
func signed(signer bool, factory func(t *testing.T) testStorage) func(t *testing.T) testStorage {
	return func(t *testing.T) testStorage {
		st := factory(t)
		var key crypto.Signer
		var exp func(*storage.VSIXManifest)
		if signer {
			key, _ = extensionsign.GenerateKey()
			exp = expectSignature
		}

		sst, err := storage.NewSignatureStorage(slog.Make(), key, []*x509.Certificate{}, st.storage)
		require.NoError(t, err)
		return testStorage{
			storage:          sst,
			write:            st.write,
			exists:           st.exists,
			expectedManifest: exp,
		}
	}
}
