package storage_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/code-marketplace/api/httpapi"
	"github.com/coder/code-marketplace/storage"
)

const ArtifactoryURIEnvKey = "ARTIFACTORY_URI"
const ArtifactoryRepoEnvKey = "ARTIFACTORY_REPO"

func readFiles(depth int, root, current string) ([]storage.ArtifactoryFile, error) {
	files, err := os.ReadDir(filepath.FromSlash(path.Join(root, current)))
	if err != nil {
		return nil, err
	}
	var artifactoryFiles []storage.ArtifactoryFile
	for _, file := range files {
		current := path.Join(current, file.Name())
		artifactoryFiles = append(artifactoryFiles, storage.ArtifactoryFile{
			URI:    current,
			Folder: file.IsDir(),
		})
		if depth > 1 {
			files, err := readFiles(depth-1, root, current)
			if err != nil {
				return nil, err
			}
			artifactoryFiles = append(artifactoryFiles, files...)
		}
	}
	return artifactoryFiles, nil
}

func handleArtifactory(extdir, repo string, rw http.ResponseWriter, r *http.Request) error {
	if r.URL.Query().Has("list") {
		depth := 1
		if r.URL.Query().Has("depth") {
			var err error
			depth, err = strconv.Atoi(r.URL.Query().Get("depth"))
			if err != nil {
				return err
			}
		}
		files, err := readFiles(depth, filepath.Join(extdir, strings.TrimPrefix(r.URL.Path, "/api/storage")), "/")
		if err != nil {
			return err
		}
		httpapi.Write(rw, http.StatusOK, &storage.ArtifactoryList{Files: files})
	} else if r.Method == http.MethodDelete {
		filename := filepath.Join(extdir, filepath.FromSlash(r.URL.Path))
		_, err := os.Stat(filename)
		if err != nil {
			return err
		}
		err = os.RemoveAll(filename)
		if err != nil {
			return err
		}
		_, err = rw.Write([]byte("ok"))
		if err != nil {
			return err
		}
	} else if r.Method == http.MethodPut {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}
		filename := filepath.FromSlash(r.URL.Path)
		err = os.MkdirAll(filepath.Dir(filepath.Join(extdir, filename)), 0o755)
		if err != nil {
			return err
		}
		err = os.WriteFile(filepath.Join(extdir, filename), b, 0o644)
		if err != nil {
			return err
		}
		_, err = rw.Write([]byte("ok"))
		if err != nil {
			return err
		}
	} else if r.Method == http.MethodGet {
		filename := filepath.Join(extdir, filepath.FromSlash(r.URL.Path))
		stat, err := os.Stat(filename)
		if err != nil {
			return err
		}
		if stat.IsDir() {
			// This is not the right response but we only use it in `exists` below to
			// check if a folder exists so it is good enough.
			httpapi.Write(rw, http.StatusOK, &storage.ArtifactoryList{})
			return nil
		}
		b, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		_, err = rw.Write(b)
		if err != nil {
			return err
		}
	} else {
		http.Error(rw, "not implemented", http.StatusNotImplemented)
	}
	return nil
}

func artifactoryFactory(t *testing.T) testStorage {
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	token := os.Getenv(storage.ArtifactoryTokenEnvKey)
	repo := os.Getenv(ArtifactoryRepoEnvKey)
	uri := os.Getenv(ArtifactoryURIEnvKey)
	if uri == "" {
		// If no URL was specified use a mock.
		extdir := t.TempDir()
		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			err := handleArtifactory(extdir, repo, rw, r)
			if err != nil {
				code := http.StatusInternalServerError
				message := err.Error()
				if errors.Is(err, os.ErrNotExist) {
					code = http.StatusNotFound
				} else if errors.Is(err, syscall.EISDIR) {
					code = http.StatusConflict
					message = "Expected a file but found a folder"
				}
				httpapi.Write(rw, code, &storage.ArtifactoryResponse{
					Errors: []storage.ArtifactoryError{{
						Status:  code,
						Message: message,
					}},
				})
			}
		}))
		uri = server.URL
		repo = "extensions"
		token = "mock"
		t.Cleanup(server.Close)
	} else {
		if token == "" {
			t.Fatalf("the %s environment variable must be set", storage.ArtifactoryTokenEnvKey)
		}
		if repo == "" {
			t.Fatalf("the %s environment variable must be set", ArtifactoryRepoEnvKey)
		}
	}
	// Since we only have one repo use sub-directories to prevent clashes.
	repo = path.Join(repo, t.Name())
	s, err := storage.NewArtifactoryStorage(context.Background(), &storage.ArtifactoryOptions{
		Logger: logger,
		Repo:   repo,
		Token:  token,
		URI:    uri,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		req, err := http.NewRequest(http.MethodDelete, uri+repo, nil)
		if err != nil {
			t.Log("Failed to clean up", err)
			return
		}
		req.Header.Add("X-JFrog-Art-Api", token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Log("Failed to clean up", err)
			return
		}
		defer res.Body.Close()
	})
	if !strings.HasSuffix(uri, "/") {
		uri = uri + "/"
	}
	return testStorage{
		storage: s,
		write: func(content []byte, elem ...string) {
			req, err := http.NewRequest(http.MethodPut, uri+path.Join(repo, path.Join(elem...)), bytes.NewReader(content))
			require.NoError(t, err)
			req.Header.Add("X-JFrog-Art-Api", token)
			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()
		},
		exists: func(elem ...string) bool {
			req, err := http.NewRequest(http.MethodGet, uri+path.Join(repo, path.Join(elem...)), nil)
			require.NoError(t, err)
			req.Header.Add("X-JFrog-Art-Api", token)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return false
			}
			defer res.Body.Close()
			return res.StatusCode == http.StatusOK
		},
	}
}
