# Code Extension Marketplace

The Code Extension Marketplace is an open-source alternative to the VS Code
Marketplace for use in editors like
[code-server](https://github.com/coder/code-server) or [VSCodium](https://github.com/VSCodium/vscodium).

It is maintained by [Coder](https://www.coder.com) and is used by our enterprise
customers in regulated and security-conscious industries like banking, asset
management, military, and intelligence where they deploy Coder in an air-gapped
network and accessing an internet-hosted marketplace is not allowed.

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

The token will be used in the `Authorization` header with the value `Bearer
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
curl 'https://example.com/api/extensionquery' -H 'Accept: application/json;api-version=3.0-preview.1' --compressed -H 'Content-Type: application/json' --data-raw '{"filters":[{"criteria":[{"filterType":8,"value":"Microsoft.VisualStudio.Code"}],"pageSize":1}],"flags":439}' | jq .results[0].extensions[0].versions[0].assetUri
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
./code-marketplace add https://github.com/VSCodeVim/Vim/releases/download/v1.24.1/vim-1.24.1.vsix [flags]
```

## Removing extensions

Extensions can be removed from the marketplace by ID and version or `--all` to
remove all versions.

```console
./code-marketplace remove ms-python.python@2022.14.0 [flags]
./code-marketplace remove ms-python.python --all [flags]
```

## Scanning frequency and caching

The marketplace does not utilize a database. When an extension query is made,
the marketplace scans the local file system or queries Artifactory on demand to
find all the available extensions.

However, for Artifactory in particular this can be slow, so this full list of
extensions is cached in memory for a default of one minute and reused for any
subsequent requests that fall within that duration. This duration can be
configured or disabled with `--list-cache-duration` and applies to both storage
backends.

This means that when you add or remove an extension, depending on when the last
request was made, it can take a duration between zero and
`--list-cache-duration` for the query response to reflect that change.

Artifactory storage also uses a second in-memory cache for extension manifests,
which are referenced in extension queries (for things like categories). This
cache is initially populated with all available extension manifests on startup.
Extensions added after the server is running are added to the cache on-demand
the next time extensions are scanned.

The manifest cache has no expiration and never evicts manifests because it was
expected that extensions are typically only ever added and individual extension
version manifests never change; however we would like to implement evicting
manifests of extensions that have been removed.

With local storage, manifests are read directly from the file system on
demand. Requests for other extension assets (such as icons) for both storage
backends have no cache and are read/proxied directly from the file system or
Artifactory since they are not in the extension query hot path.

## Usage in code-server

You can point code-server to your marketplace by setting the
`EXTENSIONS_GALLERY` environment variable.

The value of this variable is a JSON blob that specifies the service URL, item
URL, and resource URL template.

- `serviceURL`: specifies the location of the API (`https://<domain>/api`).
- `itemURL`: the frontend for extensions which is currently just a mostly blank
  page that says "not supported" (`https://<domain>/item`)
- `resourceURLTemplate`: used to download web extensions like Vim; code-server
  itself will replace the `{publisher}`, `{name}`, `{version}`, and `{path}`
  template variables so use them verbatim
  (`https://<domain>/files/{publisher}/{name}/{version}/{path}`).

For example (replace `<domain>` with your marketplace's domain):

```console
export EXTENSIONS_GALLERY='{"serviceUrl":"https://<domain>/api", "itemUrl":"https://<domain>/item", "resourceUrlTemplate": "https://<domain>/files/{publisher}/{name}/{version}/{path}"}'
code-server
```

If code-server reports content security policy errors ensure that the
marketplace is running behind an https URL.

### Custom certificate authority

If you are using a custom certificate authority or a self-signed certificate and
get errors like "unable to verify the first certificate", you may need to set
the [NODE_EXTRA_CA_CERTS](https://nodejs.org/api/cli.html#node_extra_ca_certsfile)
environment variable for code-server to find your certificates bundle.

Make sure your bundle contains the full certificate chain. This can be necessary
because Node does not read system certificates by default and while VS Code has
code for reading them, it appears not to work or be enabled for the web version.

Some so-called "web" extensions (like `vscodevim.vim`) are installed in the
browser, and extension searches are also performed from the browser, so your
certificate bundle may also need to be installed on the client machine in
addition to the remote machine.

## Usage in VS Code & VSCodium

Although not officially supported, you can follow the examples below to start
using code-marketplace with VS Code and VSCodium:

- [VS Code](https://github.com/eclipse/openvsx/wiki/Using-Open-VSX-in-VS-Code)

  Extension signing may have to be disabled in VS Code.

- [VSCodium](https://github.com/VSCodium/vscodium/blob/master/docs/index.md#howto-switch-marketplace)

  ```console
  export VSCODE_GALLERY_SERVICE_URL="https://<domain>/api"
  export VSCODE_GALLERY_ITEM_URL="https://<domain>/item"
  # Or set a product.json file in `~/.config/VSCodium/product.json`
  codium
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

- Bulk add from one Artifactory repository to another (or to itself).
- Optional database to speed up queries.
- Progress indicators when adding/removing extensions.
