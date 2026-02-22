#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "Usage: $0 <base-image-tag> <data-image-tag>" >&2
  exit 1
fi

BASE_IMAGE="$1"
DATA_IMAGE="$2"

docker build \
  -f deploy/docker/Dockerfile.data \
  --build-arg BASE_IMAGE="$BASE_IMAGE" \
  -t "$DATA_IMAGE" \
  .

echo "Built data image: $DATA_IMAGE (base: $BASE_IMAGE)"
