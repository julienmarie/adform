# Release checksums

`adform-reader_linux_amd64.sha256` records the deterministic Linux/amd64 reader artifact produced by:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags='-buildid=' -o adform-reader_linux_amd64 ./cmd/adform-reader
```

The build script runs this command and writes a matching `<artifact>.sha256` file.

The reader is pinned to Meta Graph API `v26.0` and accepts only Karajan's
read-only JSON stats contract.
