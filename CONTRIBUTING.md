# Contributing

## Development

### Requirements

- Go 1.21 or later
- GNU Make

### Building from source

Build all platform binaries:

```console
make build
```

Build a specific platform:

```console
make bin/code-marketplace-linux-amd64
```

Available targets:
- `bin/code-marketplace-mac-amd64`
- `bin/code-marketplace-mac-arm64`
- `bin/code-marketplace-linux-amd64`
- `bin/code-marketplace-linux-arm64`
- `bin/code-marketplace-windows-amd64`
- `bin/code-marketplace-windows-arm64`

### Running locally

```console
mkdir extensions
go run ./cmd/marketplace/main.go server --extensions-dir ./extensions
```

When you make a change that affects people deploying the marketplace please
update the changelog as part of your PR.

You can use `make gen` to generate a mock `extensions` directory for testing and
`make upload` to upload them to an Artifactory repository.

## Tests

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

See the readme for using the marketplace with code-server.

When testing with code-server you may run into issues with content security
policy if the marketplace runs on a different domain over HTTP; in this case you
will need to disable content security policy in your browser or manually edit
the policy in code-server's source.

## Releasing

1. Check that the changelog lists all the important changes.
2. Update the changelog with the release date.
3. Push a tag with the new version.
4. Update the resulting draft release with the changelog contents.
5. Publish the draft release.
6. Bump the Helm chart version once the Docker images have published.
