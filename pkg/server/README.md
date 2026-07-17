# Unikorn Server

## Code Generation

Everything is done with an OpenAPI schema.
This allows us to auto-generate the server routing, schema validation middleware, types and clients.
This happens automatically on update via the `Makefile`.
Please ensure updated generated code is commited with your pull request.

## API Definition

Consult the [OpenAPI schema](../../pkg/openapi/server.spec.yaml) for full details of what it does.

## Authorization

The remote authorizer's decision mode is configurable via `--authorization-engine-mode`
(`off`/`shadow`/`enforce`, default `off`) and `--authorization-check-timeout` (default
`250ms`). `off` preserves the authorizer's original behavior (local ACL walk, no remote
PDP consulted); `shadow` additionally evaluates identity's central PDP and logs
divergence without changing the served verdict; `enforce` makes the remote PDP
authoritative.

## Getting Started with Development and Testing.

Once everything is up and running, grab the IP address:

```bash
export INGRESS_ADDR=$(kubectl -n unikorn get ingress/unikorn-server -o 'jsonpath={.status.loadBalancer.ingress[0].ip}')
```
And add it to your resolver:

```bash
echo "${INGRESS_ADDR} unikorn.unikorn-cloud.org" >> /etc/hosts
```
