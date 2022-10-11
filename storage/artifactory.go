package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/code-marketplace/util"
)

const ArtifactoryTokenEnvKey = "ARTIFACTORY_TOKEN"

type ArtifactoryError struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

type ArtifactoryResponse struct {
	Errors []ArtifactoryError `json:"errors"`
}

type ArtifactoryFile struct {
	URI    string `json:"uri"`
	Folder bool   `json:"folder"`
}

type ArtifactoryFolder struct {
	Children []ArtifactoryFile `json:"children"`
}

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
		repo:   path.Clean(repo),
		token:  token,
		uri:    uri,
	}, nil
}

// request makes a request against Artifactory and returns the response.  If
// there is an error it reads the response first to get any error messages.  The
// code is returned so it can be relayed when proxying file requests.  404s are
// turned into os.ErrNotExist errors.
func (s *Artifactory) request(ctx context.Context, method, endpoint string, r io.Reader) (*http.Response, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.uri+endpoint, r)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	req.Header.Add("X-JFrog-Art-Api", s.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, resp.StatusCode, os.ErrNotExist
		}
		var ar ArtifactoryResponse
		err = json.NewDecoder(resp.Body).Decode(&ar)
		if err != nil {
			s.logger.Warn(ctx, "failed to unmarshal response", slog.F("error", err))
		}
		messages := []string{}
		for _, e := range ar.Errors {
			if e.Message != "" {
				messages = append(messages, e.Message)
			}
		}
		if len(messages) == 0 {
			messages = append(messages, "the server did not provide any additional details")
		}
		return nil, resp.StatusCode, xerrors.Errorf("request failed with code %d: %s", resp.StatusCode, strings.Join(messages, ", "))
	}
	return resp, resp.StatusCode, nil
}

func (s *Artifactory) list(ctx context.Context, endpoint string) ([]ArtifactoryFile, int, error) {
	ctx = slog.With(ctx, slog.F("path", endpoint), slog.F("repo", s.repo))
	s.logger.Debug(ctx, "listing")
	resp, code, err := s.request(ctx, http.MethodGet, path.Join("api/storage", s.repo, endpoint), nil)
	if err != nil {
		return nil, code, err
	}
	defer resp.Body.Close()
	var ar ArtifactoryFolder
	err = json.NewDecoder(resp.Body).Decode(&ar)
	if err != nil {
		return nil, code, err
	}
	return ar.Children, code, nil
}

func (s *Artifactory) read(ctx context.Context, endpoint string) (io.ReadCloser, int, error) {
	resp, code, err := s.request(ctx, http.MethodGet, path.Join(s.repo, endpoint), nil)
	if err != nil {
		return nil, code, err
	}
	return resp.Body, code, err
}

func (s *Artifactory) delete(ctx context.Context, endpoint string) (int, error) {
	ctx = slog.With(ctx, slog.F("path", endpoint), slog.F("repo", s.repo))
	s.logger.Debug(ctx, "deleting")
	resp, code, err := s.request(ctx, http.MethodDelete, path.Join(s.repo, endpoint), nil)
	if err != nil {
		return code, err
	}
	defer resp.Body.Close()
	return code, nil
}

func (s *Artifactory) upload(ctx context.Context, endpoint string, r io.Reader) (int, error) {
	ctx = slog.With(ctx, slog.F("path", endpoint), slog.F("repo", s.repo))
	s.logger.Debug(ctx, "uploading")
	resp, code, err := s.request(ctx, http.MethodPut, path.Join(s.repo, endpoint), r)
	if err != nil {
		return code, err
	}
	defer resp.Body.Close()
	return code, nil
}

func (s *Artifactory) AddExtension(ctx context.Context, manifest *VSIXManifest, vsix []byte) (string, error) {
	// Extract the zip to the correct path.
	identity := manifest.Metadata.Identity
	dir := path.Join(identity.Publisher, identity.ID, identity.Version)

	// Uploading every file in an extension such as ms-python.python can take
	// quite a while (16 minutes!!).  As a compromise only extract a file if it
	// might be directly requested by VS Code.  This includes the manifest, any
	// assets listed as addressable in that manifest, and the browser entry point.
	var browser string
	assets := []string{"extension.vsixmanifest"}
	for _, a := range manifest.Assets.Asset {
		if a.Addressable == "true" {
			assets = append(assets, a.Path)
		}
		// The browser entry point is listed in the package.json (which they also
		// confusingly call the manifest) rather than the top-level VSIX manifest.
		if a.Type == ManifestAssetType {
			packageJSON, err := ReadVSIXPackageJSON(vsix, a.Path)
			if err != nil {
				return "", err
			}
			if packageJSON.Browser != "" {
				browser = path.Join(path.Dir(a.Path), path.Clean(packageJSON.Browser))
			}
		}
	}

	err := ExtractZip(vsix, func(name string, r io.Reader) error {
		if util.Contains(assets, name) || (browser != "" && strings.HasPrefix(name, browser)) {
			_, err := s.upload(ctx, path.Join(dir, name), r)
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	// Copy the VSIX itself as well.
	vsixName := fmt.Sprintf("%s.vsix", ExtensionID(manifest))
	_, err = s.upload(ctx, path.Join(dir, vsixName), bytes.NewReader(vsix))
	if err != nil {
		return "", err
	}

	return s.uri + dir, nil
}

func (s *Artifactory) FileServer() http.Handler {
	// TODO: Since we only extract a subset of files perhaps if the file does not
	// exist we should download the vsix and extract the requested file as a
	// fallback.  Obviously this seems like quite a bit of overhead so we would
	// then emit a warning so we can notice that VS Code has added new asset types
	// that we should be extracting to avoid that overhead.  Other solutions could
	// be implemented though like extracting the VSIX to disk locally and only
	// going to Artifactory for the VSIX when it is missing on disk (basically
	// using the disk as a cache).
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		reader, code, err := s.read(r.Context(), r.URL.Path)
		if err != nil {
			http.Error(rw, err.Error(), code)
			return
		}
		defer reader.Close()
		rw.WriteHeader(http.StatusOK)
		_, _ = io.Copy(rw, reader)
	})
}

func (s *Artifactory) Manifest(ctx context.Context, publisher, name, version string) (*VSIXManifest, error) {
	reader, _, err := s.read(ctx, path.Join(publisher, name, version, "extension.vsixmanifest"))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// If the manifest is returned with an error that means it exists but is
	// invalid.  We will still return it as a best-effort.
	manifest, err := parseVSIXManifest(reader)
	if manifest == nil && err != nil {
		return nil, err
	} else if err != nil {
		s.logger.Error(ctx, "Extension has invalid manifest", slog.Error(err))
	}

	manifest.Assets.Asset = append(manifest.Assets.Asset, VSIXAsset{
		Type:        VSIXAssetType,
		Path:        fmt.Sprintf("%s.vsix", ExtensionID(manifest)),
		Addressable: "true",
	})

	return manifest, nil
}

func (s *Artifactory) RemoveExtension(ctx context.Context, publisher, name, version string) error {
	_, err := s.delete(ctx, path.Join(publisher, name, version))
	return err
}

func (s *Artifactory) WalkExtensions(ctx context.Context, fn func(manifest *VSIXManifest, versions []string) error) error {
	publishers, err := s.getDirNames(ctx, "/")
	if err != nil {
		s.logger.Error(ctx, "Error reading publisher", slog.Error(err))
	}
	for _, publisher := range publishers {
		ctx := slog.With(ctx, slog.F("publisher", publisher))
		extensions, err := s.getDirNames(ctx, publisher)
		if err != nil {
			s.logger.Error(ctx, "Error reading extensions", slog.Error(err))
		}
		for _, extension := range extensions {
			ctx := slog.With(ctx, slog.F("extension", extension))
			versions, err := s.Versions(ctx, publisher, extension)
			if err != nil {
				s.logger.Error(ctx, "Error reading versions", slog.Error(err))
			}
			if len(versions) == 0 {
				continue
			}

			// The manifest from the latest version is used for filtering.
			manifest, err := s.Manifest(ctx, publisher, extension, versions[0])
			if err != nil {
				s.logger.Error(ctx, "Unable to read extension manifest", slog.Error(err))
				continue
			}

			if err = fn(manifest, versions); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Artifactory) Versions(ctx context.Context, publisher, name string) ([]string, error) {
	versions, err := s.getDirNames(ctx, path.Join(publisher, name))
	// Return anything we did get even if there was an error.
	sort.Sort(sort.Reverse(semver.ByVersion(versions)))
	return versions, err
}

// getDirNames get the names of directories in the provided directory.  If an
// error is occured it will be returned along with any directories that were
// able to be read.
func (s *Artifactory) getDirNames(ctx context.Context, dir string) ([]string, error) {
	files, _, err := s.list(ctx, dir)
	names := []string{}
	for _, file := range files {
		if file.Folder {
			// The files come with leading slashes so clean them up.
			names = append(names, strings.TrimLeft(path.Clean(file.URI), "/"))
		}
	}
	return names, err
}
