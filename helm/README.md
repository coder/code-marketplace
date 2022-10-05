# Code Extension Marketplace Helm Chart

This directory contains the Helm chart used to deploy the marketplace onto a
Kubernetes cluster.

## Quickstart

```console
$ git clone --depth 1 https://github.com/coder/code-marketplace
$ helm upgrade --install code-marketplace ./code-marketplace/helm
```

This deploys the marketplace on the default Kubernetes cluster.

## Ingress

You will need to configure `ingress` in [values.yaml](./values.yaml) to expose the
marketplace on an external domain or change `service.type` to get yourself an
external IP address.

The marketplace must be put behind TLS otherwise code-server will reject
connecting to the API. This could mean configuring `ingress` with TLS or putting
the external IP behind a TLS-terminating reverse proxy.

More information can be found at these links:

- https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types
- https://kubernetes.io/docs/concepts/services-networking/ingress/

When hosting the marketplace behind a reverse proxy set either the `Forwarded`
header or both the `X-Forwarded-Host` and `X-Forwarded-Proto` headers (the
default `ingress` already takes care of this). These headers are used to
generate absolute URIs to extension assets in API responses. One way to test
this is to make a query and check one of the URIs in the response:

```
$ curl 'https://example.com/api/extensionquery' -H 'Accept: application/json;api-version=3.0-preview.1' --compressed -H 'Content-Type: application/json' --data-raw '{"filters":[{"criteria":[{"filterType":8,"value":"Microsoft.VisualStudio.Code"}],"pageSize":1}],"flags":439}' | jq .results[0].extensions[0].versions[0].assetUri
"https://example.com/assets/vscodevim/vim/1.24.1"
```

The marketplace does not support being hosted behind a base path; it must be
proxied at the root of your domain.

## Adding/removing extensions

One way to get extensions added or removed is to exec into the pod and use the
marketplace binary to add and remove them.

```
export POD_NAME=$(kubectl get pods -l "app.kubernetes.io/name=code-marketplace,app.kubernetes.io/instance=code-marketplace" -o jsonpath="{.items[0].metadata.name}")
$ kubectl exec -it "$POD_NAME" -- /opt/code-marketplace add https://github.com/VSCodeVim/Vim/releases/download/v1.24.1/vim-1.24.1.vsix --extensions-dir /extensions
```

In the future it will be possible to use Artifactory for storing and retrieving
extensions instead of a persistent volume.

## Uninstall

To uninstall/delete the marketplace deployment:

```console
$ helm delete code-marketplace
```

This removes all the Kubernetes components associated with the chart (including
the persistent volume) and deletes the release.

## Configuration

Please refer to [values.yaml](./values.yaml) for available Helm values and their
defaults.

Specify values using `--set`:

```console
$ helm upgrade --install code-marketplace ./helm-chart \
  --set persistence.size=10Gi
```

Or edit and use the YAML file:

```console
$ helm upgrade --install code-marketplace ./helm-chart -f values.yaml
```
