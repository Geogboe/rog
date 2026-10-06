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

# 2. Walk through repository discovery and configure roots
rog setup

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

### `rog setup`

Run a terminal wizard with three main steps. Its opening notice shows the actual
config destination (`ROG_CONFIG`, then `XDG_CONFIG_HOME/rog/config.yml`, otherwise
`~/.config/rog/config.yml`). Nothing is saved until the final confirmation.

1. **Include WSL?** Windows users with registered WSL distributions select which
   distributions to include. Discovery may start selected distributions.
   Deselecting a distribution removes its roots from the draft. This step is
   skipped outside Windows or when no distributions are available.
2. **Which project roots should rog use?** Existing roots and depths are
   preloaded. Add or edit a path with `a` or `e`, select/remove a root with Space,
   and adjust its depth with `+`/`-`. New manual roots start at depth 4; depth 1
   finds `dev/repo`, depth 2 also finds `dev/team/repo`. During path entry, Tab
   chooses the native environment or a selected WSL distribution. Windows drive
   paths entered from WSL use the Windows worker. Press `d` to discover roots,
   either instead of manual entry or alongside it. Review the recommended paths,
   owning environments, depths, and repository-location counts before proceeding.
3. **Save these settings?** Review roots before/after, warnings, the destination,
   and the backup path. Press `b` to edit or `y` to save. If the config changed
   while setup was open, review the refreshed proposal and confirm again.

Discovery only finds `.git` directories and worktree marker files. It does not
run Git commands, read Git metadata, or update the index. Findings are repository
locations pending validation, and depths are derived from their locations.
Discovery searches fixed local drives on Windows or the local filesystem on
Linux/WSL, plus selected WSL distributions. Directory symlinks, OS-managed
locations, caches, and configured exclusions are skipped. Unavailable workers
and permission failures appear as partial-discovery warnings. Workers must
support filesystem-only discovery; older workers are rejected rather than used
for Git-based discovery. Setup does not install workers.

All choices stay in memory until saving. Ctrl+C cancels setup without writing
config, backup history, or index. Saving backs up the existing config in
`setup-history/`, retains five revisions, writes atomically, and verifies the
result. Backups use restrictive permissions. Existing exclusions, report
settings, unrelated YAML, and unknown keys on retained roots are preserved.
`rog setup --rollback` previews and restores previous configurations.

After saving, **Run a full project scan now?** offers the equivalent of a local
`rog scan --full`: validate repositories, collect Git metadata/status, and build
or refresh the index. Press `s` or `y` to scan, or `n` or `r` to finish. Declining,
interrupting, or failing this scan leaves the saved config intact. Each native
installation maintains its own config and index. A noninteractive terminal can
use `rog init` for a starter configuration, then run setup in a terminal.
Developer review and operator verification guidance is in [setup QA](setup-qa.md).

### `rog scan`

Scan configured roots for Git repositories and update the index.

```bash
# Fast scan (local only): discover new repos and reuse indexed metadata
rog scan

# Refresh Git branch, commit, status, and repository metadata
rog scan --full

# Include remote status (slower, requires network)
rog scan --remote

# Use LLM to generate descriptions/tags
rog scan --llm

# Refresh existing LLM-generated metadata
rog scan --llm --refresh-meta

# Force plain progress output
rog scan --progress plain

# Separate discovery, Git, metadata, WSL transport, and index timings
rog scan --timings
```

**What it does:**
- Discovers all Git repositories in configured roots
- Reuses indexed Git and repository metadata for known repositories; values such as dirty/clean state may be stale until a full scan
- Extracts Git metadata, language, and `.rogmeta.yml` metadata for new repositories
- Rechecks previously rejected `.git` markers when they change or after 24 hours; `--full` retries them immediately
- With `--full`, refreshes those fields for every repository; each working-tree status check has a 3-second limit, and a timed-out check is shown as `status unknown` and excluded from `--clean` and `--dirty` results
- Optionally calls LLM to enrich missing metadata

On Windows, rog launches one bundled Linux scanner process per configured WSL distro. Discovery and Git inspection run inside Linux; indexed paths remain `\\wsl$\...`. The first install prompts for the distro, exact cache path, and intended changes. Declining or running without a terminal skips that WSL root and preserves its indexed entries. Use `--approve-wsl-worker-install` for explicit unattended approval. `--dry-run` never installs a worker. The cache is `${XDG_CACHE_HOME:-$HOME/.cache}/rog/workers/<build-id>/` inside the distro. A matching cached worker launches without prompting. No `fd` or `fdfind` binary is needed.

In WSL, set `windows: true` on roots with `C:\...` paths to have the installed Windows `rog.exe` scan them on Windows. Keep `rog.exe` on WSL's `PATH`. A failed or missing Windows worker marks the scan incomplete and preserves the previous index entries. WSL navigation returns `/mnt/c/...` paths; reporting asks the Windows worker to read Git. Native WSL roots on DrvFs produce a scan warning unless `scan.suppress_mount_warnings: true` is configured.

**Performance:** Scan time depends on the number and location of configured roots. `--full` also checks every repository's Git state and can be substantially slower. Progress reports discovered Git markers, refreshed/reused repositories, the active name, and elapsed time. The final summary reports the actual number of indexed repositories. A scan with skipped or failed roots exits with code 2, retains their index entries, and prints a reason. Cancelled scans leave the previous index intact.

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
rog list --output json
rog list -o yaml
rog list -o path
```

**Output columns:**
- **NAME**: Repository name
- **LANG**: Primary language
- **BRANCH**: Current branch
- **STATUS**: Local Git state and ahead/behind counts
- **PATH**: Root and relative path

The default table uses color on supported terminals. `NO_COLOR` disables it, and
redirected output has no color codes. Narrow terminals show fewer columns;
`--long` and `--fields` expose the full field set. Use `-o json` or `-o yaml`
for structured output, or `-o path` for one absolute path per line.

### `rog report`

Review work in repositories already indexed under your **Configured Roots**. Run
`rog scan` after adding a repository or changing roots. The report shows its
index timestamp and warns when evidence is incomplete. By default it covers
Monday 00:00 through now in your local timezone.

```bash
rog report                         # Weekly, Dashboard, Log, and AI tabs in a terminal
rog report --since 2026-09-01 --until 2026-09-28 -o markdown
rog report -o json > work.json
rog report -o html --file work.html --open
rog report --open .             # Save a new HTML report here and open it
rog report --open               # Save to a temporary HTML file and open it
rog report --llm -o html --file work-with-ai.html
rog report --explain rog --since 2026-09-01
```

Redirected output defaults to Markdown. `--since` is inclusive; a date-only
`--until` includes that whole day. HTML is self-contained and has print styling
for browser Print to PDF. Weekly and AI Summary, when generated, appear in the
printed copy. In the terminal, Tab or arrow keys switch views, Up/Down scroll,
and `g` on AI Summary asks before sending evidence to the configured provider.
The optional path after `--open` is an existing output directory or an `.html`
filename; it does not limit the report to that repository.

Rog reads commits reachable from all local branches and indexed worktree HEADs,
including branches that are not checked out. Separate clones remain separate
projects. It matches each repository's effective Git `user.email`; use repeated
`--author-email` values or `report.author_emails` in config when needed. Current
working-tree changes are listed separately because they have no reliable author
or historical date. Active days, commit counts, changed paths, and line counts
describe observed Git activity; they do not measure hours worked. Merge commits
appear in the Log but their diff volume is excluded from totals. Remote-only
branches, deleted branches, and stashes are not included.

Unlike `rog list`, reporting checks Git history and working trees live. A large
set of Configured Roots can take minutes; progress goes to stderr. Interactive
progress updates one line with the phase, elapsed time, project count, and a
recent repository. Use `--progress plain` for occasional log lines or
`--progress off` to suppress progress. Incomplete reports keep the readable
evidence, include warnings, and exit with code 2.
Ctrl+C exits with code 130. JSON contains the full warning list; the human
views show a short warning summary.
The Dashboard also shows eligible indexed repositories, grouped projects,
projects with matching commits or current edits, and unavailable projects. Use
`--explain <project-or-path>` to see whether a project is indexed, filtered by
the date or author email, outside a Configured Root, beyond its scan depth, or
excluded. This command gathers the report evidence before explaining it, so it
has the same collection cost as a normal report.

AI Summary is opt-in (`--llm` or the terminal Generate action). Rog sends bounded
commit metadata and selected committed patch hunks to the configured endpoint;
uncommitted patch text is never sent. Sensitive paths and recognizable secret
lines are excluded, but review the endpoint and privacy implications before
using a remote provider. The request has a 96 KiB evidence cap, a 1,200-token
output cap, and a 45-second deadline. Optional `report.input_usd_per_million`
and `report.output_usd_per_million` config values show a rough cost estimate,
not a billing guarantee. A failed AI request leaves the factual report usable.
Windows reports use the WSL worker for indexed WSL roots. A new worker build
requires the same explicit cache-install approval as `rog scan`; unattended runs
may use `--approve-wsl-worker-install`.

For an OpenAI-compatible local server, set `llm.endpoint` to its `/v1` base
URL and `llm.model` to the exact model ID returned by its models endpoint:

```yaml
llm:
  endpoint: http://127.0.0.1:11434/v1
  model: your-local-model-id
```

Then run `rog report --llm -o markdown` or open `rog report`, switch to AI
Summary, and press `g`. A plain `rog report` never contacts the model. Windows
rog uses the Windows endpoint; rog launched directly in WSL uses the WSL
endpoint. Each environment loads its own rog config file. If the local server
requires a key, provide it through `ROG_LLM_API_KEY` rather than saving it in
the config file.

### `rog select` / `rog sel`

Interactively select a repository with rog's built-in picker. It shows name, root/path, language, status, and details. The active row, matching letters, and Git status use restrained color on supported terminals; `NO_COLOR` disables it. Keyboard hints shorten in narrow terminals. Type to filter; use arrows or Page Up/Down to move, Enter to select, and Escape or Ctrl+C to cancel. The picker writes only the chosen path to stdout and draws on the terminal, so command substitution works. If multiple results require a picker but no terminal is available, rog prints an actionable error.

```bash
# Select from all repos
rog select

# Select with filters (same as list)
rog select --lang go --tag cli

# Use in scripts
cd "$(rog select)"
code "$(rog select api)"
```

**Requirements:** An interactive terminal for multiple matches. Neither `fzf` nor `fd` is required.

### `rog info`

Show detailed information about a repository.

In a supported terminal, labels are muted and the repository name and Git status are colored. Redirected output and `NO_COLOR` output stay plain.

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
- **windows** (optional): Set to `true` in WSL for a Windows drive-absolute root; requires matching `rog.exe` on WSL's `PATH`

#### `editor` (optional)

Default editor command. Can be overridden by `ROG_EDITOR` or `EDITOR` env vars.

#### `scan.progress` (optional)

`scan.suppress_mount_warnings: true` hides the warning for native WSL roots on Windows DrvFs mounts.

Controls scan progress rendering.

- `auto`: use richer interactive progress when supported, otherwise fall back to plain output
- `off`: disable progress updates
- `plain`: static ASCII line-based progress
- `rich`: interactive progress with optional ANSI color; briefly shows a recently processed repository name during scans

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

| Operation | Goal |
|-----------|------|
| `rog list`, `rog info` | < 100ms |
| `rog scan` (100 repos) | < 2s |

Scan time depends on filesystem speed and repository size. Large trees on mounted Windows filesystems can take much longer; the scan goal above is not a measured guarantee.

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
rog list --dirty --output json | jq '.[] | .abs_path'

# Get repos by language
rog list --lang python --output json | jq '.[] | .name'

# Export all metadata
rog list --output yaml > repos.yaml
```

### Batch Operations

```bash
# Pull all repos
rog list --output json | jq -r '.[] | .abs_path' | while read repo; do
  echo "Pulling $repo"
  git -C "$repo" pull
done

# Check status of all dirty repos
for repo in $(rog list --dirty --output json | jq -r '.[] | .abs_path'); do
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
