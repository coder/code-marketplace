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

### Getting the binary

The binary can be downloaded from GitHub releases. For example here is a way to
download the latest release using `wget`. Replace `$os` and `$arch` with your
operating system and architecture.

```
wget https://github.com/coder/code-marketplace/releases/latest/download/code-marketplace-$os-$arch -O ./code-marketplace
chmod +x ./code-marketplace
```

### Running the binary

The marketplace server can be ran using the `server` sub-command.

```
./code-marketplace server --extensions-dir ./extensions
```

Run `./code-marketplace --help` for a full list of options.

### Exposing the marketplace

The marketplace must be put behind TLS otherwise code-server will reject
connecting to the API. This could mean using a reverse proxy like NGINX or Caddy
with your own domain and certificates or using a service like Cloudflare.

When hosting the marketplace behind a reverse proxy set either the `Forwarded`
header or both the `X-Forwarded-Host` and `X-Forwarded-Proto` headers. These are
used to generate absolute URLs to extension assets in API responses.

The marketplace does not support being hosted behind a base path; it must be
proxied at the root of your domain.

### Health checks

The `/healthz` endpoint can be used to determine if the marketplace is ready to
receive requests.

## Adding extensions

Extensions can be added to the marketplace by file or URL. The extensions
directory does not need to be created beforehand.

```
./code-marketplace add extension.vsix --extensions-dir ./extensions
./code-marketplace add https://domain.tld/extension.vsix --extensions-dir ./extensions
```

If the extension has dependencies or is in an extension pack those details will
be printed.  Extensions listed as dependencies must also be added but extensions
in a pack are optional.

If an extension is open source you can get it from one of three locations:

1. GitHub releases (if the extension publishes releases to GitHub).
2. Open VSX (if the extension is published to Open VSX).
3. Building from source.

For example to add the Python extension from Open VSX:

```
./code-marketplace add https://open-vsx.org/api/ms-python/python/2022.14.0/file/ms-python.python-2022.14.0.vsix --extensions-dir ./extensions
```

Or the Vim extension from GitHub:

```
./code-marketplace add https://github.com/VSCodeVim/Vim/releases/download/v1.24.1/vim-1.24.1.vsix --extensions-dir ./extensions
```

## Removing extensions

Extensions can be removed from the marketplace by ID and version (or use `--all`
to remove all versions).

```
./code-marketplace remove ms-python.python-2022.14.0 --extensions-dir ./extensions
./code-marketplace remove ms-python.python --all --extensions-dir ./extensions
```

## Usage in code-server

```
export EXTENSIONS_GALLERY='{"serviceUrl":"https://<domain>/api", "itemUrl":"https://<domain>/item", "resourceUrlTemplate": "https://<domain>/files/{publisher}/{name}/{version}/{path}"}'
code-server
```

If code-server reports content security policy errors ensure that the
marketplace is running behind an https URL.

## Development

```
make test
mkdir -p extensions
go run ./cmd/marketplace/main.go server --extensions-dir ./extensions
```

When testing with code-server you may run into issues with content security
policy if the marketplace runs on a different domain over HTTP; in this case you
will need to disable content security policy in your browser or manually edit
the policy in code-server's source.

When you make a change that affects people deploying the marketplace please
update the changelog as part of your PR.

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

- jFrog integration for file storage.
- Helm chart for deployment.
