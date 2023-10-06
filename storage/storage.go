package storage

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
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

// Platform implements TargetPlatform.
// https://github.com/microsoft/vscode/blob/main/src/vs/platform/extensions/common/extensions.ts#L291-L311
type Platform string

const (
	PlatformWin32X64   Platform = "win32-x64"
	PlatformWin32Ia32  Platform = "win32-ia32"
	PlatformWin32Arm64 Platform = "win32-arm64"

	PlatformLinuxX64   Platform = "linux-x64"
	PlatformLinuxArm64 Platform = "linux-arm64"
	PlatformLinuxArmhf Platform = "linux-armhf"

	PlatformAlpineX64   Platform = "alpine-x64"
	PlatformAlpineArm64 Platform = "alpine-arm64"

	PlatformDarwinX64   Platform = "darwin-x64"
	PlatformDarwinArm64 Platform = "darwin-arm64"

	PlatformWeb Platform = "web"

	PlatformUniversal Platform = "universal"
	PlatformUnknown   Platform = "unknown"
	PlatformUndefined Platform = "undefined"
)

// VSIXManifest implements XMLManifest.PackageManifest.Metadata.Identity.
// https://github.com/microsoft/vscode-vsce/blob/main/src/xml.ts#L14
type VSIXIdentity struct {
	// ID correlates to ExtensionName, *not* ExtensionID.
	ID             string   `xml:"Id,attr"`
	Version        string   `xml:",attr"`
	Publisher      string   `xml:",attr"`
	TargetPlatform Platform `xml:",attr"`
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
	ManifestAssetType AssetType = "Microsoft.VisualStudio.Code.Manifest" // This is the package.json.
	VSIXAssetType     AssetType = "Microsoft.VisualStudio.Services.VSIXPackage"
)

// VSIXAsset implements XMLManifest.PackageManifest.Assets.Asset.
// https://github.com/microsoft/vscode-vsce/blob/main/src/xml.ts#L25
type VSIXAsset struct {
	Type        AssetType `xml:",attr"`
	Path        string    `xml:",attr"`
	Addressable string    `xml:",attr"`
}

type Options struct {
	Artifactory string
	ExtDir      string
	Repo        string
	Logger      slog.Logger
}

// Version is a subset of database.ExtVersion.
type Version struct {
	TargetPlatform Platform `json:"targetPlatform,omitempty"`
	Version        string   `json:"version"`
}

func (v Version) isUniversal() bool {
	switch v.TargetPlatform {
	case PlatformUniversal, PlatformUnknown, PlatformUndefined, "":
		return true
	default:
		return false
	}
}

// Strings encodes the version and platform into a string that can be reversed
// by VersionFromString.  For example 1.0.0@linux-x64.  For universal versions
// the @platform will be omitted.
//
// For directory names it might have been ideal to a nested path such as
// `version/platform` but we use this instead for backwards compatibility since
// we were unpacking directly into the `version` directory.  Otherwise, we would
// have to migrate existing extensions or have a mechanism for detecting in
// which format the extension was being stored.
func (v Version) String() string {
	if v.isUniversal() {
		return v.Version
	} else {
		return fmt.Sprintf("%s@%s", v.Version, v.TargetPlatform)
	}
}

// VersionFromString creates a version from a version directory.  More or less it
// reverses Version.String().  Since @ is not allowed in semantic versions this
// should be future-proof.
func VersionFromString(dir string) Version {
	parts := strings.SplitN(dir, "@", 2)
	var platform Platform
	if len(parts) > 1 {
		platform = Platform(parts[1])
	}
	return Version{
		Version:        parts[0],
		TargetPlatform: platform,
	}
}

// ByVersion implements sort.Interface for sorting Version slices.
type ByVersion []Version

func (vs ByVersion) Len() int      { return len(vs) }
func (vs ByVersion) Swap(i, j int) { vs[i], vs[j] = vs[j], vs[i] }
func (vs ByVersion) Less(i, j int) bool {
	cmp := semver.Compare(vs[i].Version, vs[j].Version)
	if cmp != 0 {
		return cmp >= 0
	}
	if vs[i].Version == vs[j].Version {
		return vs[i].TargetPlatform < vs[j].TargetPlatform
	}
	return vs[i].Version >= vs[j].Version
}

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
	Manifest(ctx context.Context, publisher, name string, version Version) (*VSIXManifest, error)
	// RemoveExtension removes the provided version of the extension.  It errors
	// if the version does not exist or if removing it fails.  If both the version
	// and platform are blank all versions of that extension will be removed.  If
	// only the platform is blank the universal version will be removed.  If only
	// the version is blank it will error; it is not currently possible to delete
	// all versions for a specific platform.
	RemoveExtension(ctx context.Context, publisher, name string, version Version) error
	// Versions returns the available versions of the provided extension in sorted
	// order.  If the extension does not exits it returns an error.
	Versions(ctx context.Context, publisher, name string) ([]Version, error)
	// WalkExtensions applies a function over every extension.  The extension
	// points to the latest version and the versions slice includes all the
	// versions in sorted order including the latest version (which will be in
	// [0]).  If the function returns an error the error is immediately returned
	// which aborts the walk.
	WalkExtensions(ctx context.Context, fn func(manifest *VSIXManifest, versions []Version) error) error
}

const ArtifactoryTokenEnvKey = "ARTIFACTORY_TOKEN"

// NewStorage returns a storage instance based on the provided extension
// directory or Artifactory URL.  If neither or both are provided an error is
// returned.
func NewStorage(ctx context.Context, options *Options) (Storage, error) {
	if (options.Repo != "" || options.Artifactory != "") && options.ExtDir != "" {
		return nil, xerrors.Errorf("cannot use both Artifactory and extension directory")
	} else if options.Artifactory != "" && options.Repo == "" {
		return nil, xerrors.Errorf("must provide repository")
	} else if options.Artifactory != "" {
		token := os.Getenv(ArtifactoryTokenEnvKey)
		if token == "" {
			return nil, xerrors.Errorf("the %s environment variable must be set", ArtifactoryTokenEnvKey)
		}
		return NewArtifactoryStorage(ctx, &ArtifactoryOptions{
			ListCacheDuration: time.Minute,
			Logger:            options.Logger,
			Repo:              options.Repo,
			Token:             token,
			URI:               options.Artifactory,
		})
	} else if options.ExtDir != "" {
		return NewLocalStorage(options.ExtDir, options.Logger)
	}
	return nil, xerrors.Errorf("must provide an Artifactory repository or local directory")
}

// ReadVSIXManifest reads and parses an extension manifest from a vsix file.  If
// the manifest is invalid it will be returned along with the validation error.
func ReadVSIXManifest(vsix []byte) (*VSIXManifest, error) {
	vmr, err := GetZipFileReader(vsix, "extension.vsixmanifest")
	if err != nil {
		return nil, err
	}
	defer vmr.Close()
	return parseVSIXManifest(vmr)
}

// parseVSIXManifest parses an extension manifest from a reader.  If the
// manifest is invalid it will be returned along with the validation error.
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

// VSIXPackageJSON partially implements Manifest.
// https://github.com/microsoft/vscode-vsce/blob/main/src/manifest.ts#L40-L99
type VSIXPackageJSON struct {
	Browser string `json:"browser"`
}

// ReadVSIXPackageJSON reads and parses an extension's package.json from a vsix
// file.
func ReadVSIXPackageJSON(vsix []byte, packageJsonPath string) (*VSIXPackageJSON, error) {
	vpjr, err := GetZipFileReader(vsix, packageJsonPath)
	if err != nil {
		return nil, err
	}
	defer vpjr.Close()
	var pj *VSIXPackageJSON
	err = json.NewDecoder(vpjr).Decode(&pj)
	if err != nil {
		return nil, err
	}
	return pj, nil
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

// ExtensionIDFromManifest returns the full ID of an extension without the the
// platform, for example publisher.name@0.0.1.
func ExtensionIDFromManifest(manifest *VSIXManifest) string {
	return ExtensionID(
		manifest.Metadata.Identity.Publisher,
		manifest.Metadata.Identity.ID,
		manifest.Metadata.Identity.Version)
}

// ExtensionID returns the full ID of an extension without the platform, for
// example publisher.name@0.0.1.
func ExtensionID(publisher, name, version string) string {
	return fmt.Sprintf("%s.%s@%s", publisher, name, version)
}

// ExtensionVSIXNameFromManifest returns the full ID of an extension including
// the platform if not universal, for example publisher.name-0.0.1 or
// publisher.name-0.0.1@linux-x64.
func ExtensionVSIXNameFromManifest(manifest *VSIXManifest) string {
	return ExtensionVSIXName(
		manifest.Metadata.Identity.Publisher,
		manifest.Metadata.Identity.ID,
		Version{
			Version:        manifest.Metadata.Identity.Version,
			TargetPlatform: manifest.Metadata.Identity.TargetPlatform,
		})
}

// ExtensionVSIXName returns the full ID of an extension including the
// platform if not universal, for example publisher.name-0.0.1 or
// publisher.name-0.0.1@linux-x64.
func ExtensionVSIXName(publisher, name string, version Version) string {
	return fmt.Sprintf("%s.%s-%s", publisher, name, version)
}

// ParseExtensionID parses an full or partial extension ID into its separate
// parts: publisher, name, and version (version may be blank).  It does not
// support specifying the platform and requires that the delimiter for the
// version be @.
func ParseExtensionID(id string) (string, string, string, error) {
	re := regexp.MustCompile(`^([^.]+)\.([^@]+)@?(.*)$`)
	match := re.FindAllStringSubmatch(id, -1)
	if match == nil {
		return "", "", "", xerrors.Errorf("\"%s\" does not match <publisher>.<name> or <publisher>.<name>@<version>", id)
	}
	return match[0][1], match[0][2], match[0][3], nil
}
