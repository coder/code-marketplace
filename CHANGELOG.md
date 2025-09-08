# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

- Update the Kubernetes Deployment `spec.strategy.type` field to be of type `Recreate`
  in order to properly handle upgrades/restarts as the default deployment creates a PVC
  of type `ReadWriteOnce` and could only be assigned to one replica.
- Expose the `--verbose` and `--sign` runtime parameters as Helm variables.

## [2.4.0](https://github.com/coder/code-marketplace/releases/tag/v2.4.0) - 2025-09-04

### Added

- Implement the new `/api/vscode/{publisher}/{extension}/latest` endpoint. This
  will fix issues where VS Code is unable to install extensions unless you
  explicitly provide the version.

## [2.3.1](https://github.com/coder/code-marketplace/releases/tag/v2.3.1) - 2025-03-06

### Changed

- Updated several dependencies with CVEs.

## [2.3.0](https://github.com/coder/code-marketplace/releases/tag/v2.3.0) - 2024-12-20

### Added

- Add empty signatures when starting the server with --sign. This will not work
  with VS Code on Windows and macOS as we do not have the key required, but it
  will work for open source versions of VS Code (VSCodium, code-server) and VS
  Code on Linux where signatures must exist but are not actually checked.

### Changed

- Ignore extensions without a manifest. This is not expected in normal use, but
  could happen if, for example, a manifest temporarily failed to download, which
  would then crash the entire process with a segfault.

## [2.2.1](https://github.com/coder/code-marketplace/releases/tag/v2.2.1) - 2024-08-14

### Fixed

- The "attempt to download manually" URL in VS Code will now work.

## [2.2.0](https://github.com/coder/code-marketplace/releases/tag/v2.2.0) - 2024-07-17

### Changed

- Default max page size increased from 50 to 200.

### Added

- New `server` sub-command flag `--max-page-size` for setting the max page size.

## [2.1.0](https://github.com/coder/code-marketplace/releases/tag/v2.1.0) - 2023-12-21

### Added

- New `server` sub-command flag `--list-cache-duration` for setting the duration
  of the cache used when listing and searching extensions. The default is still
  one minute.
- Local storage will also use a cache for listing/searching extensions
  (previously only Artifactory storage used a cache).

## [2.0.1](https://github.com/coder/code-marketplace/releases/tag/v2.0.1) - 2023-12-08

### Fixed

- Extensions with problematic UTF-8 characters will no longer cause a panic.
- Preview extensions will now show up as such.

## [2.0.0](https://github.com/coder/code-marketplace/releases/tag/v2.0.0) - 2023-10-11

### Breaking changes

- When removing extensions, the version is now delineated by `@` instead of `-`
  (for example `remove vscodevim.vim@1.0.0`). This fixes being unable to remove
  extensions with `-` in their names. Removal is the only backwards-incompatible
  change; extensions are still added, stored, and queried the same way.

### Added

- Support for platform-specific extensions. Previously all versions would have
  been treated as universal and overwritten each other but now versions for
  different platforms will be stored separately and show up separately in the
  API response. If there are platform-specific versions that have already been
  added, they will continue to be treated as universal versions so these should
  be removed and re-added to be properly registered as platform-specific.

## [1.2.2](https://github.com/coder/code-marketplace/releases/tag/v1.2.2) - 2023-05-30

### Changed

- Help/usage outputs the binary name as `code-marketplace` instead of
  `marketplace` to be consistent with documentation.
- Binary is symlinked into /usr/local/bin in the Docker image so it can be
  invoked as simply `code-marketplace`.

## [1.2.1](https://github.com/coder/code-marketplace/releases/tag/v1.2.1) - 2022-10-31

### Fixed

- Adding extensions from a URL. This broke in 1.2.0 with the addition of bulk
  adding.

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
