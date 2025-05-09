package storage_test

import (
	"testing"

	"cdr.dev/slog"
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
		key := false
		var exp func(*storage.VSIXManifest)
		if signer {
			key = true
			exp = expectSignature
		}

		return testStorage{
			storage:          storage.NewSignatureStorage(slog.Make(), key, st.storage),
			write:            st.write,
			exists:           st.exists,
			expectedManifest: exp,
		}
	}
}
