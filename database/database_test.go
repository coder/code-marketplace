package database_test

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/code-marketplace/database"
)

func scaffold(t *testing.T, dir string) {
	exts := []struct {
		Publisher   string
		Name        string
		Tags        string
		Files       []database.VSIXAsset
		Properties  []database.VSIXProperty
		Description string
		Categories  string
		Versions    []string
	}{
		{
			Publisher:   "foo",
			Name:        "zany",
			Description: "foo bar baz qux",
			Tags:        "tag1",
			Categories:  "category1",
			Files: []database.VSIXAsset{
				{Type: "Microsoft.VisualStudio.Services.Icons.Default", Path: "icon.png", Addressable: "true"},
				{Type: "Unaddressable", Path: "unaddressable.ext", Addressable: "false"},
			},
			Properties: []database.VSIXProperty{{ID: "property1", Value: "value1"}},
			Versions:   []string{"1.0.0", "2.0.0", "3.0.0", "1.5.2", "2.2.2"},
		},
		{
			Publisher:   "foo",
			Name:        "buz",
			Description: "quix baz bar buz sitting",
			Tags:        "tag2",
			Categories:  "category2",
			Versions:    []string{"version1"},
		},
		{
			Publisher:   "bar",
			Name:        "squigly",
			Description: "squigly foo and more foo bar baz",
			Tags:        "tag1,tag2",
			Categories:  "category1,category2",
			Versions:    []string{"version1", "version2"},
		},
		{
			Publisher:   "fred",
			Name:        "thud",
			Description: "frobbles the frobnozzle",
			Tags:        "tag3,tag4,tag5",
			Categories:  "category1",
			Versions:    []string{"version1", "version2"},
		},
		{
			Publisher:   "qqqqqqqqqqq",
			Name:        "qqqqq",
			Description: "qqqqqqqqqqqqqqqqqqq",
			Tags:        "qq,qqq,qqqq",
			Categories:  "q",
			Versions:    []string{"qqq", "q"},
		},
	}
	for _, ext := range exts {
		for _, ver := range ext.Versions {
			dir := filepath.Join(dir, ext.Publisher, ext.Name, ver)
			err := os.MkdirAll(dir, 0o755)
			require.NoError(t, err)

			manifest, err := xml.Marshal(database.VSIXManifest{
				Metadata: database.VSIXMetadata{
					Identity: database.VSIXIdentity{
						ID:        ext.Name,
						Version:   ver,
						Publisher: ext.Publisher,
					},
					Properties: database.VSIXProperties{
						Property: ext.Properties,
					},
					Description: ext.Description,
					Tags:        ext.Tags,
					Categories:  ext.Categories,
				},
				Assets: database.VSIXAssets{
					Asset: ext.Files,
				},
			})
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(dir, "extension.vsixmanifest"), manifest, 0o644)
			require.NoError(t, err)
		}
	}
}

func TestGetExtensionAssetPath(t *testing.T) {
	t.Parallel()

	base := "test://cdr.dev/base"
	baseURL, err := url.Parse(base)
	require.NoError(t, err)

	extdir := t.TempDir()
	scaffold(t, extdir)

	db := database.NoDB{
		ExtDir: extdir,
		Logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
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

	t.Run("GetExtensionAsset", func(t *testing.T) {
		path, err := db.GetExtensionAssetPath(context.Background(), &database.Asset{
			Publisher: "foo",
			Extension: "zany",
			Type:      database.ExtensionAssetType,
			Version:   "1.0.0",
		}, *baseURL)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("%s/files/foo/zany/1.0.0/foo.zany-1.0.0.vsix", base), path)
	})
}

type checkFunc func(t *testing.T, ext *database.Extension)

func TestGetExtensions(t *testing.T) {
	t.Parallel()

	base := "test://cdr.dev/base"
	extdir := t.TempDir()
	scaffold(t, extdir)

	cases := []struct {
		Name       string
		ExtDir     string
		Filter     database.Filter
		Flags      database.Flag
		Extensions []string
		Count      int
		CheckFunc  checkFunc
	}{
		{
			Name:       "BadDir",
			ExtDir:     extdir + "-invalid",
			Filter:     database.Filter{},
			Extensions: []string{},
			Count:      0,
		},
		{
			Name:       "NoCriteria",
			ExtDir:     extdir,
			Filter:     database.Filter{},
			Extensions: []string{},
			Count:      0,
		},
		{
			Name:   "Target",
			ExtDir: extdir,
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
			Name:   "TargetAndExcludeUnpublished",
			ExtDir: extdir,
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
			Name:   "FirstPage",
			ExtDir: extdir,
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
			Name:   "SecondPage",
			ExtDir: extdir,
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
			Name:   "StartOutOfBounds",
			ExtDir: extdir,
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
			Name:   "EndOutOfBounds",
			ExtDir: extdir,
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
			Name:   "ByTag",
			ExtDir: extdir,
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
			Name:   "ByID",
			ExtDir: extdir,
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
			Name:   "ByCategory",
			ExtDir: extdir,
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
			Name:   "ByName",
			ExtDir: extdir,
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
			Name:   "ByNames",
			ExtDir: extdir,
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
			Name:   "MultipleCriterion",
			ExtDir: extdir,
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
			Name:   "ByFeatured",
			ExtDir: extdir,
			Filter: database.Filter{
				Criteria: []database.Criteria{{
					Type: database.Featured,
				}},
			},
			Extensions: []string{},
			Count:      0,
		},
		{
			Name:   "BySearchTextRelevance",
			ExtDir: extdir,
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
			Name:   "BySearchTextMultiple",
			ExtDir: extdir,
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
			Name:   "BySearchTextMany",
			ExtDir: extdir,
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
			Name:   "BySearchTextNoMatch",
			ExtDir: extdir,
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
			Name:   "BySearchTextOneMatch",
			ExtDir: extdir,
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
			Name:   "ByPublisher",
			ExtDir: extdir,
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
			Name:   "TargetSortAscending",
			ExtDir: extdir,
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
			Name:   "TargetSortPublisher",
			ExtDir: extdir,
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
			Name:   "IncludeVersions",
			ExtDir: extdir,
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
			Name:   "IncludeFiles",
			ExtDir: extdir,
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
					require.Len(t, version.Files, 2, "files")
					require.Equal(t, fmt.Sprintf("%s/files/foo/zany/%s/foo.zany-%s.vsix", base, version.Version, version.Version), version.Files[0].Source)
					require.Equal(t, fmt.Sprintf("%s/files/foo/zany/%s/icon.png", base, version.Version), version.Files[1].Source)
					require.Empty(t, version.Properties, "properties")
				}
			},
		},
		{
			Name:   "IncludeAssetURI",
			ExtDir: extdir,
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
			Name:   "IncludeCategoriesAndTags",
			ExtDir: extdir,
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
			Name:   "IncludeVersionProperties",
			ExtDir: extdir,
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
					require.Len(t, version.Properties, 1, "properties")
				}
			},
		},
		{
			Name:   "IncludeLatestVersionOnly",
			ExtDir: extdir,
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
			Name:   "IncludeAll",
			ExtDir: extdir,
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
					require.Len(t, version.Files, 2, "files")
					require.Equal(t, fmt.Sprintf("%s/files/foo/zany/%s/foo.zany-%s.vsix", base, version.Version, version.Version), version.Files[0].Source)
					require.Equal(t, fmt.Sprintf("%s/files/foo/zany/%s/icon.png", base, version.Version), version.Files[1].Source)
					require.Len(t, version.Properties, 1, "properties")
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
				ExtDir: extdir,
				Logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
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
