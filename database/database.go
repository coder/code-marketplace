package database

import (
	"context"
	"net/url"
	"time"

	"github.com/coder/code-marketplace/storage"
)

// API references:
// https://github.com/microsoft/vscode-vsce/blob/main/src/xml.ts
// https://github.com/microsoft/vscode/blob/main/src/vs/platform/extensionManagement/common/extensionManagement.ts
// https://github.com/microsoft/vscode/blob/32095ba21fc83f38506d5550f9cb4ed0de233447/src/vs/platform/extensionManagement/common/extensionGalleryService.ts

// SortBy implements SortBy.
// https://github.com/microsoft/vscode/blob/main/src/vs/platform/extensionManagement/common/extensionManagement.ts#L254-L263
type SortBy int

const (
	NoneOrRelevance SortBy = 0
	LastUpdatedDate SortBy = 1
	Title           SortBy = 2
	PublisherName   SortBy = 3
	InstallCount    SortBy = 4
	PublishedDate   SortBy = 5
	AverageRating   SortBy = 6
	WeightedRating  SortBy = 12
)

// SortOrder implements SortOrder.
// https://github.com/microsoft/vscode/blob/main/src/vs/platform/extensionManagement/common/extensionManagement.ts#L265-L269
type SortOrder int

const (
	Default    SortOrder = 0
	Ascending  SortOrder = 1
	Descending SortOrder = 2
)

// Criteria implements ICriterium.  The criteria is an OR, not AND except for
// Target.  Any extension that matches any of the criteria (plus Target if
// included) is included in the result.
// https://github.com/microsoft/vscode/blob/a69f95fdf3dc27511517eef5ff62b21c7a418015/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L209-L212
type Criteria struct {
	Type  FilterType `json:"filterType"`
	Value string     `json:"value"`
}

// FilterType implements FilterType.
// https://github.com/microsoft/vscode/blob/a69f95fdf3dc27511517eef5ff62b21c7a418015/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L178-L187
type FilterType int

const (
	Tag              FilterType = 1
	ExtensionID      FilterType = 4
	Category         FilterType = 5
	ExtensionName    FilterType = 7
	Target           FilterType = 8
	Featured         FilterType = 9
	SearchText       FilterType = 10
	ExcludeWithFlags FilterType = 12
)

// Flag implements Flags.
// https://github.com/microsoft/vscode/blob/a69f95fdf3dc27511517eef5ff62b21c7a418015/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L94-L172
type Flag int

const (
	None                       Flag = 0x0
	IncludeVersions            Flag = 0x1
	IncludeFiles               Flag = 0x2
	IncludeCategoryAndTags     Flag = 0x4
	IncludeSharedAccounts      Flag = 0x8
	IncludeVersionProperties   Flag = 0x10
	ExcludeNonValidated        Flag = 0x20
	IncludeInstallationTargets Flag = 0x40
	IncludeAssetURI            Flag = 0x80
	IncludeStatistics          Flag = 0x100
	IncludeLatestVersionOnly   Flag = 0x200
	Unpublished                Flag = 0x1000
)

// Filter implements an untyped object.
// https://github.com/microsoft/vscode/blob/a69f95fdf3dc27511517eef5ff62b21c7a418015/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L340
type Filter struct {
	Criteria   []Criteria `json:"criteria"`
	PageNumber int        `json:"pageNumber"`
	PageSize   int        `json:"pageSize"`
	SortBy     SortBy     `json:"sortBy"`
	SortOrder  SortOrder  `json:"sortOrder"`
}

// Extension implements IRawGalleryExtension.  This represents a single
// available extension along with all its available versions.
// https://github.com/microsoft/vscode/blob/29234f0219bdbf649d6107b18651a1038d6357ac/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L65-L79
type Extension struct {
	ID               string       `json:"extensionId"`
	Name             string       `json:"extensionName"`
	DisplayName      string       `json:"displayName"`
	ShortDescription string       `json:"shortDescription"`
	Publisher        ExtPublisher `json:"publisher"`
	Versions         []ExtVersion `json:"versions"`
	Statistics       []ExtStat    `json:"statistics"`
	Tags             []string     `json:"tags,omitempty"`
	ReleaseDate      time.Time    `json:"releaseDate"`
	PublishedDate    time.Time    `json:"publishedDate"`
	LastUpdated      time.Time    `json:"lastUpdated"`
	Categories       []string     `json:"categories,omitempty"`
	Flags            string       `json:"flags"`
}

// ExtPublisher implements IRawGalleryExtensionPublisher.
// https://github.com/microsoft/vscode/blob/29234f0219bdbf649d6107b18651a1038d6357ac/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L57-L63
type ExtPublisher struct {
	DisplayName    string `json:"displayName"`
	PublisherID    string `json:"publisherId"`
	PublisherName  string `json:"publisherName"`
	Domain         string `json:"string,omitempty"`
	DomainVerified bool   `json:"isDomainVerified,omitempty"`
}

// ExtVersion implements IRawGalleryExtensionVersion.
// https://github.com/microsoft/vscode/blob/29234f0219bdbf649d6107b18651a1038d6357ac/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L42-L50
type ExtVersion struct {
	storage.Version
	LastUpdated      time.Time     `json:"lastUpdated"`
	AssetURI         string        `json:"assetUri"`
	FallbackAssetURI string        `json:"fallbackAssetUri"`
	Files            []ExtFile     `json:"files"`
	Properties       []ExtProperty `json:"properties,omitempty"`
}

// ExtFile implements IRawGalleryExtensionFile.
// https://github.com/microsoft/vscode/blob/29234f0219bdbf649d6107b18651a1038d6357ac/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L32-L35
type ExtFile struct {
	Type   storage.AssetType `json:"assetType"`
	Source string            `json:"source"`
}

// ExtProperty implements IRawGalleryExtensionProperty.
// https://github.com/microsoft/vscode/blob/29234f0219bdbf649d6107b18651a1038d6357ac/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L37-L40
type ExtProperty struct {
	Key   storage.PropertyType `json:"key"`
	Value string               `json:"value"`
}

// ExtStat implements IRawGalleryExtensionStatistics.
// https://github.com/microsoft/vscode/blob/29234f0219bdbf649d6107b18651a1038d6357ac/src/vs/platform/extensionManagement/common/extensionGalleryService.ts#L52-L55
type ExtStat struct {
	StatisticName string  `json:"statisticName"`
	Value         float32 `json:"value"`
}

type Asset struct {
	Extension string
	Publisher string
	Type      storage.AssetType
	Version   storage.Version
}

type Database interface {
	// GetExtensionAssetPath returns the path of an asset by the asset type.
	GetExtensionAssetPath(ctx context.Context, asset *Asset, baseURL url.URL) (string, error)
	// GetExtensions returns paged extensions from the database that match the
	// filter along the total number of matched extensions.
	GetExtensions(ctx context.Context, filter Filter, flags Flag, baseURL url.URL) ([]*Extension, int, error)
}
