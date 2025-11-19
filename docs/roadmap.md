# rog Roadmap

## Overview

This document outlines the development roadmap for `rog`. The project follows an iterative approach with clearly defined milestones. Each version maintains the core philosophy: fast, local-first, predictable.

---

## Version Strategy

### Versioning Scheme

- **Major** (X.0.0): Breaking changes to CLI, config format, or index structure
- **Minor** (0.X.0): New features, backward-compatible changes
- **Patch** (0.0.X): Bug fixes, performance improvements, documentation

### Release Philosophy

- **Fast iterations**: Ship early, ship often
- **Stability first**: No breaking changes without migration path
- **User feedback driven**: Features prioritized by real-world usage

---

## V1.0 - Core Foundation 

**Goal**: Fast, local-first Git repository discovery and navigation

**Status**: Complete (current version)

### Features Delivered

#### Scanning & Indexing
-  Filesystem walking with max depth
-  Git repository detection
-  Parallel scanning (worker pool)
-  Parallel root scanning
-  Git metadata extraction (branch, commits, status)
-  Language detection (marker files + extension counting)
-  Exclude directories (exact match + glob patterns)
-  Global excludes config

#### Querying & Filtering
-  Filter by language, tags, branch, root
-  Search by name, path, description
-  Dirty/clean status filtering
-  Sort by name, time, path

#### Metadata
-  Manual metadata (`.rogmeta.yml`)
-  Global metadata (`meta.yml`)
-  README description extraction
-  LLM enrichment (opt-in)
-  Source tracking (manual/global/llm/auto)

#### CLI Commands
-  `rog init` - Initialize configuration
-  `rog scan` - Scan for repositories
-  `rog list` - List repositories
-  `rog select` - Interactive selection (with `fzf`)
-  `rog info` - Show repository details
-  `rog path` - Print repository path
-  `rog open` - Open in editor
-  `rog meta init/edit` - Manage metadata
-  `rog config --validate` - Validate configuration

#### Performance
-  < 100ms for index queries
-  < 2s for scanning typical workspaces
-  In-memory index
-  Atomic file operations

#### Testing
-  96+ unit tests
-  Integration tests
-  Performance test harness
-  CI/CD pipeline

---

## V1.1 - Polish & Performance =§

**Goal**: Address early user feedback, optimize performance

**Status**: In Progress

**Target**: Q1 2025

### Planned Features

#### Performance Improvements
- ó Benchmark and optimize hot paths
- ó Profile with pprof, address bottlenecks
- ó Optimize glob pattern matching
- ó Cache language detection results

#### UX Improvements
- [ ] Better error messages
- [ ] Progress bars for long scans
- [ ] `rog doctor` command (check setup, debug issues)
- [ ] `rog stats` command (index statistics, coverage)

#### Metadata Enhancements
- [ ] Bulk metadata import/export
- [ ] Metadata templates
- [ ] Auto-tagging rules (e.g., "backend" if has `server/`)

#### CLI Polish
- [ ] Shell completions (bash, zsh, fish)
- [ ] Color themes
- [ ] Customizable list formats

---

## V1.2 - Developer Experience

**Goal**: Make `rog` indispensable for daily workflows

**Target**: Q2 2025

### Planned Features

#### Workflow Integration
- [ ] `rog exec <cmd>` - Run command in all/filtered repos
- [ ] `rog sync` - Git pull in all/filtered repos (with `--remote`)
- [ ] `rog status` - Show git status across all repos
- [ ] `rog recent` - List recently modified repos

#### Smart Filtering
- [ ] Filter by commit age
- [ ] Filter by remote host (GitHub, GitLab, etc.)
- [ ] Filter by file types (has Dockerfile, has tests)
- [ ] Saved filters (aliases for complex queries)

#### Workspace Support
- [ ] `rog workspace create/list/switch` - Logical groupings of repos
- [ ] Workspace-specific excludes and settings
- [ ] Multi-workspace scanning

---

## V2.0 - Incremental & Real-Time

**Goal**: Eliminate slow full scans, enable real-time updates

**Target**: Q3 2025

### Breaking Changes

- **Index format**: Migrate from JSON to SQLite
  - **Why**: Better query performance, schema migrations, partial updates
  - **Migration**: Auto-migrate on first run of V2.0

- **Config schema**: Add index version tracking
  - **Migration**: Auto-upgrade config on load

### Major Features

#### Incremental Scanning
- [ ] Track last scan time per directory
- [ ] Only re-scan changed directories
- [ ] Detect new/removed repos without full walk

#### Filesystem Watching
- [ ] Optional watch mode: `rog watch`
- [ ] Auto-update index on filesystem changes
- [ ] Configurable watch debouncing

#### Performance Targets
- [ ] Incremental scan: < 500ms (from ~2s full scan)
- [ ] Watch mode: < 100ms latency
- [ ] Support 10k+ repos

#### Advanced Querying
- [ ] Full-text search (not just substring)
- [ ] Query syntax: `lang:go AND tag:api NOT dirty:true`
- [ ] Fuzzy matching on all fields

---

## V2.1 - Team Collaboration

**Goal**: Enable team-wide metadata sharing

**Target**: Q4 2025

### Features

#### Remote Index Sharing
- [ ] Export index to JSON/Git
- [ ] Import index from URL/Git repo
- [ ] Merge multiple indexes (union of repos)
- [ ] Conflict resolution (manual > imported)

#### Team Metadata
- [ ] Shared `team-meta.yml` (versioned in Git)
- [ ] Override precedence: local > team > global
- [ ] Metadata sync command

#### CI/CD Integration
- [ ] `rog check` - Validate metadata in CI
- [ ] `rog diff` - Compare index states
- [ ] Exit codes for scripting

---

## V3.0 - Extensibility

**Goal**: Plugin system for custom workflows

**Target**: 2026

### Breaking Changes

- **Plugin API**: Introduce stable plugin interface
- **Config schema**: Add plugins section

### Major Features

#### Plugin System
- [ ] Plugin hooks: pre-scan, post-scan, filter, action
- [ ] Plugin discovery (from `~/.config/rog/plugins/`)
- [ ] Plugin API (Go interface + RPC)
- [ ] Example plugins: Jira integration, Slack notifications, custom linters

#### Custom Actions
- [ ] `rog action run <name>` - Execute user-defined actions
- [ ] Action templates (scripts with repo context)
- [ ] Action history and undo

#### Web UI (Optional)
- [ ] Local web server: `rog serve`
- [ ] Browser-based repo explorer
- [ ] Metadata editor GUI
- [ ] Team dashboard (if using remote index)

---

## Long-Term Ideas (No ETA)

### Potential Features

These are ideas worth exploring, but not yet scheduled:

1. **Multi-VCS Support**
   - Detect SVN, Mercurial, Fossil repositories
   - Unified interface across VCS types

2. **AI-Powered Features**
   - Auto-generate README based on code
   - Detect similar/duplicate repos
   - Recommend repos based on current context

3. **IDE Integrations**
   - VSCode extension
   - Vim/Neovim plugin
   - JetBrains plugin

4. **Mobile Companion**
   - iOS/Android app for browsing repos
   - Trigger actions remotely (e.g., "git pull all")

5. **Cloud Sync** (Controversial)
   - Sync index across machines
   - Trade-off: complexity vs convenience
   - Would need to maintain local-first philosophy

---

## Non-Goals

Features we've considered and rejected:

### L Full Git Client
**Why**: Too much scope. Use `git` or `gh` directly.

### L Project Templates
**Why**: Many tools do this well (`cookiecutter`, `degit`). Stick to discovery.

### L Dependency Graphing
**Why**: Complex, language-specific. Use dedicated tools.

### L Code Search
**Why**: `ripgrep` exists. Don't reinvent perfection.

### L Container Integration
**Why**: Out of scope. Use `docker` or `podman`.

### L Cloud-Only Features
**Why**: Violates local-first principle.

---

## Deprecation Policy

When deprecating features:

1. **Announce** in release notes (1 version early)
2. **Warn** at runtime (2 versions)
3. **Remove** (3rd version)
4. **Provide migration path** (always)

Example:
- V1.0: Feature X introduced
- V1.5: Announce deprecation of X (still works)
- V1.6: Warn when X is used (still works)
- V2.0: Remove X, provide migration guide

---

## Contribution Priorities

### High Priority

These contributions are always welcome:

- **Performance improvements** (with benchmarks)
- **Bug fixes** (with tests)
- **Documentation** (especially examples)
- **Test coverage** (especially edge cases)

### Medium Priority

These are nice-to-have:

- **New filters** (if generally useful)
- **Output formats** (if requested by users)
- **Platform-specific optimizations** (Windows, macOS, Linux)

### Low Priority

These require discussion first:

- **New commands** (does it fit the philosophy?)
- **New dependencies** (do we really need it?)
- **Breaking changes** (what's the migration path?)

---

## Feedback Loop

### How Roadmap Evolves

1. **User Feedback**: GitHub issues, discussions
2. **Usage Analytics**: (Optional, opt-in) Command usage stats
3. **Performance Metrics**: Benchmark trends over time
4. **Team Retrospectives**: Monthly review of progress

### How to Influence Roadmap

- **File an issue**: Describe your use case
- **Vote on issues**: =M existing proposals
- **Contribute**: PRs are the strongest signal
- **Sponsor**: Financial support prioritizes features

---

## Success Metrics

### V1.0 Goals (Achieved)
-  < 100ms for index queries
-  < 2s for typical scans
-  Zero network by default
-  Works offline completely

### V2.0 Goals
- Incremental scan: < 500ms
- Support 10k+ repos
- Watch mode: < 100ms latency

### V3.0 Goals
- Plugin ecosystem: 10+ community plugins
- 1000+ daily active users
- 90%+ test coverage

---

## Open Questions

### For V1.x

1. **Incremental scanning**: Directory-level timestamps or git-based?
2. **Watch mode**: inotify/fsevents or polling?
3. **SQLite migration**: When is the right time?

### For V2.x

4. **Plugin API**: Go-only or support scripting languages?
5. **Remote index**: Git-based or custom protocol?
6. **Team features**: Self-hosted or SaaS option?

### For V3.x

7. **Web UI**: Electron app or browser-only?
8. **Mobile app**: Native or web-based?
9. **Multi-VCS**: Worth the complexity?

---

## Release Schedule

### V1.1 - Q1 2025
- Performance optimizations
- UX polish
- Developer experience improvements

### V1.2 - Q2 2025
- Workflow integration
- Workspace support

### V2.0 - Q3 2025
- Incremental scanning
- SQLite migration
- Advanced querying

### V2.1 - Q4 2025
- Team collaboration
- Remote index sharing

### V3.0 - 2026
- Plugin system
- Extensibility

---

## Get Involved

- **Report bugs**: https://github.com/Geogboe/rog/issues
- **Request features**: https://github.com/Geogboe/rog/discussions
- **Contribute code**: See CONTRIBUTING.md
- **Join discussions**: Discord/Slack (links TBD)

---

## Conclusion

`rog` is a long-term project with a clear vision: fast, local-first Git repository navigation. This roadmap provides direction while remaining flexible to user feedback and real-world usage patterns.

**Remember**: Every feature must serve the core mission and honor the design philosophy. When in doubt, choose simplicity and performance.
