package testutil

import (
	"github.com/coder/code-marketplace/storage"
)

type Extension struct {
	Publisher     string
	Name          string
	Tags          string
	Files         []storage.VSIXAsset
	Properties    []storage.VSIXProperty
	Description   string
	Categories    string
	Versions      []string
	LatestVersion string
	Dependencies  []string
	Pack          []string
}

var Extensions = []Extension{
	{
		Publisher:   "foo",
		Name:        "zany",
		Description: "foo bar baz qux",
		Tags:        "tag1",
		Categories:  "category1",
		Files: []storage.VSIXAsset{
			{Type: "Microsoft.VisualStudio.Services.Icons.Default", Path: "icon.png", Addressable: "true"},
			{Type: "Unaddressable", Path: "unaddressable.ext", Addressable: "false"},
		},
		Properties: []storage.VSIXProperty{
			{
				ID:    "Microsoft.VisualStudio.Code.ExtensionPack",
				Value: "a.b,b.c",
			},
			{
				ID:    "Microsoft.VisualStudio.Code.ExtensionDependencies",
				Value: "d.e",
			},
		},
		Versions:      []string{"1.0.0", "2.0.0", "3.0.0", "1.5.2", "2.2.2"},
		LatestVersion: "3.0.0",
		Dependencies:  []string{"d.e"},
		Pack:          []string{"a.b", "b.c"},
	},
	{
		Publisher:   "foo",
		Name:        "buz",
		Description: "quix baz bar buz sitting",
		Tags:        "tag2",
		Categories:  "category2",
		Properties: []storage.VSIXProperty{
			{
				ID:    "Microsoft.VisualStudio.Code.ExtensionPack",
				Value: "",
			},
			{
				ID:    "Microsoft.VisualStudio.Code.ExtensionDependencies",
				Value: "",
			},
		},
		Versions:      []string{"version1"},
		LatestVersion: "version1",
	},
	{
		Publisher:     "bar",
		Name:          "squigly",
		Description:   "squigly foo and more foo bar baz",
		Tags:          "tag1,tag2",
		Categories:    "category1,category2",
		Versions:      []string{"version1", "version2"},
		LatestVersion: "version2",
	},
	{
		Publisher:     "fred",
		Name:          "thud",
		Description:   "frobbles the frobnozzle",
		Tags:          "tag3,tag4,tag5",
		Categories:    "category1",
		Versions:      []string{"version1", "version2"},
		LatestVersion: "version2",
	},
	{
		Publisher:     "qqqqqqqqqqq",
		Name:          "qqqqq",
		Description:   "qqqqqqqqqqqqqqqqqqq",
		Tags:          "qq,qqq,qqqq",
		Categories:    "q",
		Versions:      []string{"qqq", "q"},
		LatestVersion: "qqq",
	},
}

func ConvertExtensionToManifest(ext Extension, version string) *storage.VSIXManifest {
	return &storage.VSIXManifest{
		Metadata: storage.VSIXMetadata{
			Identity: storage.VSIXIdentity{
				ID:        ext.Name,
				Version:   version,
				Publisher: ext.Publisher,
			},
			Properties: storage.VSIXProperties{
				Property: ext.Properties,
			},
			Description: ext.Description,
			Tags:        ext.Tags,
			Categories:  ext.Categories,
		},
		Assets: storage.VSIXAssets{
			Asset: ext.Files,
		},
	}
}
