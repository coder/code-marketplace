# Code Extension Marketplace

The Code Extension Marketplace is an open-source alternative to the VS Code
Marketplace for use in editors like
[code-server](https://github.com/cdr/code-server).

This marketplace reads extensions from file storage and provides an API for
editors to consume. It does not have a frontend or any mechanisms for adding or
updating extensions in the marketplace.

## Deployment

Replace `$os` and `$arch` with your operating system and architecture.

```
wget https://github.com/coder/code-marketplace/releases/latest/download/code-marketplace-$os-$arch -O ./code-marketplace
chmod +x ./code-marketplace
./code-marketplace server --extensions-dir /my/extensions
```

Run `./code-marketplace --help` for a full list of options.

It is recommended to put the marketplace behind TLS otherwise code-server will
reject connecting to the API.

The `/healthz` endpoint can be used to determine if the marketplace is ready to
receive requests.

## File Storage

Extensions must be both copied as a vsix and extracted to the following path:

```
<extensions-dir>/<publisher>/<extension name>/<version>/
```

For example:

```
extensions
|-- ms-python
|   `-- python
|       `-- 2022.14.0
|           |-- [Content_Types].xml
|           |-- extension
|           |-- extension.vsixmanifest
|           `-- ms-python.python-2022.14.0.vsix
`-- vscodevim
    `-- vim
        `-- 1.23.2
            |-- [Content_Types].xml
            |-- extension
            |-- extension.vsixmanifest
            `-- vscodevim.vim-1.23.2.vsix
```

## Usage in code-server

```
export EXTENSIONS_GALLERY='{"serviceUrl":"https://<domain>/api", "itemUrl":"https://<domain>/item", "resourceUrlTemplate": "https://<domain>/files/{publisher}/{name}/{version}/{path}"}'
code-server
```

If code-server reports content security policy errors ensure that the
marketplace is running behind an https URL.

## Reverse proxy

To host the marketplace behind a reverse proxy set either the `Forwarded` header
or both the `X-Forwarded-Host` and `X-Forwarded-Proto` headers.

The marketplace does not support being hosted behind a base path; it must be
proxied at the root of your domain.

## Getting extensions

If an extension is open source you can get it from one of three locations:

1. GitHub releases (if the extension publishes releases to GitHub).
2. Open VSX (if the extension is published to Open VSX).
3. Building from source.

For example to download the Python extension from Open VSX:

```
mkdir -p extensions/ms-python/python/2022.14.0
wget https://open-vsx.org/api/ms-python/python/2022.14.0/file/ms-python.python-2022.14.0.vsix
unzip ms-python.python-2022.14.0.vsix -d extensions/ms-python/python/2022.14.0
mv ms-python.python-2022.14.0.vsix extensions/ms-python/python/2022.14.0
```

Make sure to both extract the contents *and* copy/move the `.vsix` file.

If an extension has dependencies those must be added as well.  An extension's
dependencies can be found in the extension's `package.json` under
`extensionDependencies`.

Extensions under `extensionPack` in the extension's `package.json` can be added
as well although doing so is not required.

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
