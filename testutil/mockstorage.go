package testutil

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/afero/mem"

	"github.com/coder/code-marketplace/storage"
)

var _ storage.Storage = (*MockStorage)(nil)

// MockStorage implements storage.Storage for tests.
type MockStorage struct{}

func NewMockStorage() *MockStorage {
	return &MockStorage{}
}

func (s *MockStorage) AddExtension(ctx context.Context, manifest *storage.VSIXManifest, vsix []byte, extra ...storage.File) (string, error) {
	return "", errors.New("not implemented")
}
func (s *MockStorage) Open(ctx context.Context, path string) (fs.File, error) {
	if filepath.Base(path) == "/nonexistent" {
		return nil, fs.ErrNotExist
	}

	f := mem.NewFileHandle(mem.CreateFile(path))
	_, _ = f.Write([]byte("foobar"))
	return f, nil
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

func (s *MockStorage) Manifest(ctx context.Context, publisher, name string, version storage.Version) (*storage.VSIXManifest, error) {
	for _, ext := range Extensions {
		if ext.Publisher == publisher && ext.Name == name {
			for _, ver := range ext.Versions {
				// Use the string encoding to match since that is how the real storage
				// implementations will do it too.
				if ver.String() == version.String() {
					return ConvertExtensionToManifest(ext, ver), nil
				}
			}
			break
		}
	}
	return nil, os.ErrNotExist
}

func (s *MockStorage) RemoveExtension(ctx context.Context, publisher, name string, version storage.Version) error {
	return errors.New("not implemented")
}

func (s *MockStorage) WalkExtensions(ctx context.Context, fn func(manifest *storage.VSIXManifest, versions []storage.Version) error) error {
	for _, ext := range Extensions {
		versions := make([]storage.Version, len(ext.Versions))
		copy(versions, ext.Versions)
		sort.Sort(storage.ByVersion(versions))
		if err := fn(ConvertExtensionToManifest(ext, versions[0]), versions); err != nil {
			return nil
		}
	}
	return nil
}

func (s *MockStorage) Versions(ctx context.Context, publisher, name string) ([]storage.Version, error) {
	return nil, errors.New("not implemented")
}
