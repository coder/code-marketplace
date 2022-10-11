package testutil

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"

	"github.com/coder/code-marketplace/database"
	"github.com/coder/code-marketplace/storage"
)

// MockDB implements database.Database for tests.
type MockDB struct {
	exts []*database.Extension
}

func NewMockDB(exts []*database.Extension) *MockDB {
	return &MockDB{exts: exts}
}

func (db *MockDB) GetExtensionAssetPath(ctx context.Context, asset *database.Asset, baseURL url.URL) (string, error) {
	if asset.Publisher == "error" {
		return "", errors.New("fake error")
	}
	if asset.Publisher == "notexist" {
		return "", os.ErrNotExist
	}
	assetPath := "foo"
	if asset.Type == storage.VSIXAssetType {
		assetPath = "extension.vsix"
	}
	return strings.Join([]string{baseURL.Path, "files", asset.Publisher, asset.Extension, asset.Version, assetPath}, "/"), nil
}

func (db *MockDB) GetExtensions(ctx context.Context, filter database.Filter, flags database.Flag, baseURL url.URL) ([]*database.Extension, int, error) {
	if flags&database.Unpublished != 0 {
		return nil, 0, errors.New("fake error")
	}
	if len(filter.Criteria) == 0 {
		return nil, 0, nil
	}
	return db.exts, len(db.exts), nil
}
