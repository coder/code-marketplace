package storage_test

import (
	"testing"

	"github.com/coder/code-marketplace/extensionsign"
	"github.com/coder/code-marketplace/storage"
)

func signed(factory func(t *testing.T) testStorage) func(t *testing.T) testStorage {
	return func(t *testing.T) testStorage {
		st := factory(t)
		key, _ := extensionsign.GenerateKey()

		return testStorage{
			storage: storage.NewSignatureStorage(key, st.storage),
			write:   st.write,
			exists:  st.exists,
		}
	}
}
