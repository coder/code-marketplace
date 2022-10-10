package storage

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"golang.org/x/xerrors"
)

// VSIXManifest implement XMLManifest.PackageManifest.
// https://github.com/microsoft/vscode-vsce/blob/main/src/xml.ts#L9-L26
type VSIXManifest struct {
	Metadata     VSIXMetadata
	Installation struct {
		InstallationTarget struct {
			ID string `xml:"Id,attr"`
		}
	}
	Dependencies []string
	Assets       VSIXAssets
}

// VSIXManifest implement XMLManifest.PackageManifest.Metadata.
// https://github.com/microsoft/vscode-vsce/blob/main/src/xml.ts#L11-L22
type VSIXMetadata struct {
	Description  string
	DisplayName  string
	Identity     VSIXIdentity
	Tags         string
	GalleryFlags string
	License      string
	Icon         string
	Properties   VSIXProperties
	Categories   string
}

// VSIXManifest implement XMLManifest.PackageManifest.Metadata.Identity.
// https://github.com/microsoft/vscode-vsce/blob/main/src/xml.ts#L14
type VSIXIdentity struct {
	// ID correlates to ExtensionName, *not* ExtensionID.
	ID             string `xml:"Id,attr"`
	Version        string `xml:",attr"`
	Publisher      string `xml:",attr"`
	TargetPlatform string `xml:",attr"`
}

// VSIXProperties implements XMLManifest.PackageManifest.Metadata.Properties.
// https://github.com/microsoft/vscode-vsce/blob/main/src/xml.ts#L19
type VSIXProperties struct {
	Property []VSIXProperty
}

type PropertyType string

const (
	DependencyPropertyType PropertyType = "Microsoft.VisualStudio.Code.ExtensionDependencies"
	PackPropertyType       PropertyType = "Microsoft.VisualStudio.Code.ExtensionPack"
)

// VSIXProperty implements XMLManifest.PackageManifest.Metadata.Properties.Property.
// https://github.com/microsoft/vscode-vsce/blob/main/src/xml.ts#L19
type VSIXProperty struct {
	ID    PropertyType `xml:"Id,attr"`
	Value string       `xml:",attr"`
}

// VSIXAssets implements XMLManifest.PackageManifest.Assets.
// https://github.com/microsoft/vscode-vsce/blob/main/src/xml.ts#L25
type VSIXAssets struct {
	Asset []VSIXAsset
}

type AssetType string

const (
	VSIXAssetType AssetType = "Microsoft.VisualStudio.Services.VSIXPackage"
)

// VSIXAsset implements XMLManifest.PackageManifest.Assets.Asset.
// https://github.com/microsoft/vscode-vsce/blob/main/src/xml.ts#L25
type VSIXAsset struct {
	Type        AssetType `xml:",attr"`
	Path        string    `xml:",attr"`
	Addressable string    `xml:",attr"`
}

// TODO: Add Artifactory implementation of Storage.
type Storage interface {
	// AddExtension adds the provided VSIX into storage and returns the location
	// for verification purposes.
	AddExtension(ctx context.Context, manifest *VSIXManifest, vsix []byte) (string, error)
	// FileServer provides a handler for fetching extension repository files from
	// a client.
	FileServer() http.Handler
	// Manifest returns the manifest bytes for the provided extension.  The
	// extension asset itself (the VSIX) will be included on the manifest even if
	// it does not exist on the manifest on disk.
	Manifest(ctx context.Context, publisher, name, version string) (*VSIXManifest, error)
	// RemoveExtension removes the provided version of the extension.  It errors
	// if the provided version does not exist or if removing it fails.  If version
	// is blank all versions of that extension will be removed.
	RemoveExtension(ctx context.Context, publisher, name, version string) error
	// Versions returns the available versions of the provided extension in sorted
	// order.  If the extension does not exits it returns an error.
	Versions(ctx context.Context, publisher, name string) ([]string, error)
	// WalkExtensions applies a function over every extension.  The extension
	// points to the latest version and the versions slice includes all the
	// versions in sorted order including the latest version (which will be in
	// [0]).  If the function returns an error the error is immediately returned
	// which aborts the walk.
	WalkExtensions(ctx context.Context, fn func(manifest *VSIXManifest, versions []string) error) error
}

// Read and parse an extension manifest from a vsix file.  If the manifest is
// invalid it will be returned along with the validation error.
func ReadVSIXManifest(vsix []byte) (*VSIXManifest, error) {
	vmr, err := GetZipFileReader(vsix, "extension.vsixmanifest")
	if err != nil {
		return nil, err
	}
	defer vmr.Close()
	return parseVSIXManifest(vmr)
}

// Parse an extension manifest from a reader.  If the manifest is invalid it
// will be returned along with the validation error.
func parseVSIXManifest(reader io.Reader) (*VSIXManifest, error) {
	var vm *VSIXManifest

	decoder := xml.NewDecoder(reader)
	decoder.Strict = false
	err := decoder.Decode(&vm)
	if err != nil {
		return nil, err
	}

	return vm, validateManifest(vm)
}

// validateManifest checks a manifest for issues.
func validateManifest(manifest *VSIXManifest) error {
	identity := manifest.Metadata.Identity
	if identity.Publisher == "" {
		return xerrors.Errorf("manifest did not contain a publisher")
	} else if identity.ID == "" {
		return xerrors.Errorf("manifest did not contain an ID")
	} else if identity.Version == "" {
		return xerrors.Errorf("manifest did not contain a version")
	}

	return nil
}

// ReadVSIX reads the bytes of a VSIX from the specified source.  The source
// might be a URI or a local file path.
func ReadVSIX(ctx context.Context, source string) ([]byte, error) {
	if !strings.HasPrefix(source, "http://") && !strings.HasPrefix(source, "https://") {
		// Assume it is a local file path.
		return os.ReadFile(source)
	}

	resp, err := http.Get(source)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return nil, xerrors.Errorf("error retrieving vsix: status code %d", resp.StatusCode)
	}

	return io.ReadAll(&io.LimitedReader{
		R: resp.Body,
		N: 100 * 1000 * 1000, // 100 MB
	})
}

// ExtensionID returns the full ID of an extension.
func ExtensionID(manifest *VSIXManifest) string {
	return fmt.Sprintf("%s.%s-%s",
		manifest.Metadata.Identity.Publisher,
		manifest.Metadata.Identity.ID,
		manifest.Metadata.Identity.Version)
}

// ParseExtensionID parses an extension ID into its separate parts: publisher,
// name, and version (version may be blank).
func ParseExtensionID(id string) (string, string, string, error) {
	re := regexp.MustCompile(`^([^.]+)\.([^-]+)-?(.*)$`)
	match := re.FindAllStringSubmatch(id, -1)
	if match == nil {
		return "", "", "", xerrors.Errorf("\"%s\" does not match <publisher>.<name> or <publisher>.<name>-<version>", id)
	}
	return match[0][1], match[0][2], match[0][3], nil
}
