#!/usr/bin/env bash
# rog installer — Linux & macOS
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Geogboe/rog/main/install.sh | bash
#
# Environment variables:
#   ROG_VERSION     Pin a specific version (e.g. v0.2.0). Defaults to latest release.
#   ROG_INSTALL_DIR Override install directory. Defaults to $HOME/.local/bin.
#   ROG_DEBUG       Set to 1 for verbose output.

set -euo pipefail

REPO="Geogboe/rog"
BINARY="rog"
INSTALL_DIR="${ROG_INSTALL_DIR:-${HOME}/.local/bin}"
TMPDIR_CLEANUP=""  # global so EXIT trap can always reference it

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

# Colors only when stdout is a terminal
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    BOLD='\033[1m'
    NC='\033[0m'
else
    RED='' GREEN='' YELLOW='' BLUE='' BOLD='' NC=''
fi

info()  { printf "${BLUE}==>${NC} ${BOLD}%s${NC}\n" "$*" >&2; }
ok()    { printf "${GREEN}✓${NC} %s\n" "$*" >&2; }
warn()  { printf "${YELLOW}warning:${NC} %s\n" "$*" >&2; }
error() { printf "${RED}error:${NC} %s\n" "$*" >&2; exit 1; }
debug() { [ "${ROG_DEBUG:-}" = "1" ] && printf "${BLUE}[debug]${NC} %s\n" "$*" >&2 || true; }

# ---------------------------------------------------------------------------
# Detection
# ---------------------------------------------------------------------------

detect_os() {
    local os
    os="$(uname -s)"
    case "${os}" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)       error "Unsupported OS: ${os}" ;;
    esac
}

detect_arch() {
    local arch
    arch="$(uname -m)"
    case "${arch}" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)             error "Unsupported architecture: ${arch}" ;;
    esac
}

# ---------------------------------------------------------------------------
# Version resolution
# ---------------------------------------------------------------------------

resolve_version() {
    if [ -n "${ROG_VERSION:-}" ]; then
        debug "Using pinned version: ${ROG_VERSION}"
        echo "${ROG_VERSION}"
        return
    fi
    info "Fetching latest release version..."
    local version
    # Use /releases?per_page=1 so prereleases are included (/releases/latest skips them)
    version="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases?per_page=1" \
        | grep '"tag_name"' \
        | head -1 \
        | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
    [ -n "${version}" ] || error "Could not determine latest version. Set ROG_VERSION to install a specific version."
    debug "Latest version: ${version}"
    echo "${version}"
}

# ---------------------------------------------------------------------------
# Checksum verification
# ---------------------------------------------------------------------------

verify_checksum() {
    local file="$1"
    local checksums_file="$2"
    local filename
    filename="$(basename "${file}")"

    local expected
    expected="$(grep "[[:space:]]${filename}$" "${checksums_file}" | awk '{print $1}')"
    [ -n "${expected}" ] || error "Checksum not found for ${filename} in checksums.txt"

    local actual
    if command -v sha256sum >/dev/null 2>&1; then
        actual="$(sha256sum "${file}" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
        actual="$(shasum -a 256 "${file}" | awk '{print $1}')"
    else
        warn "No SHA256 tool found (sha256sum or shasum). Skipping checksum verification."
        return
    fi

    if [ "${actual}" != "${expected}" ]; then
        error "Checksum mismatch for ${filename}
  expected: ${expected}
  got:      ${actual}"
    fi
    ok "Checksum verified"
}

# ---------------------------------------------------------------------------
# PATH check — prints a copy-pasteable one-liner if the install dir is missing
# ---------------------------------------------------------------------------

check_path() {
    local install_dir="$1"
    # Check current PATH (colon-separated)
    case ":${PATH}:" in
        *":${install_dir}:"*) return ;;
    esac

    warn "${install_dir} is not in your PATH"
    printf "\n"
    printf "  Add it by running:\n\n"

    local shell_name
    shell_name="$(basename "${SHELL:-}")"
    case "${shell_name}" in
        bash)
            printf "    echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc && source ~/.bashrc\n"
            ;;
        zsh)
            printf "    echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc && source ~/.zshrc\n"
            ;;
        fish)
            printf "    fish_add_path ~/.local/bin\n"
            ;;
        *)
            printf "    export PATH=\"${install_dir}:\$PATH\"\n"
            printf "\n  Add that line to your shell's rc file (e.g. ~/.bashrc or ~/.profile) to make it permanent.\n"
            ;;
    esac
    printf "\n"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

main() {
    command -v curl >/dev/null 2>&1 || error "curl is required but not installed"

    local os arch version version_num archive checksums_url archive_url

    os="$(detect_os)"
    arch="$(detect_arch)"
    version="$(resolve_version)"
    version_num="${version#v}"  # strip leading 'v'

    archive="${BINARY}-${version_num}-${os}-${arch}.tar.gz"
    checksums_url="https://github.com/${REPO}/releases/download/${version}/checksums.txt"
    archive_url="https://github.com/${REPO}/releases/download/${version}/${archive}"

    debug "OS: ${os}, Arch: ${arch}"
    debug "Version: ${version} (${version_num})"
    debug "Archive: ${archive}"
    debug "Install dir: ${INSTALL_DIR}"

    info "Installing rog ${version} (${os}/${arch})..."

    # Create temp dir; clean up on exit (TMPDIR_CLEANUP is global so trap can access it)
    TMPDIR_CLEANUP="$(mktemp -d)"
    trap 'rm -rf "${TMPDIR_CLEANUP}"' EXIT

    # Download checksums
    info "Downloading checksums..."
    curl -fsSL "${checksums_url}" -o "${TMPDIR_CLEANUP}/checksums.txt"
    debug "Checksums saved to ${TMPDIR_CLEANUP}/checksums.txt"

    # Download archive
    info "Downloading ${archive}..."
    curl -fsSL --progress-bar "${archive_url}" -o "${TMPDIR_CLEANUP}/${archive}"

    # Verify checksum
    info "Verifying checksum..."
    verify_checksum "${TMPDIR_CLEANUP}/${archive}" "${TMPDIR_CLEANUP}/checksums.txt"

    # Create install dir
    mkdir -p "${INSTALL_DIR}"

    # Extract binary
    info "Installing to ${INSTALL_DIR}..."
    tar -xzf "${TMPDIR_CLEANUP}/${archive}" -C "${TMPDIR_CLEANUP}" "${BINARY}"
    install -m 755 "${TMPDIR_CLEANUP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"

    ok "rog ${version} installed to ${INSTALL_DIR}/${BINARY}"
    printf "\n"

    # PATH check
    check_path "${INSTALL_DIR}"

    printf "Run 'rog --help' to get started.\n"
}

main "$@"
