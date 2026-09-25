#!/usr/bin/env bash
# Build the Linux worker bundled with Windows rog releases and source builds.
# Writes a deterministic gzip asset and its SHA-256 manifest.
set -euo pipefail

project_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
worker_tmp=$(mktemp)
trap 'rm -f "$worker_tmp"' EXIT
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags='-s -w' -o "$worker_tmp" "$project_root/cmd/rog-worker"
asset_dir="$project_root/internal/workerbundle"
gzip -n -9 -c "$worker_tmp" > "$asset_dir/linux_amd64.gz"
sha256sum "$worker_tmp" | cut -d ' ' -f 1 > "$asset_dir/linux_amd64.sha256"
