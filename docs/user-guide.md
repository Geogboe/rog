# rog User Guide

## Introduction

**rog** is a fast, local-first Git repository navigator that helps you find, understand, and manage all your projects. It builds an index of your repositories and provides powerful search, filtering, and navigation capabilities.

## Philosophy

- **Fast**: Index-driven operations complete in milliseconds
- **Predictable**: No surprise network calls unless explicitly requested
- **Local-first**: Everything works offline except `--remote` and `--llm` flags
- **Never touches your repos**: Read-only unless you explicitly ask otherwise

## Installation

```bash
# Build from source
git clone https://github.com/Geogboe/rog
cd rog
go build -o rog .
sudo mv rog /usr/local/bin/

# Or use go install
go install github.com/Geogboe/rog@latest
```

## Quick Start

```bash
# 1. Initialize configuration
rog init

# 2. Edit config to add your project roots
vi ~/.config/rog/config.yml

# 3. Scan your repositories
rog scan

# 3a. Use richer progress output when you want it
rog scan --progress rich

# 4. List all repositories
rog list

# 5. Get info about a specific repo
rog info myproject

# 6. Jump to a repo
cd "$(rog path myproject)"
```

## Commands

### `rog init`

Initialize rog by creating a default configuration file at `~/.config/rog/config.yml`.

```bash
rog init
```

This creates a basic config that you should customize with your actual project directories.

### `rog scan`

Scan configured roots for Git repositories and update the index.

```bash
# Basic scan (local only, fast)
rog scan

# Include remote status (slower, requires network)
rog scan --remote

# Use LLM to generate descriptions/tags
rog scan --llm

# Refresh existing LLM-generated metadata
rog scan --llm --refresh-meta

# Force plain progress output
rog scan --progress plain
```

**What it does:**
- Discovers all Git repositories in configured roots
- Extracts Git metadata (branch, commits, status)
- Detects primary programming language
- Reads `.rogmeta.yml` files for manual metadata
- Optionally calls LLM to enrich missing metadata

**Performance:** Typically < 2s for hundreds of repos (local-only)

### `rog list`

List repositories with fuzzy search and filtering.

```bash
# List all repositories
rog list

# Fuzzy search
rog list api
rog list search backend

# Filter by language
rog list --lang go
rog list --lang python --lang rust

# Filter by tags
rog list --tag cli
rog list --tag web --tag rest

# Filter by branch
rog list --branch main

# Filter by status
rog list --dirty           # Uncommitted changes
rog list --clean           # No changes
rog list --ahead           # Ahead of remote
rog list --behind          # Behind remote

# Sort results
rog list --sort last-commit
rog list --sort path
rog list --sort last-scan

# Limit results
rog list --limit 10

# Detailed output
rog list --long

# Machine-readable output
rog list --format json
rog list --format yaml
```

**Output columns:**
- **NAME**: Repository name
- **LANG**: Primary language
- **HOST**: Git host (github.com, etc.)
- **BRANCH**: Current branch
- **STATUS**: Remote status + local status
- **LAST COMMIT**: Time since last commit
- **ROOT**: Which root it belongs to
- **PATH**: Relative path within root

### `rog select` / `rog sel`

Interactively select a repository using fzf (if available).

```bash
# Select from all repos
rog select

# Select with filters (same as list)
rog select --lang go --tag cli

# Use in scripts
cd "$(rog select)"
code "$(rog select api)"
```

**Requirements:** `fzf` for interactive selection (falls back to plain list if not installed)

### `rog info`

Show detailed information about a repository.

```bash
# By name
rog info myproject

# By fuzzy search
rog info api

# By absolute path
rog info /home/user/projects/myproject
```

**Output includes:**
- Name, description, tags
- Full path, root, relative path
- Remote URL and host
- Branch and status
- Last commit info
- Primary language
- Scan timestamps

### `rog path`

Print the absolute path of a repository (for scripting).

```bash
# Jump to repository
cd "$(rog path myproject)"

# Open in editor
code "$(rog path api)"

# Run commands in repo
git -C "$(rog path backend)" pull
```

### `rog open`

Open a repository in your configured editor.

```bash
rog open myproject
```

**Editor resolution:**
1. `ROG_EDITOR` environment variable
2. `editor` field in config.yml
3. `EDITOR` environment variable
4. `vi` (fallback)

### `rog meta`

Manage repository metadata.

```bash
# Create .rogmeta.yml in current directory
cd /path/to/repo
rog meta init

# Edit .rogmeta.yml
rog meta edit

# Initialize global metadata file
rog meta init --global

# Edit global metadata file
rog meta edit --global
```

## Configuration

Configuration file: `~/.config/rog/config.yml`

### Basic Example

```yaml
roots:
  - name: dev
    path: ~/dev
    max_depth: 4
    exclude:
      - node_modules
      - vendor

  - name: work
    path: ~/work
    max_depth: 5

editor: code

scan:
  progress: auto

llm:
  endpoint: http://localhost:11434/v1
  model: codellama
  extra_instructions: "Focus on domain and technology tags."
```

### Configuration Fields

#### `roots` (required)

List of directories to scan for repositories.

- **name**: Logical name for this root (shown in `rog list`)
- **path**: Absolute or home-relative path (`~/dev`)
- **max_depth**: How deep to recurse (default: 4)
- **exclude**: Directory names to skip (e.g., `node_modules`)
- **wsl** (optional): Set to `true` for WSL roots (Windows only)
- **wsl_distro** (optional): WSL distro name (e.g., `Ubuntu`)

#### `editor` (optional)

Default editor command. Can be overridden by `ROG_EDITOR` or `EDITOR` env vars.

#### `scan.progress` (optional)

Controls scan progress rendering.

- `auto`: use richer interactive progress when supported, otherwise fall back to plain output
- `off`: disable progress updates
- `plain`: static ASCII line-based progress
- `rich`: interactive progress with optional ANSI color

#### `llm` (optional)

LLM configuration for enriching metadata.

- **endpoint**: OpenAI-compatible API endpoint
- **model**: Model name
- **api_key** (optional): API key (or use `ROG_LLM_API_KEY`)
- **extra_instructions** (optional): Additional prompt instructions

### Environment Variables

Override configuration values:

- `ROG_CONFIG`: Path to config file
- `ROG_DATA`: Path to data directory (index.json location)
- `ROG_EDITOR`: Editor command
- `ROG_PROGRESS`: Scan progress mode (`auto`, `off`, `plain`, `rich`)
- `ROG_LLM_ENDPOINT`: LLM API endpoint
- `ROG_LLM_MODEL`: LLM model name
- `ROG_LLM_API_KEY`: LLM API key
- `ROG_LLM_EXTRA`: Extra LLM instructions

Progress mode precedence is:

1. `rog scan --progress <mode>`
2. `ROG_PROGRESS`
3. `scan.progress` in config
4. Default: `auto`

## Metadata

### Metadata Precedence

When rog determines metadata (description, tags, language), it uses this priority:

1. **Manual** (`.rogmeta.yml` in repo) - highest priority
2. **Global** (`~/.config/rog/meta.yml`)
3. **LLM-generated** (`rog scan --llm`)
4. **Auto-detected** (file-based language detection)

### Per-Repository Metadata (`.rogmeta.yml`)

Create in any repository:

```yaml
name: my-custom-name
description: "A fast API server for processing webhooks"
tags:
  - go
  - rest-api
  - webhooks
primary_language: Go
```

This metadata has highest priority and won't be overwritten by LLM.

### Global Metadata (`~/.config/rog/meta.yml`)

For repos you can't or don't want to modify:

```yaml
repos:
  - root: dev
    path: tools/legacy-app
    description: "Legacy Java application for data processing"
    tags:
      - java
      - legacy
      - batch-processing

  - root: work
    path: clients/acme/backend
    description: "ACME Corp backend API"
    tags:
      - python
      - django
      - rest-api
```

**Note:** `root` and `path` must exactly match the values in the index.

### LLM Enrichment

Generate descriptions and tags automatically:

```bash
# Generate for repos missing metadata
rog scan --llm

# Regenerate LLM metadata (keeps manual/global metadata)
rog scan --llm --refresh-meta
```

**Requirements:**
- OpenAI-compatible LLM API (OpenAI, Ollama, LocalAI, etc.)
- Configured `llm` section in config.yml

**What it does:**
- Reads README files (first 500 chars)
- Analyzes top-level directory structure
- Generates concise description (<= 140 chars)
- Generates 3-7 relevant tags

**Tag guidelines:**
- Lowercase with hyphens (`rest-api`, not `REST API`)
- Focus on: language, domain, type, technology
- Avoid: repo name, versions, overly generic terms

## Search and Filtering

### Fuzzy Search

Search terms match against:
- Repository name
- Description
- Tags
- Path
- Remote URL

All search terms must match (AND logic):

```bash
# Both "api" AND "search" must match
rog list api search
```

### Exact Filters

Flags provide exact matching:

```bash
# Language must be exactly "Go"
rog list --lang go

# All specified tags must be present
rog list --tag cli --tag rest

# Branch must match exactly
rog list --branch main
```

### Combining Search and Filters

```bash
# Fuzzy "api" + must be Python + must have "rest" tag
rog list api --lang python --tag rest

# Fuzzy "search" + must be dirty + sort by last commit
rog list search --dirty --sort last-commit
```

## Workflows

### Daily Development

```bash
# Check what's dirty
rog list --dirty

# Find that API project
cd "$(rog select api)"

# Open recent project
rog list --sort last-commit --limit 5
rog open my-project
```

### Code Archaeology

```bash
# Find all Go CLI tools
rog list --lang go --tag cli

# Find repos you haven't touched in a while
rog list --sort last-commit | tail -20

# Find repos with specific technology
rog list postgres
rog list --tag docker
```

### Maintenance

```bash
# Update remote status
rog scan --remote

# See what's behind
rog list --behind

# Enrich metadata for new repos
rog scan --llm
```

## Performance

| Operation | Target | Typical |
|-----------|--------|---------|
| `rog list` | < 100ms | ~20ms |
| `rog info` | < 100ms | ~10ms |
| `rog scan` (100 repos) | < 2s | ~1.5s |
| `rog scan --remote` | < 10s | ~5s |
| `rog scan --llm` (100 repos) | < 30s | ~20s |

**Tips for speed:**
- Run `rog scan` periodically (not every time)
- Use `--remote` only when you need remote status
- Use `--llm` only for new repos or when updating metadata

## Tips and Tricks

### Shell Integration

Add to your `.bashrc` or `.zshrc`:

```bash
# Quickly jump to repos
alias r='cd "$(rog select)"'

# Open repo in editor
alias re='rog open "$(rog select)"'

# List dirty repos
alias rd='rog list --dirty'

# Recently worked on
alias rr='rog list --sort last-commit --limit 10'
```

### Editor Integration (VS Code)

```bash
# Add to config.yml
editor: code

# Or use environment variable
export ROG_EDITOR="code"

# Then use
rog open myproject
```

### Periodic Scanning

Add to crontab:

```bash
# Scan every hour
0 * * * * /usr/local/bin/rog scan

# Scan with remote check every 6 hours
0 */6 * * * /usr/local/bin/rog scan --remote
```

### Finding Archived Projects

```bash
# Find repos not touched in over a year
rog list --sort last-commit | grep "1y ago"

# Find repos behind remote (might be abandoned)
rog list --behind
```

## Troubleshooting

### "No repositories found"

- Run `rog scan` first
- Check config paths: `cat ~/.config/rog/config.yml`
- Ensure paths are absolute or use `~/...` format

### "LLM enrichment failed"

- Check LLM endpoint is running
- Verify API key in config or environment
- Test endpoint: `curl http://localhost:11434/v1/models`

### Slow scanning

- Reduce `max_depth` in roots
- Add more exclusions (`.git` is auto-excluded)
- Exclude large directories: `node_modules`, `vendor`, `target`

### Wrong language detected

- Add manual override in `.rogmeta.yml`:
  ```yaml
  primary_language: Go
  ```

### Repositories not showing up

- Check if directory has `.git` folder
- Verify path is within max_depth
- Check if directory name is in exclude list

## Advanced Usage

### Scripting with JSON Output

```bash
# Get all dirty repos as JSON
rog list --dirty --format json | jq '.[] | .abs_path'

# Get repos by language
rog list --lang python --format json | jq '.[] | .name'

# Export all metadata
rog list --format yaml > repos.yaml
```

### Batch Operations

```bash
# Pull all repos
rog list --format json | jq -r '.[] | .abs_path' | while read repo; do
  echo "Pulling $repo"
  git -C "$repo" pull
done

# Check status of all dirty repos
for repo in $(rog list --dirty --format json | jq -r '.[] | .abs_path'); do
  echo "=== $repo ==="
  git -C "$repo" status
done
```

## WSL Support (Windows)

See [WSL Support Documentation](wsl-support.md) for details on using rog with Windows Subsystem for Linux.

Quick example:

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

## FAQ

**Q: Does rog modify my repositories?**
A: No. rog is read-only except when you explicitly create `.rogmeta.yml` files.

**Q: Do I need to run rog scan frequently?**
A: No. Run it when you add new repos or want updated status. The index persists.

**Q: Can I use rog without LLM?**
A: Yes! LLM is completely optional. Manual and global metadata work fine.

**Q: What if I have thousands of repositories?**
A: rog uses in-memory indexing and should handle thousands easily. If performance degrades, consider splitting into multiple roots or reducing max_depth.

**Q: Can I exclude specific repositories?**
A: Not directly, but you can use directory exclusions. Or just ignore them in queries.

**Q: Does rog work on Mac/Linux/Windows?**
A: Yes, cross-platform. WSL features are Windows-only.

## See Also

- [Architecture Documentation](architecture.md) - Technical details
- [WSL Support](wsl-support.md) - Windows Subsystem for Linux integration
- [GitHub Repository](https://github.com/Geogboe/rog) - Source code and issues
