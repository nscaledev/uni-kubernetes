# Development Guidelines

## Architectural Specification

All contributors and AI agents must follow the Nscale Cloud Platform Unified Architectural Specification:

**https://raw.githubusercontent.com/nscaledev/uni-specifications/refs/heads/main/SPECIFICATION.md**

## Pre-commit / Pre-push Checklist

The following make commands must pass before committing and pushing changes:

```sh
make license
make validate
make lint
make generate
[[ -z $(git status --porcelain) ]]  # generated code must be checked in
make test-unit
```
