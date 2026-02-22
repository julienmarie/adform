# Adform Deployment (Code Image + Data Image)

This repository now contains deployment assets for a two-image model:

1. Base image (code): `adform` binary only.
2. Data image (runtime): base image + `accounts/` content.

This keeps code and account data separable while producing one immutable runtime image per deploy.

## Canonical Data Tree

Use `accounts/<account>/...` as the canonical account data layout.

Existing legacy paths are intentionally kept in place for backward compatibility.

## Build

Build base image (from code):

```bash
./deploy/scripts/build-base.sh ghcr.io/your-org/adform:code-<git-sha>
```

Build data image (from account data + base image):

```bash
./deploy/scripts/build-data.sh \
  ghcr.io/your-org/adform:code-<git-sha> \
  ghcr.io/your-org/adform-data:deploy-<data-sha>
```

## GitHub Actions

This repo publishes the base code image automatically via:

- `.github/workflows/publish-code-image.yml`

Published tags in `ghcr.io/<owner>/adform`:

- `code-main` on pushes to `main`
- `code-<full-sha>` on every push
- `code-<git-tag>` on tagged releases (`v*`)

The private data repo (`btd_marketing`) can build from `code-main` by default, or from a pinned `code-<sha>` for deterministic rollouts.

## Run Locally

```bash
docker run --rm -p 8080:8080 \
  -e ADFORM_SERVER_ACCOUNT=btd_main \
  -e ADFORM_SERVER_ENV=prod \
  -e ADFORM_STATE_PATH=/var/lib/adform/state.db \
  -v adform-state:/var/lib/adform \
  ghcr.io/your-org/adform-data:deploy-<data-sha>
```

## Kubernetes

Example manifests are under `deploy/k8s`.

Apply:

```bash
kubectl apply -k deploy/k8s
```

Notes:
- No Ingress manifest is included.
- State uses `emptyDir` by default in the example Deployment.
- For persistence, replace `emptyDir` with a PVC-backed volume.
