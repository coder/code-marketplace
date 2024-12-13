package storage_test

import (
	"crypto"
	"testing"

	"github.com/coder/code-marketplace/extensionsign"
	"github.com/coder/code-marketplace/storage"
)

func expectSignature(manifest *storage.VSIXManifest) {
	manifest.Assets.Asset = append(manifest.Assets.Asset, storage.VSIXAsset{
		Type:        storage.VSIXSignatureType,
		Path:        storage.SigzipFileExtension,
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

		return testStorage{
			storage:          storage.NewSignatureStorage(key, st.storage),
			write:            st.write,
			exists:           st.exists,
			expectedManifest: exp,
		}
	}
}
