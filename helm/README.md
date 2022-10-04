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

It is recommended to configure `ingress` with TLS or put the external IP behind
a TLS-terminating reverse proxy because code-server will refuse to connect to
the marketplace if it is not behind HTTPS.

More information can be found at these links:

https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types
https://kubernetes.io/docs/concepts/services-networking/ingress/

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
