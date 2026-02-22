#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 <base-image-tag>" >&2
  exit 1
fi

IMAGE_TAG="$1"

docker build \
  -f deploy/docker/Dockerfile.base \
  -t "$IMAGE_TAG" \
  .

echo "Built base image: $IMAGE_TAG"
