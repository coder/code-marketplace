package database_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/code-marketplace/database"
	"github.com/coder/code-marketplace/storage"
	"github.com/coder/code-marketplace/testutil"
)

type memoryStorage struct{}

func (s *memoryStorage) AddExtension(ctx context.Context, manifest *storage.VSIXManifest, vsix []byte) (string, error) {
	return "", errors.New("not implemented")
}

func (s *memoryStorage) RemoveExtension(ctx context.Context, publisher, extension, version string) error {
	return errors.New("not implemented")
}

func (s *memoryStorage) FileServer() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		http.Error(rw, "not implemented", http.StatusNotImplemented)
	})
}

func (s *memoryStorage) Versions(ctx context.Context, publisher, name string) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (s *memoryStorage) Manifest(ctx context.Context, publisher, extension, version string) (*storage.VSIXManifest, error) {
	for _, ext := range testutil.Extensions {
		if ext.Publisher == publisher && ext.Name == extension {
			for _, ver := range ext.Versions {
				if ver == version {
					return testutil.ConvertExtensionToManifest(ext, ver), nil
				}
			}
			break
		}
	}
	return nil, os.ErrNotExist
}

func (s *memoryStorage) WalkExtensions(ctx context.Context, fn func(manifest *storage.VSIXManifest, versions []string) error) error {
	for _, ext := range testutil.Extensions {
		if err := fn(testutil.ConvertExtensionToManifest(ext, ext.Versions[0]), ext.Versions); err != nil {
			return nil
		}
	}
	return nil
}

func TestGetExtensionAssetPath(t *testing.T) {
	t.Parallel()

	base := "test://cdr.dev/base"
	baseURL, err := url.Parse(base)
	require.NoError(t, err)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	db := database.NoDB{
		Storage: &memoryStorage{},
		Logger:  logger,
	}

	t.Run("NoExtension", func(t *testing.T) {
		_, err := db.GetExtensionAssetPath(context.Background(), &database.Asset{
			Publisher: "publisher",
			Extension: "extension",
			Type:      "type",
			Version:   "version",
		}, *baseURL)
		require.Error(t, err)
	})

	t.Run("NoAsset", func(t *testing.T) {
		_, err := db.GetExtensionAssetPath(context.Background(), &database.Asset{
			Publisher: "foo",
			Extension: "zany",
			Type:      "nope",
			Version:   "1.0.0",
		}, *baseURL)
		require.Error(t, err)
	})

	t.Run("UnaddressableAsset", func(t *testing.T) {
		_, err := db.GetExtensionAssetPath(context.Background(), &database.Asset{
			Publisher: "foo",
			Extension: "zany",
			Type:      "Unaddressable",
			Version:   "1.0.0",
		}, *baseURL)
		require.Error(t, err)
	})

	t.Run("GetAsset", func(t *testing.T) {
		path, err := db.GetExtensionAssetPath(context.Background(), &database.Asset{
			Publisher: "foo",
			Extension: "zany",
			Type:      "Microsoft.VisualStudio.Services.Icons.Default",
			Version:   "1.0.0",
		}, *baseURL)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("%s/files/foo/zany/1.0.0/icon.png", base), path)
	})
}

type checkFunc func(t *testing.T, ext *database.Extension)

func TestGetExtensions(t *testing.T) {
	t.Parallel()

	base := "test://cdr.dev/base"
	cases := []struct {
		Name       string
		WalkError  bool
		ExtDir     string
		Filter     database.Filter
		Flags      database.Flag
		Extensions []string
		Count      int
		CheckFunc  checkFunc
	}{
		{
			Name:       "BadDir",
			WalkError:  true,
			Filter:     database.Filter{},
			Extensions: []string{},
			Count:      0,
		},
		{
			Name:       "NoCriteria",
			Filter:     database.Filter{},
			Extensions: []string{},
			Count:      0,
		},
		{
			Name: "Target",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.Target,
					Value: "Microsoft.VisualStudio.Code",
				}},
			},
			Extensions: []string{"foo.buz", "qqqqqqqqqqq.qqqqq", "bar.squigly", "fred.thud", "foo.zany"},
			Count:      5,
		},
		{
			Name: "TargetAndExcludeUnpublished",
			Filter: database.Filter{
				Criteria: []database.Criteria{
					{
						Type:  database.Target,
						Value: "Microsoft.VisualStudio.Code",
					},
					{
						Type:  database.ExcludeWithFlags,
						Value: "4096",
					},
				},
			},
			Extensions: []string{"foo.buz", "qqqqqqqqqqq.qqqqq", "bar.squigly", "fred.thud", "foo.zany"},
			Count:      5,
		},
		{
			Name: "FirstPage",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.Target,
					Value: "Microsoft.VisualStudio.Code",
				}},
				PageNumber: 1,
				PageSize:   1,
			},
			Extensions: []string{"foo.buz"},
			Count:      5,
		},
		{
			Name: "SecondPage",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.Target,
					Value: "Microsoft.VisualStudio.Code",
				}},
				PageNumber: 2,
				PageSize:   1,
			},
			Extensions: []string{"qqqqqqqqqqq.qqqqq"},
			Count:      5,
		},
		{
			Name: "StartOutOfBounds",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.Target,
					Value: "Microsoft.VisualStudio.Code",
				}},
				PageNumber: 10,
				PageSize:   3,
			},
			Extensions: []string{},
			Count:      5,
		},
		{
			Name: "EndOutOfBounds",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.Target,
					Value: "Microsoft.VisualStudio.Code",
				}},
				PageNumber: 2,
				PageSize:   4,
			},
			Extensions: []string{"foo.zany"},
			Count:      5,
		},
		{
			Name: "ByTag",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.Tag,
					Value: "tag1",
				}},
			},
			Extensions: []string{"bar.squigly", "foo.zany"},
			Count:      2,
		},
		{
			Name: "ByID",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type: database.ExtensionID,
					// Normally this is a GUID but we are using `publisher.extension`.
					Value: "foo.zany",
				}},
			},
			Extensions: []string{"foo.zany"},
			Count:      1,
		},
		{
			Name: "ByCategory",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.Category,
					Value: "CaTeGoRy2",
				}},
			},
			Extensions: []string{"foo.buz", "bar.squigly"},
			Count:      2,
		},
		{
			Name: "ByName",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.ExtensionName,
					Value: "fOo.zAny",
				}},
			},
			Extensions: []string{"foo.zany"},
			Count:      1,
		},
		{
			Name: "ByNames",
			Filter: database.Filter{
				Criteria: []database.Criteria{
					{
						Type:  database.ExtensionName,
						Value: "foo.zany",
					},
					{
						Type:  database.ExtensionName,
						Value: "fred.thud",
					},
				},
				SortOrder: database.Ascending,
			},
			Extensions: []string{"foo.zany", "fred.thud"},
			Count:      2,
		},
		{
			Name: "MultipleCriterion",
			Filter: database.Filter{
				Criteria: []database.Criteria{
					{
						Type:  database.ExtensionName,
						Value: "foo.zany",
					},
					{
						Type:  database.Target,
						Value: "Microsoft.VisualStudio.Code",
					},
					{
						Type:  database.ExcludeWithFlags,
						Value: "4096",
					},
				},
			},
			Extensions: []string{"foo.zany"},
			Count:      1,
		},
		{
			// Not implemented.
			Name: "ByFeatured",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type: database.Featured,
				}},
			},
			Extensions: []string{},
			Count:      0,
		},
		{
			Name: "BySearchTextRelevance",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.SearchText,
					Value: "qux",
				}},
			},
			// foo.buz matches earlier but foo.zany matches more precisely.
			Extensions: []string{"foo.zany", "foo.buz"},
			Count:      2,
		},
		{
			Name: "BySearchTextMultiple",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.SearchText,
					Value: "foo",
				}},
			},
			// These all match foo at least once but with different accuracies.
			Extensions: []string{"foo.zany", "foo.buz", "fred.thud", "bar.squigly"},
			Count:      4,
		},
		{
			Name: "BySearchTextMany",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.SearchText,
					Value: "foo bar baz qux zany",
				}},
			},
			// Only one extension has all these keywords.
			Extensions: []string{"foo.zany"},
			Count:      1,
		},
		{
			Name: "BySearchTextNoMatch",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.SearchText,
					Value: "kitten",
				}},
			},
			Extensions: []string{},
			Count:      0,
		},
		{
			Name: "BySearchTextOneMatch",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.SearchText,
					Value: "squigly",
				}},
			},
			Extensions: []string{"bar.squigly"},
			Count:      1,
		},
		{
			Name: "ByPublisher",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.SearchText,
					Value: "publisher:\"foo\"",
				}},
			},
			Extensions: []string{"foo.buz", "foo.zany"},
			Count:      2,
		},
		{
			Name: "TargetSortAscending",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.Target,
					Value: "Microsoft.VisualStudio.Code",
				}},
				PageNumber: 1,
				PageSize:   50,
				SortOrder:  database.Ascending,
			},
			Extensions: []string{"foo.zany", "fred.thud", "bar.squigly", "qqqqqqqqqqq.qqqqq", "foo.buz"},
			Count:      5,
		},
		{
			Name: "TargetSortPublisher",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.Target,
					Value: "Microsoft.VisualStudio.Code",
				}},
				PageNumber: 1,
				PageSize:   50,
				SortBy:     database.PublisherName,
			},
			Extensions: []string{"bar.squigly", "foo.buz", "foo.zany", "fred.thud", "qqqqqqqqqqq.qqqqq"},
			Count:      5,
		},
		{
			Name: "IncludeVersions",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.ExtensionID,
					Value: "foo.zany",
				}},
			},
			Flags: database.IncludeVersions,
			Count: 1,
			CheckFunc: func(t *testing.T, ext *database.Extension) {
				require.Empty(t, ext.Categories, "categories")
				require.Empty(t, ext.Tags, "tags")
				require.Len(t, ext.Versions, 5, "versions")
				for _, version := range ext.Versions {
					require.Empty(t, version.Files, "files")
					require.Empty(t, version.Properties, "properties")
				}
			},
		},
		{
			Name: "IncludeFiles",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.ExtensionID,
					Value: "foo.zany",
				}},
			},
			Flags: database.IncludeFiles,
			Count: 1,
			CheckFunc: func(t *testing.T, ext *database.Extension) {
				require.Empty(t, ext.Categories, "categories")
				require.Empty(t, ext.Tags, "tags")
				require.Len(t, ext.Versions, 5, "versions")
				for _, version := range ext.Versions {
					// Should ignore non-addressable files.
					require.Len(t, version.Files, 1, "files")
					require.Equal(t, fmt.Sprintf("%s/files/foo/zany/%s/icon.png", base, version.Version), version.Files[0].Source)
					require.Empty(t, version.Properties, "properties")
				}
			},
		},
		{
			Name: "IncludeAssetURI",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.ExtensionID,
					Value: "foo.zany",
				}},
			},
			Flags: database.IncludeAssetURI,
			Count: 1,
			CheckFunc: func(t *testing.T, ext *database.Extension) {
				require.Empty(t, ext.Categories, "categories")
				require.Empty(t, ext.Tags, "tags")
				require.Len(t, ext.Versions, 5, "versions")
				for _, version := range ext.Versions {
					require.Empty(t, version.Files, "files")
					require.Empty(t, version.Properties, "properties")
					require.Equal(t, fmt.Sprintf("%s/assets/foo/zany/%s", base, version.Version), version.AssetURI)
					require.Equal(t, version.AssetURI, version.FallbackAssetURI)
				}
			},
		},
		{
			Name: "IncludeCategoriesAndTags",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.ExtensionID,
					Value: "foo.zany",
				}},
			},
			Flags: database.IncludeCategoryAndTags,
			Count: 1,
			CheckFunc: func(t *testing.T, ext *database.Extension) {
				require.Len(t, ext.Categories, 1, "categories")
				require.Len(t, ext.Tags, 1, "tags")
				require.Empty(t, ext.Versions, "versions")
			},
		},
		{
			Name: "IncludeVersionProperties",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.ExtensionID,
					Value: "foo.zany",
				}},
			},
			Flags: database.IncludeVersionProperties,
			Count: 1,
			CheckFunc: func(t *testing.T, ext *database.Extension) {
				require.Empty(t, ext.Categories, "categories")
				require.Empty(t, ext.Tags, "tags")
				require.Len(t, ext.Versions, 5, "versions")
				for _, version := range ext.Versions {
					require.Empty(t, version.Files, "files")
					require.Len(t, version.Properties, 2, "properties")
				}
			},
		},
		{
			Name: "IncludeLatestVersionOnly",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.ExtensionID,
					Value: "foo.zany",
				}},
			},
			Flags: database.IncludeLatestVersionOnly,
			Count: 1,
			CheckFunc: func(t *testing.T, ext *database.Extension) {
				require.Empty(t, ext.Categories, "categories")
				require.Empty(t, ext.Tags, "tags")
				require.Len(t, ext.Versions, 1, "versions")
				for _, version := range ext.Versions {
					require.Empty(t, version.Files, "files")
					require.Empty(t, version.Properties, "properties")
				}
			},
		},
		{
			Name: "IncludeAll",
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type:  database.ExtensionID,
					Value: "foo.zany",
				}},
			},
			Flags: database.IncludeVersions | database.IncludeFiles | database.IncludeCategoryAndTags | database.IncludeVersionProperties | database.IncludeAssetURI,
			Count: 1,
			CheckFunc: func(t *testing.T, ext *database.Extension) {
				require.Len(t, ext.Categories, 1, "categories")
				require.Len(t, ext.Tags, 1, "tags")
				require.Len(t, ext.Versions, 5, "versions")
				for _, version := range ext.Versions {
					require.Len(t, version.Files, 1, "files")
					require.Equal(t, fmt.Sprintf("%s/files/foo/zany/%s/icon.png", base, version.Version), version.Files[0].Source)
					require.Len(t, version.Properties, 2, "properties")
					require.Equal(t, version.AssetURI, version.FallbackAssetURI)
				}
			},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			db := database.NoDB{
				Storage: &memoryStorage{},
				Logger:  slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
			}
			baseURL, err := url.Parse(base)
			require.NoError(t, err)
			exts, count, err := db.GetExtensions(context.Background(), c.Filter, c.Flags, *baseURL)
			require.NoError(t, err)
			require.Equal(t, c.Count, count)

			if len(c.Extensions) > 0 {
				extids := []string{}
				for _, ext := range exts {
					require.Empty(t, ext.Categories, "categories")
					require.Empty(t, ext.Tags, "tags")
					require.Empty(t, ext.Versions, "versions")
					extids = append(extids, ext.ID)
				}
				require.Equal(t, c.Extensions, extids)
			} else {
				for _, ext := range exts {
					c.CheckFunc(t, ext)
				}
			}
		})
	}
}
