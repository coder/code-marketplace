package storage

import (
	"context"
	"errors"
	"net/http"
	"os"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

const ArtifactoryTokenEnvKey = "ARTIFACTORY_TOKEN"

// Artifactory implements Storage.  It stores extensions remotely through
// Artifactory by both copying the VSIX and extracting said VSIX to a tree
// structure in the form of publisher/extension/version to easily serve
// individual assets via HTTP.
type Artifactory struct {
	logger slog.Logger
	repo   string
	token  string
	uri    string
}

func NewArtifactoryStorage(uri, repo string, logger slog.Logger) (*Artifactory, error) {
	token := os.Getenv(ArtifactoryTokenEnvKey)
	if token == "" {
		return nil, xerrors.Errorf("the %s environment variable must be set", ArtifactoryTokenEnvKey)
	}

	if !strings.HasSuffix(uri, "/") {
		uri = uri + "/"
	}

	return &Artifactory{
		logger: logger,
		repo:   repo,
		token:  token,
		uri:    uri,
	}, nil
}

func (s *Artifactory) AddExtension(ctx context.Context, manifest *VSIXManifest, vsix []byte) (string, error) {
	return "", errors.New("not implemented")
}

func (s *Artifactory) FileServer() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		http.Error(rw, "not found", http.StatusNotFound)
	})
}

func (s *Artifactory) Manifest(ctx context.Context, publisher, name, version string) (*VSIXManifest, error) {
	return nil, errors.New("not implemented")
}

func (s *Artifactory) RemoveExtension(ctx context.Context, publisher, name, version string) error {
	return errors.New("not implemented")
}

func (s *Artifactory) WalkExtensions(ctx context.Context, fn func(manifest *VSIXManifest, versions []string) error) error {
	return errors.New("not implemented")
}

func (s *Artifactory) Versions(ctx context.Context, publisher, name string) ([]string, error) {
	return nil, errors.New("not implemented")
}
