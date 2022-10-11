package database

import (
	"context"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/lithammer/fuzzysearch/fuzzy"

	"cdr.dev/slog"

	"github.com/coder/code-marketplace/storage"
)

// NoDB implements Database.  It reads extensions directly off storage then
// filters, sorts, and paginates them.  In other words, the file system is the
// database.
type NoDB struct {
	Storage storage.Storage
	Logger  slog.Logger
}

func (db *NoDB) GetExtensionAssetPath(ctx context.Context, asset *Asset, baseURL url.URL) (string, error) {
	manifest, err := db.Storage.Manifest(ctx, asset.Publisher, asset.Extension, asset.Version)
	if err != nil {
		return "", err
	}

	fileBase := (&url.URL{
		Scheme: baseURL.Scheme,
		Host:   baseURL.Host,
		Path: path.Join(
			baseURL.Path,
			"files",
			asset.Publisher,
			asset.Extension,
			asset.Version),
	}).String()

	for _, a := range manifest.Assets.Asset {
		if a.Addressable == "true" && a.Type == asset.Type {
			return fileBase + "/" + a.Path, nil
		}
	}

	return "", os.ErrNotExist
}

func (db *NoDB) GetExtensions(ctx context.Context, filter Filter, flags Flag, baseURL url.URL) ([]*Extension, int, error) {
	vscodeExts := []*noDBExtension{}

	err := db.Storage.WalkExtensions(ctx, func(manifest *storage.VSIXManifest, versions []string) error {
		vscodeExt := convertManifestToExtension(manifest)
		if matched, distances := getMatches(vscodeExt, filter); matched {
			vscodeExt.versions = versions
			vscodeExt.distances = distances
			vscodeExts = append(vscodeExts, vscodeExt)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	total := len(vscodeExts)
	sortExtensions(vscodeExts, filter)
	vscodeExts = paginateExtensions(vscodeExts, filter)
	db.handleFlags(ctx, vscodeExts, flags, baseURL)

	convertedExts := []*Extension{}
	for _, ext := range vscodeExts {
		convertedExts = append(convertedExts, &Extension{
			ID:               ext.ID,
			Name:             ext.Name,
			DisplayName:      ext.DisplayName,
			ShortDescription: ext.ShortDescription,
			Publisher:        ext.Publisher,
			Versions:         ext.Versions,
			Statistics:       ext.Statistics,
			Tags:             ext.Tags,
			ReleaseDate:      ext.ReleaseDate,
			PublishedDate:    ext.PublishedDate,
			LastUpdated:      ext.LastUpdated,
			Categories:       ext.Categories,
			Flags:            ext.Flags,
		})
	}
	return convertedExts, total, nil
}

func getMatches(extension *noDBExtension, filter Filter) (bool, []int) {
	// Normally we would want to handle ExcludeWithFlags but the only flag that
	// seems usable with it (and the only flag VS Code seems to send) is
	// Unpublished and currently there is no concept of unpublished extensions so
	// there is nothing to do.
	var (
		triedFilter = false
		hasTarget   = false
		distances   = []int{}
	)
	match := func(matches bool) {
		triedFilter = true
		if matches {
			distances = append(distances, 0)
		}
	}
	for _, c := range filter.Criteria {
		switch c.Type {
		case Tag:
			match(containsFold(extension.Tags, c.Value))
		case ExtensionID:
			match(strings.EqualFold(extension.ID, c.Value))
		case Category:
			match(containsFold(extension.Categories, c.Value))
		case ExtensionName:
			// The value here is the fully qualified name `publisher.extension`.
			match(strings.EqualFold(extension.Publisher.PublisherName+"."+extension.Name, c.Value))
		case Target:
			// Unlike the other criteria the target is an AND so if it does not match
			// we can abort early.
			if c.Value != "Microsoft.VisualStudio.Code" {
				return false, nil
			}
			// Otherwise we need to only include the extension if one of the other
			// criteria also matched which we can only know after we have gone through
			// them all since not all criteria are for matching (ExcludeWithFlags).
			hasTarget = true
		case Featured:
			// Currently unsupported; this would require a database.
			match(false)
		case SearchText:
			triedFilter = true
			// REVIEW: Does this even make any sense?
			// REVIEW: Should include categories and tags?
			// Search each token of the input individually.
			tokens := strings.FieldsFunc(c.Value, func(r rune) bool {
				return r == ' ' || r == ',' || r == '.'
			})
			// Publisher is implement as SearchText via `publisher:"name"`.
			searchTokens := []string{}
			for _, token := range tokens {
				parts := strings.SplitN(token, ":", 2)
				if len(parts) == 2 && parts[0] == "publisher" {
					match(strings.EqualFold(extension.Publisher.PublisherName, strings.Trim(parts[1], "\"")))
				} else if token != "" {
					searchTokens = append(searchTokens, token)
				}
			}
			candidates := []string{extension.Name, extension.Publisher.PublisherName, extension.ShortDescription}
			allMatches := fuzzy.Ranks{}
			for _, token := range searchTokens {
				matches := fuzzy.RankFindFold(token, candidates)
				if len(matches) == 0 {
					// If even one token does not match all the matches are invalid.
					allMatches = fuzzy.Ranks{}
					break
				}
				allMatches = append(allMatches, matches...)
			}
			for _, match := range allMatches {
				distances = append(distances, match.Distance)
			}
		}
	}
	if !triedFilter && hasTarget {
		return true, nil
	}
	sort.Ints(distances)
	return len(distances) > 0, distances
}

func sortExtensions(extensions []*noDBExtension, filter Filter) {
	sort.Slice(extensions, func(i, j int) bool {
		less := false
		a := extensions[i]
		b := extensions[j]
	outer:
		switch filter.SortBy {
		// These are not supported because we are not storing this information.
		case LastUpdatedDate:
			fallthrough
		case PublishedDate:
			fallthrough
		case AverageRating:
			fallthrough
		case WeightedRating:
			fallthrough
		case InstallCount:
			fallthrough
		case Title:
			less = a.Name < b.Name
		case PublisherName:
			if a.Publisher.PublisherName < b.Publisher.PublisherName {
				less = true
			} else if a.Publisher.PublisherName == b.Publisher.PublisherName {
				less = a.Name < b.Name
			}
		default: // NoneOrRelevance
			// No idea if this is any good but select the extension with the closest
			// match.  If they both have a match with the same closeness look for the
			// next closest and so on.
			blen := len(b.distances)
			for k := range a.distances { // Iterate in order since these are sorted.
				if k >= blen { // Same closeness so far but a has more matches than b.
					less = true
					break outer
				} else if a.distances[k] < b.distances[k] {
					less = true
					break outer
				} else if a.distances[k] > b.distances[k] {
					break outer
				}
			}
			// Same closeness so far but b has more matches than a.
			if len(a.distances) < blen {
				break outer
			}
			// Same closeness, use name instead.
			less = a.Name < b.Name
		}
		if filter.SortOrder == Ascending {
			return !less
		} else {
			return less
		}
	})
}

func paginateExtensions(exts []*noDBExtension, filter Filter) []*noDBExtension {
	page := filter.PageNumber
	if page <= 0 {
		page = 1
	}
	size := filter.PageSize
	if size <= 0 {
		size = 50
	}
	start := (page - 1) * size
	length := len(exts)
	if start > length {
		start = length
	}
	end := start + size
	if end > length {
		end = length
	}
	return exts[start:end]
}

func (db *NoDB) handleFlags(ctx context.Context, exts []*noDBExtension, flags Flag, baseURL url.URL) {
	for _, ext := range exts {
		// Files, properties, and asset URIs are part of versions so if they are set
		// assume we also want to include versions.
		if flags&IncludeVersions != 0 ||
			flags&IncludeFiles != 0 ||
			flags&IncludeVersionProperties != 0 ||
			flags&IncludeLatestVersionOnly != 0 ||
			flags&IncludeAssetURI != 0 {
			ext.Versions = db.getVersions(ctx, ext, flags, baseURL)
		}

		// TODO: This does not seem to be included in any interfaces so no idea
		// where to put this info if it is requested.
		// flags&IncludeInstallationTargets != 0

		// Categories and tags are already included (for filtering on them) so we
		// need to instead remove them.
		if flags&IncludeCategoryAndTags == 0 {
			ext.Categories = []string{}
			ext.Tags = []string{}
		}

		// Unsupported flags.
		// if flags&IncludeSharedAccounts != 0 {}
		// if flags&ExcludeNonValidated != 0 {}
		// if flags&IncludeStatistics != 0 {}
		// if flags&Unpublished != 0 {}
	}
}

func (db *NoDB) getVersions(ctx context.Context, ext *noDBExtension, flags Flag, baseURL url.URL) []ExtVersion {
	ctx = slog.With(ctx,
		slog.F("publisher", ext.Publisher.PublisherName),
		slog.F("extension", ext.Name))

	versionStrs := ext.versions
	if flags&IncludeLatestVersionOnly != 0 {
		versionStrs = []string{ext.versions[0]}
	}

	versions := []ExtVersion{}
	for _, versionStr := range versionStrs {
		ctx := slog.With(ctx, slog.F("version", versionStr))
		manifest, err := db.Storage.Manifest(ctx, ext.Publisher.PublisherName, ext.Name, versionStr)
		if err != nil {
			db.Logger.Error(ctx, "Unable to parse version manifest", slog.Error(err))
			continue
		}

		version := ExtVersion{
			Version: versionStr,
			// LastUpdated:    time.Now(), // TODO: Use modified time?
			TargetPlatform: manifest.Metadata.Identity.TargetPlatform,
		}

		if flags&IncludeFiles != 0 {
			fileBase := (&url.URL{
				Scheme: baseURL.Scheme,
				Host:   baseURL.Host,
				Path: path.Join(
					baseURL.Path,
					"/files",
					ext.Publisher.PublisherName,
					ext.Name,
					versionStr),
			}).String()
			for _, asset := range manifest.Assets.Asset {
				if asset.Addressable != "true" {
					continue
				}
				version.Files = append(version.Files, ExtFile{
					Type:   asset.Type,
					Source: fileBase + "/" + asset.Path,
				})
			}
		}

		if flags&IncludeVersionProperties != 0 {
			version.Properties = []ExtProperty{}
			for _, prop := range manifest.Metadata.Properties.Property {
				version.Properties = append(version.Properties, ExtProperty{
					Key:   prop.ID,
					Value: prop.Value,
				})
			}
		}

		if flags&IncludeAssetURI != 0 {
			version.AssetURI = (&url.URL{
				Scheme: baseURL.Scheme,
				Host:   baseURL.Host,
				Path: path.Join(
					baseURL.Path,
					"assets",
					ext.Publisher.PublisherName,
					ext.Name,
					versionStr),
			}).String()
			version.FallbackAssetURI = version.AssetURI
		}

		versions = append(versions, version)
	}
	return versions
}

// noDBExtension adds some properties for internally filtering.
type noDBExtension struct {
	Extension
	// Used internally for ranking.  Lower means more relevant.
	distances []int `json:"-"`
	// Used internally to avoid reading and sorting versions twice.
	versions []string `json:"-"`
}

func convertManifestToExtension(manifest *storage.VSIXManifest) *noDBExtension {
	return &noDBExtension{
		Extension: Extension{
			// Normally this is a GUID but in the absence of a database just put
			// together the publisher and extension name since that will be unique.
			ID: manifest.Metadata.Identity.Publisher + "." + manifest.Metadata.Identity.ID,
			// The ID in the manifest is actually the extension name (for example
			// `python`) which vscode-vsce pulls from the package.json's `name`.
			Name:             manifest.Metadata.Identity.ID,
			DisplayName:      manifest.Metadata.DisplayName,
			ShortDescription: manifest.Metadata.Description,
			Publisher: ExtPublisher{
				// Normally this is a GUID but in the absence of a database just put the
				// publisher name since that will be unique.
				PublisherID:   manifest.Metadata.Identity.Publisher,
				PublisherName: manifest.Metadata.Identity.Publisher,
				// There is not actually a separate display name field for publishers.
				DisplayName: manifest.Metadata.Identity.Publisher,
			},
			Tags: strings.Split(manifest.Metadata.Tags, ","),
			// ReleaseDate:   time.Now(), // TODO: Use creation time?
			// PublishedDate: time.Now(), // TODO: Use creation time?
			// LastUpdated:   time.Now(), // TODO: Use modified time?
			Categories: strings.Split(manifest.Metadata.Categories, ","),
			Flags:      manifest.Metadata.GalleryFlags,
		},
	}
}

func containsFold(a []string, b string) bool {
	for _, astr := range a {
		if strings.EqualFold(astr, b) {
			return true
		}
	}
	return false
}
