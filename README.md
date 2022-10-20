# Code Extension Marketplace

The Code Extension Marketplace is an open-source alternative to the VS Code
Marketplace for use in editors like
[code-server](https://github.com/coder/code-server).

This marketplace reads extensions from file storage and provides an API for
editors to consume. It does not have a frontend or any mechanisms for extension
authors to add or update extensions in the marketplace.

## Deployment

The marketplace is a single binary. Deployment involves running the binary,
pointing it to a directory of extensions, and exposing the binary's bound
address in some way.

### Kubernetes

If deploying with Kubernetes see the [Helm directory](./helm) otherwise read on.

### Getting the binary

The binary can be downloaded from GitHub releases. For example here is a way to
download the latest release using `wget`. Replace `$os` and `$arch` with your
operating system and architecture.

```console
wget https://github.com/coder/code-marketplace/releases/latest/download/code-marketplace-$os-$arch -O ./code-marketplace
chmod +x ./code-marketplace
```

### Running the server

The marketplace server can be ran using the `server` sub-command.

```console
./code-marketplace server [flags]
```

Run `./code-marketplace --help` for a full list of options.

### Local storage

To use a local directory for extension storage use the `--extensions-dir` flag.

```console

./code-marketplace [command] --extensions-dir ./extensions
```

### Artifactory storage

It is possible use Artifactory as a file store instead of local storage. For
this to work the `ARTIFACTORY_TOKEN` environment variable must be set.

```console
export ARTIFACTORY_TOKEN="my-token"
./code-marketplace [command] --artifactory http://artifactory.server/artifactory --repo extensions
```

The token will be used as the `Authorization` header with the value `Bearer
<TOKEN>`.

### Exposing the marketplace

The marketplace must be put behind TLS otherwise code-server will reject
connecting to the API. This could mean using a TLS-terminating reverse proxy
like NGINX or Caddy with your own domain and certificates or using a service
like Cloudflare.

When hosting the marketplace behind a reverse proxy set either the `Forwarded`
header or both the `X-Forwarded-Host` and `X-Forwarded-Proto` headers. These
headers are used to generate absolute URLs to extension assets in API responses.
One way to test this is to make a query and check one of the URLs in the
response:

```console
$ curl 'https://example.com/api/extensionquery' -H 'Accept: application/json;api-version=3.0-preview.1' --compressed -H 'Content-Type: application/json' --data-raw '{"filters":[{"criteria":[{"filterType":8,"value":"Microsoft.VisualStudio.Code"}],"pageSize":1}],"flags":439}' | jq .results[0].extensions[0].versions[0].assetUri
"https://example.com/assets/vscodevim/vim/1.24.1"
```

The marketplace does not support being hosted behind a base path; it must be
proxied at the root of your domain.

### Health checks

The `/healthz` endpoint can be used to determine if the marketplace is ready to
receive requests.

## Adding extensions

Extensions can be added to the marketplace by file, directory, or web URL.

```console
./code-marketplace add extension.vsix [flags]
./code-marketplace add extension-vsixs/ [flags]
./code-marketplace add https://domain.tld/extension.vsix [flags]
```

If the extension has dependencies or is in an extension pack those details will
be printed.  Extensions listed as dependencies must also be added but extensions
in a pack are optional.

If an extension is open source you can get it from one of three locations:

1. GitHub releases (if the extension publishes releases to GitHub).
2. Open VSX (if the extension is published to Open VSX).
3. Building from source.

For example to add the Python extension from Open VSX:

```console
./code-marketplace add https://open-vsx.org/api/ms-python/python/2022.14.0/file/ms-python.python-2022.14.0.vsix [flags]
```

Or the Vim extension from GitHub:

```console
./code-marketplace add
https://github.com/VSCodeVim/Vim/releases/download/v1.24.1/vim-1.24.1.vsix [flags]
```

## Removing extensions

Extensions can be removed from the marketplace by ID and version (or use `--all`
to remove all versions).

```console
./code-marketplace remove ms-python.python-2022.14.0 [flags]
./code-marketplace remove ms-python.python --all [flags]
```

## Usage in code-server

```console
export EXTENSIONS_GALLERY='{"serviceUrl":"https://<domain>/api", "itemUrl":"https://<domain>/item", "resourceUrlTemplate": "https://<domain>/files/{publisher}/{name}/{version}/{path}"}'
code-server
```

If code-server reports content security policy errors ensure that the
marketplace is running behind an https URL.

## Development

```console
mkdir extensions
go run ./cmd/marketplace/main.go server [flags]
```

When testing with code-server you may run into issues with content security
policy if the marketplace runs on a different domain over HTTP; in this case you
will need to disable content security policy in your browser or manually edit
the policy in code-server's source.

When you make a change that affects people deploying the marketplace please
update the changelog as part of your PR.

You can use `make gen` to generate a mock `extensions` directory for testing and
`make upload` to upload them to an Artifactory repository.

### Tests

To run the tests:

```
make test
```

To run the Artifactory tests against a real repository instead of a mock:

```
export ARTIFACTORY_URI=myuri
export ARTIFACTORY_REPO=myrepo
export ARTIFACTORY_TOKEN=mytoken
make test
```

## Missing features

- Recommended extensions.
- Featured extensions.
- Download counts.
- Ratings.
- Searching by popularity.
- Published, released, and updated dates for extensions (for example this will
  cause bogus release dates to show for versions).
- Frontend for browsing available extensions.
- Extension validation (only the marketplace owner can add extensions anyway).
- Adding and updating extensions by extension authors.

## Planned work

- Bulk add.
- Bulk add from one Artifactory repository to another (or to itself).
- Optional database to speed up queries.
- Progress indicators when adding/removing extensions.
