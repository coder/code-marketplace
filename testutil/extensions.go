package testutil

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/require"

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
	Versions      []storage.Version
	LatestVersion string
	Dependencies  []string
	Pack          []string
}

func (e Extension) Copy() Extension {
	var n Extension
	data, _ := json.Marshal(e)
	_ = json.Unmarshal(data, &n)
	return n
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
		Versions: []storage.Version{
			{Version: "1.0.0"},
			{Version: "1.0.0", TargetPlatform: storage.PlatformWin32X64},
			{Version: "2.0.0"},
			{Version: "3.0.0"},
			{Version: "3.0.0", TargetPlatform: storage.PlatformLinuxX64},
			{Version: "3.0.0", TargetPlatform: storage.PlatformLinuxArm64},
			{Version: "3.0.0", TargetPlatform: storage.PlatformWin32X64},
			{Version: "3.0.0", TargetPlatform: storage.PlatformAlpineX64},
			{Version: "3.0.0", TargetPlatform: storage.PlatformDarwinX64},
			{Version: "1.5.2"},
			{Version: "2.2.2"},
		},
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
		Versions:      []storage.Version{{Version: "version1"}},
		LatestVersion: "version1",
	},
	{
		Publisher:     "bar",
		Name:          "squigly",
		Description:   "squigly foo and more foo bar baz",
		Tags:          "tag1,tag2",
		Categories:    "category1,category2",
		Versions:      []storage.Version{{Version: "version1"}, {Version: "version2"}},
		LatestVersion: "version2",
	},
	{
		Publisher:     "fred",
		Name:          "thud",
		Description:   "frobbles the frobnozzle",
		Tags:          "tag3,tag4,tag5",
		Categories:    "category1",
		Versions:      []storage.Version{{Version: "version1"}, {Version: "version2"}},
		LatestVersion: "version2",
	},
	{
		Publisher:     "qqqqqqqqqqq",
		Name:          "qqqqq",
		Description:   "qqqqqqqqqqqqqqqqqqq",
		Tags:          "qq,qqq,qqqq",
		Categories:    "q",
		Versions:      []storage.Version{{Version: "qqq"}, {Version: "q"}},
		LatestVersion: "qqq",
	},
}

func ConvertExtensionToManifest(ext Extension, version storage.Version) *storage.VSIXManifest {
	ext = ext.Copy()
	return &storage.VSIXManifest{
		Metadata: storage.VSIXMetadata{
			Identity: storage.VSIXIdentity{
				ID:             ext.Name,
				Version:        version.Version,
				Publisher:      ext.Publisher,
				TargetPlatform: version.TargetPlatform,
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

func ConvertExtensionToManifestBytes(t *testing.T, ext Extension, version storage.Version) []byte {
	manifestBytes, err := xml.Marshal(ConvertExtensionToManifest(ext, version))
	require.NoError(t, err)
	return manifestBytes
}

type file struct {
	name string
	body []byte
}

// createVSIX returns the bytes for a VSIX file containing the provided raw
// manifest and package.json bytes (if not nil) and an icon.
func CreateVSIX(t *testing.T, manifestBytes []byte, packageJSONBytes []byte) []byte {
	files := []file{{"icon.png", []byte("fake icon")}}
	if manifestBytes != nil {
		files = append(files, file{"extension.vsixmanifest", manifestBytes})
	}
	if packageJSONBytes != nil {
		files = append(files, file{"extension/package.json", packageJSONBytes})
	}
	buf := bytes.NewBuffer(nil)
	zw := zip.NewWriter(buf)
	for _, file := range files {
		fw, err := zw.Create(file.name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(file.body))
		require.NoError(t, err)
	}
	err := zw.Close()
	require.NoError(t, err)
	return buf.Bytes()
}

// CreateVSIXFromManifest returns the bytes for a VSIX file containing the
// provided manifest and an icon.
func CreateVSIXFromManifest(t *testing.T, manifest *storage.VSIXManifest) []byte {
	manifestBytes, err := xml.Marshal(manifest)
	require.NoError(t, err)
	return CreateVSIX(t, manifestBytes, nil)
}

func CreateVSIXFromPackageJSON(t *testing.T, packageJSON *storage.VSIXPackageJSON) []byte {
	packageJSONBytes, err := json.Marshal(packageJSON)
	require.NoError(t, err)
	return CreateVSIX(t, nil, packageJSONBytes)
}

// CreateVSIXFromExtension returns the bytes for a VSIX file containing the
// manifest for the provided test extension and an icon.
func CreateVSIXFromExtension(t *testing.T, ext Extension, version storage.Version) []byte {
	return CreateVSIXFromManifest(t, ConvertExtensionToManifest(ext, version))
}
