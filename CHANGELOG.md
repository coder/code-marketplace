# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

## [1.2.0](https://github.com/coder/code-marketplace/releases/tag/v1.2.0) - 2022-10-20

### Added

- Artifactory integration. Set the ARTIFACTORY_TOKEN environment variable and
  pass --artifactory and --repo (instead of --extensions-dir) to use.
- Stat endpoints. This is just to prevent noisy 404s from being logged; the
  endpoints do nothing since stats are not yet supported.
- Bulk add from a directory.  This only works when adding from a local directory
  and not from web URLs.

## [1.1.0](https://github.com/coder/code-marketplace/releases/tag/v1.1.0) - 2022-10-03

### Added

- `add` sub-command for adding extensions to the marketplace.
- `remove` sub-command for removing extensions from the marketplace.

### Changed

- Compile statically so binaries work on Alpine.

## [1.0.0](https://github.com/coder/code-marketplace/releases/tag/v1.0.0) - 2022-09-12

### Added

- Initial marketplace implementation.
