package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
	"golang.org/x/sync/errgroup"
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

type ArtifactoryList struct {
	Files []ArtifactoryFile `json:"files"`
}

// Artifactory implements Storage.  It stores extensions remotely through
// Artifactory by both copying the VSIX and extracting said VSIX to a tree
// structure in the form of publisher/extension/version to easily serve
// individual assets via HTTP.
type Artifactory struct {
	listCache       *[]ArtifactoryFile
	listDuration    time.Duration
	listExpiration  time.Time
	listMutex       sync.Mutex
	logger          slog.Logger
	manifests       sync.Map
	manifestMutexes sync.Map
	repo            string
	token           string
	uri             string
}

func NewArtifactoryStorage(ctx context.Context, uri, repo string, logger slog.Logger) (*Artifactory, error) {
	token := os.Getenv(ArtifactoryTokenEnvKey)
	if token == "" {
		return nil, xerrors.Errorf("the %s environment variable must be set", ArtifactoryTokenEnvKey)
	}

	if !strings.HasSuffix(uri, "/") {
		uri = uri + "/"
	}

	s := &Artifactory{
		// TODO: Should probably make the duration configurable?  And/or have a
		// command for ejecting the cache?  Maybe automatically when you run the add
		// or remove commands.
		listDuration: time.Minute,
		logger:       logger,
		repo:         path.Clean(repo),
		token:        token,
		uri:          uri,
	}

	s.logger.Info(ctx, "Seeding manifest cache...")

	start := time.Now()
	count := 0
	var eg errgroup.Group
	err := s.WalkExtensions(ctx, func(manifest *VSIXManifest, versions []string) error {
		for _, ver := range versions {
			count++
			ver := ver
			eg.Go(func() error {
				identity := manifest.Metadata.Identity
				_, err := s.Manifest(ctx, identity.Publisher, identity.ID, ver)
				if err != nil && !errors.Is(err, context.Canceled) {
					return err
				} else if err != nil {
					id := ExtensionID(identity.Publisher, identity.ID, ver)
					s.logger.Error(ctx, "Unable to read extension manifest", slog.Error(err), slog.F("id", id))
				}
				return nil
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = eg.Wait()
	if err != nil {
		return nil, err
	}

	s.logger.Info(ctx, "Seeded manifest cache",
		slog.F("count", count),
		slog.F("took", time.Since(start)))

	return s, nil
}

// request makes a request against Artifactory and returns the response.  If
// there is an error it reads the response first to get any error messages.  The
// code is returned so it can be relayed when proxying file requests.  404s are
// turned into os.ErrNotExist errors.
func (s *Artifactory) request(ctx context.Context, method, endpoint string, r io.Reader) (*http.Response, int, error) {
	start := time.Now()
	ctx = slog.With(ctx, slog.F("path", endpoint), slog.F("method", method))
	defer func() {
		s.logger.Debug(ctx, "artifactory request", slog.F("took", time.Since(start)))
	}()
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

func (s *Artifactory) list(ctx context.Context, endpoint string, depth int) ([]ArtifactoryFile, int, error) {
	query := fmt.Sprintf("?list&deep=1&depth=%d&listFolders=1", depth)
	resp, code, err := s.request(ctx, http.MethodGet, path.Join("api/storage", s.repo, endpoint)+query, nil)
	if err != nil {
		return nil, code, err
	}
	start := time.Now()
	defer func() {
		s.logger.Debug(ctx, "parse list response", slog.F("took", time.Since(start)))
	}()
	defer resp.Body.Close()
	var ar ArtifactoryList
	err = json.NewDecoder(resp.Body).Decode(&ar)
	if err != nil {
		return nil, code, err
	}
	return ar.Files, code, nil
}

func (s *Artifactory) read(ctx context.Context, endpoint string) (io.ReadCloser, int, error) {
	resp, code, err := s.request(ctx, http.MethodGet, path.Join(s.repo, endpoint), nil)
	if err != nil {
		return nil, code, err
	}
	return resp.Body, code, err
}

func (s *Artifactory) delete(ctx context.Context, endpoint string) (int, error) {
	resp, code, err := s.request(ctx, http.MethodDelete, path.Join(s.repo, endpoint), nil)
	if err != nil {
		return code, err
	}
	defer resp.Body.Close()
	return code, nil
}

func (s *Artifactory) upload(ctx context.Context, endpoint string, r io.Reader) (int, error) {
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
	vsixName := fmt.Sprintf("%s.vsix", ExtensionIDFromManifest(manifest))
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
	// These queries are so slow it seems worth the extra memory to cache the
	// manifests for future use.
	// TODO: Remove manifests that are no longer found in the list to prevent
	// indefinitely caching manifests belonging to extensions that have since been
	// removed or dump the cache periodically.
	id := ExtensionID(publisher, name, version)
	rawMutex, _ := s.manifestMutexes.LoadOrStore(id, &sync.Mutex{})
	mutex := rawMutex.(*sync.Mutex)
	mutex.Lock()
	defer mutex.Unlock()

	rawManifest, ok := s.manifests.Load(id)
	if ok {
		return rawManifest.(*VSIXManifest), nil
	}

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
		Path:        fmt.Sprintf("%s.vsix", ExtensionIDFromManifest(manifest)),
		Addressable: "true",
	})

	rawManifest, _ = s.manifests.LoadOrStore(id, manifest)
	return rawManifest.(*VSIXManifest), nil
}

func (s *Artifactory) RemoveExtension(ctx context.Context, publisher, name, version string) error {
	_, err := s.delete(ctx, path.Join(publisher, name, version))
	return err
}

type extension struct {
	manifest  *VSIXManifest
	name      string
	publisher string
	versions  []string
}

func (s *Artifactory) listWithCache(ctx context.Context) *[]ArtifactoryFile {
	s.listMutex.Lock()
	defer s.listMutex.Unlock()
	if s.listCache == nil || time.Now().After(s.listExpiration) {
		s.listExpiration = time.Now().Add(s.listDuration)
		list, _, err := s.list(ctx, "/", 3)
		if err != nil {
			s.logger.Error(ctx, "Error reading extensions", slog.Error(err))
		}
		s.listCache = &list
	}
	return s.listCache
}

func (s *Artifactory) WalkExtensions(ctx context.Context, fn func(manifest *VSIXManifest, versions []string) error) error {
	// Listing one directory at a time is very slow so get them all at once.  If
	// we already fetched it recently just use that since getting them all at once
	// is also pretty slow (on the parsing end).
	files := s.listWithCache(ctx)
	extensions := make(map[string]*extension)
	for _, file := range *files {
		// There should only be folders up to this depth but check just in case.
		if !file.Folder {
			continue
		}
		parts := strings.Split(file.URI, "/")
		// We will get all directories up to the requested depth so for example
		// /publisher, /publisher/extension, and /publisher/extension/version.
		if len(parts) == 4 {
			id := fmt.Sprintf("%s.%s", parts[1], parts[2])
			e, ok := extensions[id]
			if ok {
				e.versions = append(e.versions, parts[3])
			} else {
				extensions[id] = &extension{
					name:      parts[2],
					publisher: parts[1],
					versions:  []string{parts[3]},
				}
			}
		}
	}
	// The manifest from the latest version is used for filtering.  Fetching
	// manifests is very slow so parallelize them.  We could call `fn` in this
	// loop but it would require that `fn` be thread-safe.  For now I opted to
	// fetch all the manifests then run the callback in a separate loop.
	var eg errgroup.Group
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, ext := range extensions {
		ext := ext
		sort.Sort(sort.Reverse(semver.ByVersion(ext.versions)))
		eg.Go(func() error {
			manifest, err := s.Manifest(ctx, ext.publisher, ext.name, ext.versions[0])
			if err != nil && errors.Is(err, context.Canceled) {
				return err
			} else if err != nil {
				id := ExtensionID(ext.publisher, ext.name, ext.versions[0])
				s.logger.Error(ctx, "Unable to read extension manifest", slog.Error(err), slog.F("id", id))
			} else {
				ext.manifest = manifest
			}
			return nil
		})
	}
	err := eg.Wait()
	if err != nil {
		return err
	}
	for _, ext := range extensions {
		if err = fn(ext.manifest, ext.versions); err != nil {
			return err
		}
	}
	return nil
}

func (s *Artifactory) Versions(ctx context.Context, publisher, name string) ([]string, error) {
	files, _, err := s.list(ctx, path.Join(publisher, name), 1)
	if err != nil {
		return nil, err
	}
	versions := []string{}
	for _, file := range files {
		// There should only be directories but check just in case.
		if file.Folder {
			// The files come with leading slashes so remove them.
			versions = append(versions, strings.TrimLeft(file.URI, "/"))
		}
	}
	sort.Sort(sort.Reverse(semver.ByVersion(versions)))
	return versions, nil
}
