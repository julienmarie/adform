#!/bin/sh
set -eu

output=${1:-dist/adform-reader_linux_amd64}
go_bin=${GO:-go}

mkdir -p "$(dirname "$output")"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$go_bin" build \
  -trimpath \
  -buildvcs=false \
  -ldflags='-buildid=' \
  -o "$output" \
  ./cmd/adform-reader
sha256sum "$output" >"$output.sha256"
