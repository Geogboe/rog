# rog Philosophy & Design Paradigms

## Core Philosophy

`rog` is built around a simple premise: **developers spend too much time finding and context-switching between repositories**. The solution isn't more complexityit's fast, predictable, local-first tooling that gets out of your way.

## Design Paradigms

### 1. Local-First, Network-Explicit

**Principle**: Everything works offline by default. Network operations are opt-in and clearly flagged.

**Why**:
- No surprises when working on a plane or with flaky WiFi
- Faster operations (no waiting for network timeouts)
- Clear mental model: local commands are fast, remote commands take time

**Implementation**:
```bash
# These NEVER touch the network
rog list
rog select
rog info
rog path

# These EXPLICITLY require network
rog scan --remote    # Fetch ahead/behind status
rog scan --llm       # Call LLM API
```

**Rejected Alternative**: Auto-fetch remote status
- **Why rejected**: Surprises users with slow operations, breaks offline usage

---

### 2. Fast by Default, Correct by Design

**Principle**: Target < 100ms for index queries, < 2s for scanning. But never sacrifice correctness for speed.

**Why**:
- Speed makes tools a joy to use (see: `fzf`, `ripgrep`)
- Slow tools break flow state
- But wrong data is worse than slow data

**Implementation**:
- In-memory index for instant queries
- Worker pool parallelism for scanning
- Glob pattern exclusions to skip entire subtrees
- Atomic writes to prevent corruption

**Rejected Alternative**: Eventual consistency, background syncing
- **Why rejected**: Complexity, potential for stale data, user confusion

---

### 3. Explicit Over Implicit

**Principle**: No magic. Users should understand what `rog` is doing.

**Why**:
- Debugging is easier
- Fewer surprises
- Builds trust

**Examples**:
-  `rog scan` shows "Scanning 3 roots..."
-  `rog list --debug` shows filtering logic
- L Auto-scanning in the background (rejected)
- L Silent metadata updates (rejected)

**Implementation**:
- Verbose error messages
- Progress indicators for long operations
- Debug logging available via `--debug`
- No hidden config files (everything in `~/.config/rog/`)

---

### 4. Read-Only by Default

**Principle**: Never modify user repositories unless explicitly requested.

**Why**:
- Safety: can't break working code
- Trust: users can run `rog` without fear
- Simplicity: no need for undo/rollback

**Implementation**:
- All git operations are read-only (`git status`, `git log`)
- Index and metadata live outside repos
- Only `.rogmeta.yml` is written to repos (and only with `rog meta` commands)

**Future**: If we add write operations (e.g., `rog fix-branch`), they'll require explicit confirmation

---

### 5. Composability & Unix Philosophy

**Principle**: Do one thing well, play nicely with other tools.

**Why**:
- Enables powerful workflows via composition
- Integrates into existing toolchains
- Doesn't lock users into `rog`-only workflows

**Examples**:
```bash
# Pipe to other tools
rog list --format=json | jq '.[] | select(.language == "Go")'

# Use with fzf
cd $(rog select --format=path)

# Combine with git
rog list --dirty | while read repo; do
  git -C $repo status
done

# Integrate into scripts
for repo in $(rog list --lang=python --format=path); do
  pytest $repo/tests
done
```

**Implementation**:
- `--format` flag for structured output (json, csv, plain)
- Exit codes follow conventions (0 = success, 1 = error)
- Respects `EDITOR` environment variable
- Works with standard input/output

---

### 6. Progressive Enhancement

**Principle**: Core functionality works everywhere. Advanced features are layered on top.

**Why**:
- Accessible to all users
- Graceful degradation
- Simpler onboarding

**Layers**:
1. **Base**: Scan, list, filter (pure Go, zero dependencies)
2. **Enhanced**: Interactive selection with `fzf` (if installed)
3. **Intelligent**: LLM enrichment (if configured)
4. **Advanced**: Remote status (if network available)

**Implementation**:
- Detect `fzf` at runtime, fallback to plain list
- LLM is opt-in via config + `--llm` flag
- Remote operations require `--remote` flag
- All features documented with requirements

---

### 7. Performance as a Feature

**Principle**: Fast tools are more useful tools. Performance isn't optional.

**Why**:
- Slow tools break flow state
- Fast feedback loops improve decision quality
- Performance enables new use cases

**Benchmarks**:
| Metric | Target | Why |
|--------|--------|-----|
| `rog list` | < 100ms | Instant feel, no perceived delay |
| `rog scan` (100 repos) | < 2s | Fast enough to run frequently |
| Index load | < 50ms | No delay on any command |
| `rog select` | < 1s | Interactive feel preserved |

**Implementation**:
- In-memory index (not database)
- Parallel scanning (2 × NumCPU workers)
- Parallel root walking
- Efficient exclusions (skip entire subtrees)
- Minimal allocations in hot paths

**Continuous Monitoring**:
- Performance tests in CI
- Benchmarks for critical paths
- Profile-guided optimization when needed

---

### 8. Zero Configuration, Infinite Customization

**Principle**: Works out of the box, but deeply customizable when needed.

**Why**:
- New users shouldn't need to configure anything
- Power users want control

**Defaults**:
- Scans `$HOME` with depth 3
- Excludes common directories (`node_modules`, `vendor`, `.git`)
- Uses `$EDITOR` or `vi`

**Customization**:
```yaml
# ~/.config/rog/config.yml
global_excludes:
  - "node_modules"
  - "vendor"
  - "*-cache"

roots:
  - name: work
    path: ~/work
    max_depth: 5
    exclude: ["proprietary"]

  - name: oss
    path: ~/src
    max_depth: 3

editor: code
llm:
  endpoint: http://localhost:1234/v1/chat/completions
  model: llama-3.1
```

---

### 9. Fail Fast, Degrade Gracefully

**Principle**: Errors in config ’ fail immediately. Errors in data ’ warn and continue.

**Why**:
- Invalid config is a user mistake (help them fix it)
- Broken repos are environmental (don't block other repos)

**Examples**:
```bash
# Config error ’ fail fast
$ rog scan
Error: Root 'work' path does not exist: /invalid/path
Fix: Update ~/.config/rog/config.yml

# Repo error ’ warn and continue
$ rog scan
Warning: failed to process /home/user/broken-repo: not a git repository
Warning: failed to process /home/user/no-perms: permission denied
Found 98 repositories
 Scan completed in 1.2s
```

**Implementation**:
- Validate config on load (`rog config --validate`)
- Log warnings for individual repo failures
- Continue scanning other repos
- Return success if at least one repo was scanned

---

### 10. Convention Over Configuration (Where Sensible)

**Principle**: Adopt widely-used conventions to reduce cognitive load.

**Why**:
- Users already know these patterns
- Reduces documentation burden
- Interoperability with other tools

**Conventions Adopted**:
- **XDG Base Directory Spec**: `$XDG_CONFIG_HOME`, `$XDG_DATA_HOME`
- **Git Conventions**: `origin` remote, `main`/`master` branch
- **Environment Variables**: `EDITOR`, `PATH`
- **Exit Codes**: 0 = success, 1 = error, 2 = usage error

**Conventions Rejected**:
- L Dotfiles in `$HOME` (use XDG instead)
- L Custom metadata formats (use YAML, a widely-known format)
- L Proprietary protocols (use OpenAI-compatible LLM API)

---

## Anti-Patterns We Avoid

### 1. Framework Lock-In
**Bad**: Require users to restructure repos or add special files
**Good**: Work with repos as-is, metadata is optional

### 2. Hidden Complexity
**Bad**: "Smart" auto-detection that sometimes fails mysteriously
**Good**: Explicit flags, clear errors, debug logging

### 3. One True Way
**Bad**: Force a specific workflow or tool integration
**Good**: Composable commands, multiple output formats

### 4. Premature Optimization
**Bad**: Complex caching, incremental updates, background syncing (before it's needed)
**Good**: Simple in-memory index, explicit re-scans, measure then optimize

### 5. Feature Creep
**Bad**: Integrated git client, CI/CD runner, project templates
**Good**: Fast repo discovery and navigation. That's it.

---

## Decision-Making Framework

When considering new features or changes, ask:

1. **Does this serve the core mission?** (Fast repo discovery and navigation)
   - If no ’ reject
   - If yes ’ continue

2. **Is it fast enough?** (< 100ms for queries, < 2s for scans)
   - If no ’ optimize or reject
   - If yes ’ continue

3. **Is it explicit or magical?**
   - If magical ’ can we make it explicit?
   - If explicit ’ continue

4. **Does it require network?**
   - If yes ’ must be flagged (e.g., `--remote`)
   - If no ’ continue

5. **Is it composable?**
   - If no ’ can we make it composable?
   - If yes ’ continue

6. **Does it work out of the box?**
   - If requires configuration ’ can we provide sane defaults?
   - If yes ’ continue

7. **Is the complexity justified?**
   - If complex ’ is the value proportional?
   - If simple ’ ship it

---

## Design Influences

`rog` draws inspiration from:

- **`fzf`**: Blazing fast, does one thing well, composable
- **`ripgrep`**: Performance as a feature, smart defaults
- **`chezmoi`**: Local-first, read-only by default, clear state
- **`gh`**: Explicit network operations, clean UX
- **`exa`**: Beautiful defaults, respects conventions

---

## Evolution Strategy

### Current Focus (V1)
-  Fast scanning and indexing
-  Flexible filtering and search
-  Interactive selection (via `fzf`)
-  Basic metadata (manual, auto-detected, LLM)

### Near Future (V2)
- = Incremental scanning (only changed dirs)
- = Watch mode (auto-update index)
- = Workspace management (groups of repos)
- = Plugin system (custom filters, actions)

### Long Term (V3+)
- =Ë Remote index sharing (team-wide metadata)
- =Ë CI integration (lint metadata, detect drift)
- =Ë IDE plugins (VSCode, Vim, etc.)
- =Ë Web UI (optional, for teams)

**Principle**: Each version must maintain core philosophy. No feature is worth sacrificing speed, simplicity, or local-first design.

---

## For AI Tools & Contributors

When working with `rog`:

1. **Prioritize performance**: Always benchmark before/after changes
2. **Preserve simplicity**: Resist the urge to add frameworks or abstractions
3. **Maintain explicitness**: No hidden side effects, no surprises
4. **Respect local-first**: Network operations must be opt-in
5. **Write tests**: Every feature needs unit + integration tests
6. **Document decisions**: Update this file when making design choices

---

## Conclusion

`rog` is not trying to be everything to everyone. It's a fast, local-first tool for discovering and navigating Git repositories. Every decision should serve that mission and honor these principles.

**When in doubt**: Ask "Does this make `rog` faster, simpler, or more reliable?" If the answer is no, don't build it.
