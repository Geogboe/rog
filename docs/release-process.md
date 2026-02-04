# Release Process

This document describes the automated release process for `rog`.

## Overview

The project uses:

- **CI** (`.github/workflows/test.yml`) to validate pushes and pull requests
- **Release Please** (`.github/workflows/release-please.yml`) to manage versioning, changelog updates, tags, and GitHub releases
- **GoReleaser** (`.goreleaser.yaml`) to build and upload release artifacts

Releases are created from changes merged into `main`. You do not create tags manually.

## Supported Platforms

The release pipeline builds binaries for the following platforms:

- **Linux**
  - amd64 (x86_64)
  - arm64 (aarch64)
- **macOS**
  - amd64 (Intel)
  - arm64 (Apple Silicon)
- **Windows**
  - amd64 (x86_64)

## Creating a Release

To cut a release:

1. Use Conventional Commits in merged PRs (`feat:`, `fix:`, etc.).
2. Push/merge changes to `main`.
3. Release Please will open or update a **release PR** with:
   - version bumps
   - changelog updates
4. Merge the release PR.
5. GitHub Actions will automatically:
   - create a version tag and GitHub release
   - run GoReleaser
   - publish archives and `checksums.txt` to the release

### First Release and Pre-release Track

The repository is configured with Release Please prerelease settings and currently has `v0.1.0` published as a GitHub prerelease.

## Release Artifacts

Each release includes:

- `rog-<version>-linux-amd64.tar.gz` - Linux x86_64 binary
- `rog-<version>-linux-arm64.tar.gz` - Linux ARM64 binary
- `rog-<version>-darwin-amd64.tar.gz` - macOS Intel binary
- `rog-<version>-darwin-arm64.tar.gz` - macOS Apple Silicon binary
- `rog-<version>-windows-amd64.zip` - Windows x86_64 binary
- `checksums.txt` - SHA256 checksums for all archives

`<version>` is the numeric version without `v` (example: `0.1.0`).

## Versioning

This project follows [Semantic Versioning](https://semver.org/):

- `v1.0.0` - Major release (breaking changes)
- `v1.1.0` - Minor release (new features, backward compatible)
- `v1.1.1` - Patch release (bug fixes, backward compatible)

## Pre-releases

Pre-release versions are generated automatically by Release Please based on repository configuration. While pre-release mode is enabled, releases are marked as pre-releases in GitHub.

## Testing Releases Locally

Before merging a release PR, you can test locally:

```bash
# CI checks
go vet ./...
go test ./...

# Local GoReleaser snapshot build (no publish)
goreleaser release --snapshot --clean
```

## Workflow Configuration

- `.github/workflows/test.yml`: tests and cross-platform build checks
- `.github/workflows/release-please.yml`: runs release-please on pushes to `main`
- `release-please-config.json` + `.release-please-manifest.json`: release strategy
- `.goreleaser.yaml`: build matrix, archives, checksum, release behavior

## Troubleshooting

### Release Workflow Failed

1. Check the [Actions tab](https://github.com/Geogboe/rog/actions) for error details
2. Confirm the release PR was merged into `main`
3. Ensure repository settings allow Actions to create PRs:
   - Settings -> Actions -> General -> Workflow permissions: **Read and write permissions**
   - **Allow GitHub Actions to create and approve pull requests** is enabled
4. Ensure release workflow/job permissions include `contents: write` and `pull-requests: write`
5. Check that the Go version in workflows matches `go.mod`

### Missing Binaries

If some binaries are missing from the release:

1. Check the workflow logs for build errors
2. Verify cross-compilation works locally for that platform
3. Ensure all dependencies support the target platform

### Modifying the Release Workflow

To modify the release process:

1. Edit `.github/workflows/release-please.yml` and/or `.goreleaser.yaml`
2. Test changes by pushing to a feature branch
3. Use `workflow_dispatch` on `release-please.yml` for ad hoc verification if needed
