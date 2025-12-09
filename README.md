# rog

> A fast, local-first Git repository navigator and catalog system

**rog** helps you find, understand, and navigate all your Git repositories with instant searching, smart filtering, and optional LLM enrichment.

Like `proj`, but it's not proj.

## Features

- **⚡ Fast**: Index-driven queries complete in milliseconds
- **🔍 Smart Search**: Fuzzy matching across names, descriptions, tags, and paths
- **🏷️ Rich Metadata**: Auto-detect languages, add tags and descriptions
- **🤖 LLM Enrichment**: Optional AI-generated metadata for your repos
- **📊 Powerful Filtering**: By language, tags, branch, status, and more
- **🎯 Scriptable**: Clean JSON/YAML output for automation
- **🪟 WSL Support**: Seamlessly index Windows and WSL repositories
- **🚫 Zero Surprises**: No network calls unless explicitly requested
- **🔒 Read-Only**: Never modifies your repos without permission

## Quick Start

```bash
# Install
go install github.com/Geogboe/rog@latest

# Initialize
rog init

# Edit config to add your project directories
vi ~/.config/rog/config.yml

# Scan repositories
rog scan

# List all repos
rog list

# Jump to a repo
cd "$(rog path myproject)"
```

## Installation

### Pre-built Binaries (Recommended)

Download the latest release for your platform from the [releases page](https://github.com/Geogboe/rog/releases):

```bash
# Linux (amd64)
curl -L https://github.com/Geogboe/rog/releases/latest/download/rog-VERSION-linux-amd64.tar.gz | tar xz
sudo mv rog-linux-amd64 /usr/local/bin/rog

# macOS (Apple Silicon)
curl -L https://github.com/Geogboe/rog/releases/latest/download/rog-VERSION-darwin-arm64.tar.gz | tar xz
sudo mv rog-darwin-arm64 /usr/local/bin/rog

# macOS (Intel)
curl -L https://github.com/Geogboe/rog/releases/latest/download/rog-VERSION-darwin-amd64.tar.gz | tar xz
sudo mv rog-darwin-amd64 /usr/local/bin/rog

# Windows (PowerShell)
# Download from releases page and extract to a directory in your PATH
```

Replace `VERSION` with the actual version number (e.g., `v1.0.0`).

### Using Go Install

```bash
go install github.com/Geogboe/rog@latest
```

### From Source

```bash
git clone https://github.com/Geogboe/rog
cd rog
go build -o rog .
sudo mv rog /usr/local/bin/
```

## Usage

### Basic Commands

```bash
# List all repositories
rog list

# Search for repos
rog list api backend

# Filter by language
rog list --lang go --tag cli

# Show dirty repos
rog list --dirty

# Get detailed info
rog info myproject

# Open in editor
rog open myproject

# Get path (for scripting)
cd "$(rog path api)"
```

### Interactive Selection

```bash
# Select with fzf (if installed)
rog select

# Use in scripts
cd "$(rog select)"
code "$(rog select --lang go)"
```

### Advanced Filtering

```bash
# Multiple languages
rog list --lang go --lang rust

# All tags must match (AND logic)
rog list --tag cli --tag rest

# Sort and limit
rog list --sort last-commit --limit 10

# Machine-readable output
rog list --format json
rog list --format yaml
```

### Remote Status

```bash
# Check remote status (requires network)
rog scan --remote

# Show repos behind remote
rog list --behind

# Show repos ahead of remote
rog list --ahead
```

### LLM Enrichment

```bash
# Generate descriptions and tags
rog scan --llm

# Refresh existing LLM metadata
rog scan --llm --refresh-meta
```

## Configuration

Default location: `~/.config/rog/config.yml`

```yaml
# Global excludes apply to all roots (supports glob patterns)
global_excludes:
  - node_modules
  - vendor
  - ".git"
  - target
  - build
  - dist
  - "*-cache"        # Glob pattern: matches test-cache, build-cache, etc.
  - "**/__pycache__" # Matches __pycache__ at any depth

roots:
  - name: dev
    path: ~/dev
    max_depth: 4
    exclude:
      # Root-specific excludes (merged with global_excludes)
      - ".idea"
      - ".vscode"

  - name: work
    path: ~/work/projects
    max_depth: 5

editor: code

llm:
  endpoint: http://localhost:11434/v1
  model: codellama
  extra_instructions: "Focus on domain and technology tags."
```

### Configuration Options

- `global_excludes`: Directory patterns to exclude from all roots (supports glob patterns like `*-cache`, `**/vendor`)
- `roots[].exclude`: Root-specific excludes (added to global_excludes, not replacing)
- `roots[].max_depth`: How deep to scan (default: 4)
- `editor`: Editor command (default: `$EDITOR` or `vi`)
- `llm`: Optional LLM configuration for metadata enrichment

### WSL Support (Windows)

```yaml
roots:
  - name: windows-dev
    path: C:\Users\username\dev
    max_depth: 3

  - name: wsl-ubuntu
    path: /home/username/dev
    max_depth: 4
    wsl: true
    wsl_distro: Ubuntu
```

## Metadata

### Per-Repository Metadata

Create `.rogmeta.yml` in any repository:

```yaml
description: "A fast API server for webhook processing"
tags:
  - go
  - rest-api
  - webhooks
primary_language: Go
```

### Global Metadata

Edit `~/.config/rog/meta.yml`:

```yaml
repos:
  - root: dev
    path: tools/legacy-app
    description: "Legacy Java batch processor"
    tags:
      - java
      - legacy
      - batch
```

### Metadata Precedence

1. `.rogmeta.yml` (manual, highest priority)
2. Global `meta.yml` (manual)
3. LLM-generated
4. Auto-detected

## Examples

### Shell Integration

Add to `.bashrc` or `.zshrc`:

```bash
# Quick jump to repo
alias r='cd "$(rog select)"'

# Open repo in editor
alias re='rog open "$(rog select)"'

# Show dirty repos
alias rd='rog list --dirty'

# Recently worked on
alias rr='rog list --sort last-commit --limit 10'
```

### Batch Operations

```bash
# Pull all repos
rog list --format json | jq -r '.[] | .abs_path' | while read repo; do
  git -C "$repo" pull
done

# Check status of dirty repos
for repo in $(rog list --dirty --format json | jq -r '.[] | .abs_path'); do
  echo "=== $repo ==="
  git -C "$repo" status
done
```

### Finding Repos

```bash
# Find all Go CLI tools
rog list --lang go --tag cli

# Find repos with uncommitted changes
rog list --dirty

# Find recently updated repos
rog list --sort last-commit --limit 10

# Find repos behind remote
rog scan --remote
rog list --behind
```

## Performance

| Operation | Target | Typical |
|-----------|--------|---------|
| `rog list` | < 100ms | ~20ms |
| `rog info` | < 100ms | ~10ms |
| `rog scan` (with fd*) | < 5s/500 repos | ~2-3s |
| `rog scan` (no fd) | < 10s/500 repos | ~8s |
| `rog scan --remote` | < 30s/500 repos | ~15s |

\* Install `fd` for 10-30x faster scanning: `brew install fd` or `cargo install fd-find`

## Documentation

- [User Guide](docs/user-guide.md) - Comprehensive usage guide
- [Architecture](docs/architecture.md) - Technical implementation details
- [WSL Support](docs/wsl-support.md) - Windows Subsystem for Linux integration

## Philosophy

**rog** is designed around these principles:

1. **Fast & Predictable**: Index-driven operations, no surprise network calls
2. **Local-First**: Everything works offline except explicit `--remote` and `--llm` flags
3. **Clean UX**: Simple commands over complex DSLs
4. **Never Touch Repos**: Read-only unless you explicitly create metadata files
5. **Scriptable**: First-class support for automation and shell integration

## Contributing

Contributions welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License - see [LICENSE](LICENSE) for details

## Acknowledgments

Inspired by tools like `fd`, `rg`, `fzf`, and various project management CLIs. Built with:

- [cobra](https://github.com/spf13/cobra) - CLI framework
- [yaml.v3](https://gopkg.in/yaml.v3) - YAML parsing
- Go standard library - Everything else

---

**rog** - navigate your code with context
