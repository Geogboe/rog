# Release Process

This document describes the automated release process for `rog`.

## Overview

The project uses GitHub Actions to automatically build and release binaries for multiple platforms when a version tag is pushed.

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

To create a new release:

1. Ensure all changes are committed and pushed to the main branch
2. Create and push a version tag:

```bash
# Create a tag (use semantic versioning)
git tag -a v1.0.0 -m "Release v1.0.0"

# Push the tag to GitHub
git push origin v1.0.0
```

3. The GitHub Actions workflow will automatically:
   - Build binaries for all supported platforms
   - Create compressed archives (`.tar.gz` for Unix, `.zip` for Windows)
   - Generate SHA256 checksums
   - Create a GitHub release with all artifacts
   - Auto-generate release notes from commits

## Release Artifacts

Each release includes:

- `rog-VERSION-linux-amd64.tar.gz` - Linux x86_64 binary
- `rog-VERSION-linux-arm64.tar.gz` - Linux ARM64 binary
- `rog-VERSION-darwin-amd64.tar.gz` - macOS Intel binary
- `rog-VERSION-darwin-arm64.tar.gz` - macOS Apple Silicon binary
- `rog-VERSION-windows-amd64.zip` - Windows x86_64 binary
- `checksums.txt` - SHA256 checksums for all archives

## Versioning

This project follows [Semantic Versioning](https://semver.org/):

- `v1.0.0` - Major release (breaking changes)
- `v1.1.0` - Minor release (new features, backward compatible)
- `v1.1.1` - Patch release (bug fixes, backward compatible)

## Pre-releases

To create a pre-release (alpha, beta, rc):

```bash
# Tag with pre-release identifier
git tag -a v1.0.0-beta.1 -m "Beta release"
git push origin v1.0.0-beta.1
```

The workflow will automatically mark releases with pre-release identifiers as pre-releases on GitHub.

## Testing Releases Locally

Before creating a release tag, you can test the build process locally:

```bash
# Test cross-compilation for all platforms
make test-build  # If Makefile exists

# Or manually test each platform
GOOS=linux GOARCH=amd64 go build -o rog-linux-amd64 -ldflags="-s -w" .
GOOS=linux GOARCH=arm64 go build -o rog-linux-arm64 -ldflags="-s -w" .
GOOS=darwin GOARCH=amd64 go build -o rog-darwin-amd64 -ldflags="-s -w" .
GOOS=darwin GOARCH=arm64 go build -o rog-darwin-arm64 -ldflags="-s -w" .
GOOS=windows GOARCH=amd64 go build -o rog-windows-amd64.exe -ldflags="-s -w" .
```

## Workflow Configuration

The release workflow is defined in `.github/workflows/release.yml` and:

- Triggers on any tag matching `v*`
- Uses Go 1.24.7 (matching go.mod)
- Applies build optimizations with `-ldflags="-s -w"` to reduce binary size
- Creates release artifacts with checksums
- Automatically generates release notes from commits

## Troubleshooting

### Release Workflow Failed

1. Check the [Actions tab](https://github.com/Geogboe/rog/actions) for error details
2. Verify the tag name starts with `v` (e.g., `v1.0.0`)
3. Ensure all tests pass before creating the release tag
4. Check that the Go version in the workflow matches `go.mod`

### Missing Binaries

If some binaries are missing from the release:

1. Check the workflow logs for build errors
2. Verify cross-compilation works locally for that platform
3. Ensure all dependencies support the target platform

### Modifying the Release Workflow

To modify the release process:

1. Edit `.github/workflows/release.yml`
2. Test changes by pushing to a feature branch
3. Create a test tag (e.g., `v0.0.0-test`) to verify the workflow
4. Delete the test release and tag after verification

## Manual Release (Fallback)

If the automated workflow fails, you can create a release manually:

```bash
# Build all binaries
./scripts/build-all.sh  # Create this if needed

# Create archives
tar -czf rog-v1.0.0-linux-amd64.tar.gz rog-linux-amd64
# ... repeat for other platforms

# Generate checksums
sha256sum rog-v1.0.0-*.tar.gz rog-v1.0.0-*.zip > checksums.txt

# Create release via GitHub web interface
# Upload all artifacts manually
```
