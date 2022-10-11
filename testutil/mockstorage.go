package testutil

import (
	"context"
	"errors"
	"net/http"
	"os"

	"github.com/coder/code-marketplace/storage"
)

// MockStorage implements storage.Storage for tests.
type MockStorage struct{}

func NewMockStorage() *MockStorage {
	return &MockStorage{}
}

func (s *MockStorage) AddExtension(ctx context.Context, manifest *storage.VSIXManifest, vsix []byte) (string, error) {
	return "", errors.New("not implemented")
}

func (s *MockStorage) FileServer() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/nonexistent" {
			http.Error(rw, "not found", http.StatusNotFound)
		} else {
			_, _ = rw.Write([]byte("foobar"))
		}
	})
}

func (s *MockStorage) Manifest(ctx context.Context, publisher, name, version string) (*storage.VSIXManifest, error) {
	for _, ext := range Extensions {
		if ext.Publisher == publisher && ext.Name == name {
			for _, ver := range ext.Versions {
				if ver == version {
					return ConvertExtensionToManifest(ext, ver), nil
				}
			}
			break
		}
	}
	return nil, os.ErrNotExist
}

func (s *MockStorage) RemoveExtension(ctx context.Context, publisher, name, version string) error {
	return errors.New("not implemented")
}

func (s *MockStorage) WalkExtensions(ctx context.Context, fn func(manifest *storage.VSIXManifest, versions []string) error) error {
	for _, ext := range Extensions {
		if err := fn(ConvertExtensionToManifest(ext, ext.Versions[0]), ext.Versions); err != nil {
			return nil
		}
	}
	return nil
}

func (s *MockStorage) Versions(ctx context.Context, publisher, name string) ([]string, error) {
	return nil, errors.New("not implemented")
}
