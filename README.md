# Code Extension Marketplace

The Code Extension Marketplace is an open-source alternative to the VS Code
Marketplace for use in editors like
[code-server](https://github.com/cdr/code-server).

This marketplace reads extensions from file storage and provides an API for
editors to consume. It does not have a frontend or any mechanisms for extension
authors to add or update extensions in the marketplace.

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

## Adding extensions

Extensions can be added to the marketplace by file or URL.  The extensions
directory does not need to be created beforehand.

```
./code-marketplace add extension.vsix --extensions-dir ./extensions
./code-marketplace add https://domain.tld/extension.vsix --extensions-dir ./extensions
```

Extensions listed as dependencies must also be added.

If the extension is part of an extension pack the other extensions in the pack
can also be added but doing so is optional.

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
