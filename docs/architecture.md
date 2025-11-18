# rog - Technical Architecture

## Overview

rog is a fast, local-first Git repository navigator built in Go. This document describes the technical architecture, design decisions, and implementation approach.

## Design Principles

1. **Performance First**: Index-driven queries must be < 100ms
2. **Predictability**: No surprise network calls; explicit flags for I/O
3. **Simplicity**: Clean data models, minimal abstractions
4. **Reliability**: Graceful degradation, clear error messages
5. **Zero Side Effects**: Never modify user repositories without explicit action

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         CLI Layer                           │
│  (cobra commands: init, scan, list, select, info, etc.)    │
└────────────┬────────────────────────────────────────────────┘
             │
             ├─────► config     (configuration loading)
             ├─────► scanner    (repo discovery & git introspection)
             ├─────► index      (storage & querying)
             ├─────► query      (search, filter, fuzzy match)
             ├─────► git        (git operations)
             ├─────► llm        (LLM enrichment)
             └─────► metadata   (meta file handling)
```

## Core Components

### 1. Config Package (`internal/config`)

**Responsibilities**:
- Load and parse `config.yml`
- Handle environment variable overrides
- Provide default values
- Validate configuration

**Data Structures**:
```go
type Config struct {
    Roots  []Root
    Editor string
    LLM    *LLMConfig
}

type Root struct {
    Name     string
    Path     string
    MaxDepth int
    Exclude  []string
}

type LLMConfig struct {
    Endpoint         string
    Model            string
    APIKey           string
    ExtraInstructions string
}
```

**Priority**: Environment Variables > config.yml > Defaults

### 2. Index Package (`internal/index`)

**Responsibilities**:
- Load/save index from disk (JSON format)
- CRUD operations on repository entries
- In-memory index for fast queries
- Atomic writes (write temp, rename)

**Data Structures**:
```go
type Index struct {
    Repos      map[string]*Repo  // Key: absolute path
    UpdatedAt  time.Time
}

type Repo struct {
    // Identity
    ID          string    // Hash of absolute path
    Name        string
    Root        string    // Root identifier
    RelPath     string    // Relative to root
    AbsPath     string

    // Git Info
    RemoteURL        string
    Host             string
    CurrentBranch    string
    LastCommitTime   time.Time
    LastCommitAuthor string
    IsDirty          bool
    HasUntracked     bool
    Ahead            int
    Behind           int
    LastGitCheckAt   time.Time

    // Metadata
    PrimaryLanguage string
    Description     string
    Tags            []string

    // Tracking
    FirstSeenAt time.Time
    LastScanAt  time.Time

    // Metadata source tracking
    DescriptionSource string  // "manual", "global", "llm", "auto"
    TagsSource        string
}
```

**File Location**: `~/.config/rog/index.json` (or `$ROG_DATA/index.json`)

### 3. Scanner Package (`internal/scanner`)

**Responsibilities**:
- Walk filesystem roots with max depth
- Detect git repositories (presence of `.git/`)
- Extract git metadata (branch, commits, status)
- Detect primary language
- Read metadata files
- Update index

**Algorithm**:
```
for each root in config:
    walk(root.Path, maxDepth=root.MaxDepth):
        if path.contains(".git/"):
            repo = extractRepoInfo(path)
            checkLanguage(repo)
            readMetadata(repo)
            index.upsert(repo)
            skip descending into this directory
        if path in root.Exclude:
            skip
```

**Concurrency**: Use worker pool to process repos in parallel (limit: NumCPU * 2)

### 4. Git Package (`internal/git`)

**Responsibilities**:
- Get current branch
- Get last commit info (time, author, hash)
- Check working tree status (dirty, untracked)
- Get remote URL
- Compare with remote (ahead/behind) - only with `--remote`

**Implementation Choice**: Shell out to `git` command instead of go-git
- **Rationale**:
  - More reliable (uses user's git config)
  - Simpler implementation
  - Smaller binary size
  - Avoids go-git complexity and edge cases
  - Users already have git installed

**Key Functions**:
```go
func GetBranch(repoPath string) (string, error)
func GetLastCommit(repoPath string) (*CommitInfo, error)
func GetStatus(repoPath string) (*Status, error)
func GetRemoteURL(repoPath string) (string, error)
func GetRemoteStatus(repoPath string) (ahead, behind int, error)
```

### 5. Language Detection (`internal/scanner/language.go`)

**Algorithm** (priority order):

1. **Tool Files** (highest priority):
   - `go.mod` → Go
   - `Cargo.toml` → Rust
   - `package.json` → JavaScript/TypeScript
   - `pom.xml`, `build.gradle` → Java
   - `requirements.txt`, `pyproject.toml` → Python
   - `Gemfile` → Ruby
   - etc.

2. **File Extension Dominance**:
   - Count files by extension (up to 100 files for performance)
   - Weight by depth (shallower = higher weight)
   - Return most dominant language

3. **Fallback**: "unknown"

**Performance**: Limit to scanning top 2 levels, max 100 files

### 6. Metadata Package (`internal/metadata`)

**Responsibilities**:
- Parse `.rogmeta.yml` in repos
- Parse global `meta.yml`
- Merge metadata with precedence rules

**Precedence** (highest to lowest):
1. `.rogmeta.yml` (manual)
2. Global `meta.yml` (manual)
3. LLM-generated
4. Auto-detected

**Data Structures**:
```go
type RepoMeta struct {
    Name            string
    Description     string
    Tags            []string
    PrimaryLanguage string
}

type GlobalMeta struct {
    Repos []GlobalRepoMeta
}

type GlobalRepoMeta struct {
    Root        string
    Path        string
    Description string
    Tags        []string
    PrimaryLanguage string
}
```

### 7. LLM Package (`internal/llm`)

**Responsibilities**:
- Call OpenAI-compatible API
- Generate descriptions and tags
- Parse and validate responses

**Request Format**:
```go
type EnrichRequest struct {
    Name         string
    Root         string
    RelPath      string
    Language     string
    Host         string
    ReadmeSnippet string  // First 500 chars
    TopLevelItems []string
}
```

**Response Format**:
```json
{
  "description": "One sentence description under 140 chars",
  "tags": ["tag1", "tag2", "tag3"]
}
```

**System Prompt** (fixed, non-customizable):
```
You are a code repository analyzer. Your job is to generate concise
metadata for Git repositories.

REQUIRED OUTPUT FORMAT (JSON only, no markdown, no explanations):
{
  "description": "...",
  "tags": [...]
}

DESCRIPTION RULES:
- Exactly one sentence
- Maximum 140 characters
- Describe WHAT the project does, not HOW
- Be specific and technical

TAG RULES:
- Provide 3-7 tags
- All lowercase
- Use hyphens not spaces (e.g., "search-api")
- Categories: language, domain, type, technology
- Avoid: repo name, version numbers, overly generic terms
- Avoid: extremely specific implementation details
```

**User Prompt** (customizable):
```
[Extra instructions from config.llm.extra_instructions]

Repository: {name}
Location: {root}/{relpath}
Language: {language}
Host: {host}

README (first 500 chars):
{readme}

Top-level structure:
{files and dirs}
```

**Error Handling**:
- Timeout: 30s
- Retry: 2 attempts with exponential backoff
- Invalid JSON: Skip repo, log warning
- Rate limiting: Honor 429 responses

### 8. Query Package (`internal/query`)

**Responsibilities**:
- Filter repos by structured criteria (lang, tags, branch, status)
- Fuzzy text search (name, description, tags, path, url)
- Sort results
- Limit results

**Fuzzy Matching**: Use simple substring + scoring approach
- Exact match: highest score
- Prefix match: high score
- Contains match: medium score
- Case-insensitive

**Filter Types**:
```go
type Filter struct {
    SearchTerms []string  // Fuzzy
    Languages   []string  // Exact
    Tags        []string  // Exact (all must match)
    Branch      string    // Exact
    Dirty       *bool
    Ahead       *bool
    Behind      *bool
    Root        string
}

type SortField int
const (
    SortByName SortField = iota
    SortByLastCommit
    SortByPath
    SortByLastScan
)
```

### 9. CLI Commands (`cmd/`)

**Framework**: [spf13/cobra](https://github.com/spf13/cobra)

**Command Structure**:
```
rog
├── init         (cmd/init.go)
├── scan         (cmd/scan.go)
├── list         (cmd/list.go)
├── select       (cmd/select.go)
├── info         (cmd/info.go)
├── path         (cmd/path.go)
├── open         (cmd/open.go)
└── meta         (cmd/meta/)
    ├── init.go
    ├── edit.go
    └── set.go
```

## Key Design Decisions

### 1. Index Storage: JSON vs SQLite

**Decision**: Start with JSON, migrate to SQLite if needed

**Rationale**:
- Simple implementation
- Human-readable for debugging
- Fast enough for < 1000 repos
- Easy atomic writes
- No dependencies

**Future**: If performance becomes an issue (> 10k repos), migrate to SQLite

### 2. Git Operations: go-git vs Shell Out

**Decision**: Shell out to `git` command

**Rationale**:
- Simpler code
- Uses user's git config and credentials
- More reliable for edge cases
- Smaller binary
- Users already have git installed

### 3. Concurrency Model

**Scanning**: Worker pool with bounded concurrency (NumCPU * 2)
- Prevents resource exhaustion
- Good balance of speed and safety

**Index Operations**: Single-threaded (fast enough in-memory)

### 4. Fuzzy Selection: fzf vs Built-in

**Decision**: Shell out to `fzf` if available, fallback to plain list

**Rationale**:
- fzf is widely used and loved
- Don't reinvent superior UX
- Fallback maintains functionality

### 5. Configuration: Viper vs Manual

**Decision**: Manual parsing with gopkg.in/yaml.v3

**Rationale**:
- Simple config structure
- Full control over precedence
- No magic
- Lighter dependency

## Performance Targets

| Operation | Target | Strategy |
|-----------|--------|----------|
| `rog list` | < 50ms | In-memory index, simple filters |
| `rog info` | < 50ms | Hash map lookup |
| `rog scan` (100 repos) | < 2s | Parallel scanning, efficient git calls |
| `rog scan --llm` (100 repos) | < 30s | Parallel LLM calls with rate limiting |
| Index load | < 50ms | JSON unmarshal, keep in memory |

## Error Handling Philosophy

1. **Fail Fast**: Invalid config → immediate error
2. **Graceful Degradation**: Missing git → skip that repo, warn user
3. **Clear Messages**: Tell user what went wrong and how to fix it
4. **No Silent Failures**: Always log warnings for skipped repos

## Security Considerations

1. **No Arbitrary Code Execution**: Never eval user input
2. **Path Traversal**: Validate all paths, resolve symlinks carefully
3. **LLM Injection**: Sanitize repo content before sending to LLM
4. **Credentials**: Never log or store LLM API keys
5. **File Permissions**: Respect filesystem permissions, skip inaccessible repos

## Testing Strategy

### Unit Tests
- Config parsing and validation
- Language detection algorithm
- Metadata precedence logic
- Filter and query logic
- Git command parsing

### Integration Tests
- Scan workflow with mock git repos
- Index persistence (save/load)
- LLM integration with mock server
- Metadata file reading

### E2E Tests
- Full CLI workflows
- Real git repositories (fixtures)
- Edge cases: symlinks, submodules, bare repos

### Performance Tests
- Scan 1000+ repos
- List/filter/search with large index
- Concurrent scanning

## Dependencies

### Core
- `github.com/spf13/cobra` - CLI framework
- `gopkg.in/yaml.v3` - YAML parsing
- Standard library (no frameworks)

### Optional
- `fzf` (external binary) - Interactive selection

### Development
- `github.com/stretchr/testify` - Test assertions
- `github.com/rogpeppe/go-internal/testscript` - CLI testing

## File Structure

```
rog/
├── cmd/
│   ├── root.go          # Root command setup
│   ├── init.go
│   ├── scan.go
│   ├── list.go
│   ├── select.go
│   ├── info.go
│   ├── path.go
│   ├── open.go
│   └── meta/
│       ├── init.go
│       ├── edit.go
│       └── set.go
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   ├── config_test.go
│   │   └── defaults.go
│   ├── index/
│   │   ├── index.go
│   │   ├── index_test.go
│   │   └── repo.go
│   ├── scanner/
│   │   ├── scanner.go
│   │   ├── scanner_test.go
│   │   ├── language.go
│   │   └── language_test.go
│   ├── git/
│   │   ├── git.go
│   │   └── git_test.go
│   ├── llm/
│   │   ├── client.go
│   │   ├── client_test.go
│   │   └── prompt.go
│   ├── metadata/
│   │   ├── metadata.go
│   │   └── metadata_test.go
│   └── query/
│       ├── query.go
│       ├── filter.go
│       └── query_test.go
├── tests/
│   ├── integration/
│   └── e2e/
├── docs/
│   ├── architecture.md  # This file
│   ├── user-guide.md
│   └── development.md
├── main.go
├── go.mod
└── go.sum
```

## Open Questions & Future Considerations

1. **Submodules**: How to handle? Scan separately or treat as part of parent?
   - **Decision**: Treat as separate repos for now

2. **Monorepos**: Should we support multiple projects in one repo?
   - **Decision**: V1 treats entire repo as single unit

3. **Performance**: When to migrate from JSON to SQLite?
   - **Decision**: Monitor feedback, migrate if users have > 1000 repos

4. **Remote Sync**: How often to suggest `--remote`?
   - **Decision**: Warn if LastGitCheckAt > 7 days and user queries ahead/behind

5. **LLM Costs**: Should we batch LLM calls?
   - **Decision**: V1 sequential with concurrency limit, V2 can batch

6. **Cross-platform**: Path handling differences?
   - **Decision**: Use filepath package consistently, test on multiple OSs

## Next Steps

1. Initialize Go module
2. Implement config package (simplest, no dependencies)
3. Implement index package (simple JSON storage)
4. Implement git package (shell out to git commands)
5. Implement scanner package (ties together git + index)
6. Implement CLI commands (init, scan, list first)
7. Test and iterate
