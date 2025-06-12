# Kubernetes Service

![Unikorn Logo](https://raw.githubusercontent.com/unikorn-cloud/assets/main/images/logos/light-on-dark/logo.svg#gh-dark-mode-only)
![Unikorn Logo](https://raw.githubusercontent.com/unikorn-cloud/assets/main/images/logos/dark-on-light/logo.svg#gh-light-mode-only)

## Overview

### Resources

Kubernetes service abstracts away installation of Cluster API.

There are two resource types:

* Cluster Managers, that basically are instances of Cluster API that live in Projects provided by the Identity service.
* Clusters, are Kubernetes clusters, and managed by cluster managers.

Cluster managers are actually contained themselves in virtual clusters, this allows horizontal scaling and multi-tenant separation.

### Services

This service is split up into domain specific micro-services:

* Cluster manager and cluster controllers.
  These are reactive services that watch for resource changes, then reconcile reality against the requested state.
* Server is a RESTful interface that manages our custom resource types.
  As it's intended as a public API e.g. for Terraform or a user interface, it integrates authn/authz functionality too.
* Monitor is a daemon that periodically polls out custome resource types, and provides functionality that cannot be triggered by reactive controllers.
  Most notably, this includes automatic upgrades.

## Installation

### Prerequisites

The use the Kubernetes service you first need to install:

* [The identity service](https://github.com/unikorn-cloud/identity) to provide API authentication and authorization.
* [The region service](https://github.com/unikorn-cloud/region) to provide provider agnostic cloud services (e.g. images, flavors and identity management).

### Installing the Service

Is all done via Helm, which means we can also deploy using ArgoCD.
As this is a private repository, we're keeping the charts private for now also, so you'll need to either checkout the correct branch for a local Helm installation, or imbue Argo with an access token to get access to the repository.

#### Installing ArgoCD

ArgoCD is a **required** to use this service.

Deploy Argo using Helm (the release name is _hard coded_, don't change it yet please):

```
helm repo add argo https://argoproj.github.io/argo-helm
helm repo update
helm install argocd argo/argo-cd -n argocd --create-namespace
```

#### Installing Prerequisites

The server component has a couple prerequisites that are required for correct functionality.
If not installing server you can skip to the next section.

You'll need to install:

* cert-manager (used to generate keying material for JWE/JWS and for ingress TLS)
* nginx-ingress (to perform routing, avoiding CORS, and TLS termination)

<details>
<summary>Helm</summary>

```shell
helm repo add jetstack https://charts.jetstack.io
helm repo add nginx https://helm.nginx.com/stable
helm repo update
helm install cert-manager jetstack/cert-manager -n cert-manager --create-namespace
helm install nginx-ingress nginx/nginx-ingress -n nginx-ingress --create-namespace --set controller.ingressClassResource.default=true
```
</details>

<details>
<summary>ArgoCD</summary>

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cert-manager
  namespace: argocd
spec:
  project: default
  source:
    chart: cert-manager
    helm:
      parameters:
      - name: installCRDs
        value: "true"
      releaseName: cert-manager
    repoURL: https://charts.jetstack.io
    targetRevision: v1.10.1
  destination:
    name: in-cluster
    namespace: cert-manager
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
    - CreateNamespace=true
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: nginx-ingress
  namespace: argocd
spec:
  project: default
  source:
    chart: nginx-ingress
    helm:
      parameters:
      - name: controller.service.httpPort.enable
        value: "false"
      releaseName: nginx-ingress
    repoURL: https://helm.nginx.com/stable
    targetRevision: 0.16.1
  destination:
    name: in-cluster
    namespace: nginx-ingress
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
    - CreateNamespace=true
```
</details>

#### Installing the Kubernetes Service

<details>
<summary>Helm</summary>

Create a `values.yaml` for the server component:
A typical `values.yaml` that uses cert-manager and ACME, and external DNS could look like:

```yaml
server:
  ingress:
    host: unikorn.unikorn-cloud.org
    clusterIssuer: letsencrypt-production
    externalDns: true
  oidc:
    issuer: https://identity.unikorn-cloud.org
```

```shell
helm install unikorn-kubernetes charts/kubernetes --namespace unikorn-kubernetes --create-namespace --values values.yaml
```
</details>

<details>
<summary>ArgoCD</summary>

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: unikorn
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://unikorn-cloud.github.io/kubernetes
    chart: kubernetes
    targetRevision: v0.1.8
  destination:
    namespace: unikorn
    server: https://kubernetes.default.svc
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
    - CreateNamespace=true
```
</details>

### Configuring Service Authentication and Authorization

The [Identity Service](https://github.com/unikorn-cloud/identity) describes how to configure a service organization, groups and role mappings for services that require them.

This service requires asynchronous access to the Region API in order to poll cloud identity and physical network status during cluster creation, and delete those resources on cluster deletion.

This service defines the `unikorn-kubernetes` user that will need to be added to a group in the service organization.
It will need the built in role `infra-manager-service` that allows:

* Read access to the `region` endpoints to access external networks
* Read/delete access to the `identites` endpoints to poll and delete cloud identities
* Read/delete access to the `physicalnetworks` endpoints to poll and delete physical networks

## Monitoring & Logging

* Prometheus monitoring can be enabled with the `--set monitoring.enabled=true` flag.
* OTLP (e.g. Jaeger) tracing can be enabled with the `set server.otlpEndpoint=jaeger-collector.default:4318` flag.

See the [monitoring & logging](docs/monitoring.md) documentation from more information on configuring those services in the first instance..

## Documentation

### API

Consult the [server API documentation](pkg/server/README.md) to get started.

## Development

Consult the [developer documentation](DEVELOPER.md) for local development instructions.

The [architecture documentation](ARCHITECTURE.md) details how it all works, and the design considerations.
