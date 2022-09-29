package storage

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/xerrors"
)

const VSIXAssetType = "Microsoft.VisualStudio.Services.VSIXPackage"

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

// VSIXProperty implements XMLManifest.PackageManifest.Metadata.Properties.Property.
// https://github.com/microsoft/vscode-vsce/blob/main/src/xml.ts#L19
type VSIXProperty struct {
	ID    string `xml:"Id,attr"`
	Value string `xml:",attr"`
}

// VSIXAssets implements XMLManifest.PackageManifest.Assets.
// https://github.com/microsoft/vscode-vsce/blob/main/src/xml.ts#L25
type VSIXAssets struct {
	Asset []VSIXAsset
}

// VSIXAsset implements XMLManifest.PackageManifest.Assets.Asset.
// https://github.com/microsoft/vscode-vsce/blob/main/src/xml.ts#L25
type VSIXAsset struct {
	Type        string `xml:",attr"`
	Path        string `xml:",attr"`
	Addressable string `xml:",attr"`
}

// TODO: Add Artifactory implementation of Storage.
type Storage interface {
	// AddExtension adds the extension found at the specified source by copying it
	// into the extension storage directory and returns the location of the new
	// extension.  The source may be an URI or a local file path.
	AddExtension(ctx context.Context, source string) (string, error)
	// FileServer provides a handler for fetching extension repository files from
	// a client.
	FileServer() http.Handler
	// Manifest returns the manifest for the provided extension version.
	Manifest(ctx context.Context, publisher, extension, version string) (*VSIXManifest, error)
	// WalkExtensions applies a function over every extension providing the
	// manifest for the latest version and a list of all available versions.  If
	// the function returns error the error is returned and the iteration aborted.
	WalkExtensions(ctx context.Context, fn func(manifest *VSIXManifest, versions []string) error) error
}

// Parse an extension manifest.
func parseVSIXManifest(reader io.Reader) (*VSIXManifest, error) {
	var vm *VSIXManifest

	decoder := xml.NewDecoder(reader)
	decoder.Strict = false
	err := decoder.Decode(&vm)
	if err != nil {
		return nil, err
	}

	// The extension asset is not stored in the manifest.  Since we always store
	// it next to the manifest using the publisher.name-version format we can set
	// that as the path.
	vm.Assets.Asset = append(vm.Assets.Asset, VSIXAsset{
		Type: VSIXAssetType,
		Path: fmt.Sprintf("%s.%s-%s.vsix",
			vm.Metadata.Identity.Publisher,
			vm.Metadata.Identity.ID,
			vm.Metadata.Identity.Version),
		Addressable: "true",
	})

	return vm, nil
}

// validateManifest checks a manifest for issues.
func validateManifest(manifest *VSIXManifest) error {
	if manifest == nil {
		return xerrors.Errorf("vsix did not contain a manifest")
	}
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

// readVSIX reads the bytes of a VSIX from the specified source.  The source
// might be a URI or a local file path.
func readVSIX(ctx context.Context, source string) ([]byte, error) {
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
