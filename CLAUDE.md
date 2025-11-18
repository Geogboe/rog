# CLAUDE.md - Development Guidelines for rog

## Critical Principles

### Critique and Clarification
- **Challenge assumptions**: If something doesn't make sense, ask for clarification rather than making assumptions
- **Full understanding first**: Don't just accept requirements at face value - ensure deep understanding of the bigger picture
- **Exception**: When explicitly told to "just iterate", proceed without seeking clarification
- **Be thoughtful**: Provide meaningful critiques that help understand architecture and design implications

### Version Control
- **Commit and push often**: Small, incremental commits are better than large monolithic ones
- **Use conventional commits**: Follow the format `type(scope): description`
  - Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`
  - Example: `feat(scan): add remote sync capability`
- **No confirmation needed**: Push commits without asking for permission
- **Keep history clean**: But prioritize velocity over perfection

### Code Quality
- **DRY (Don't Repeat Yourself)**: If a reputable package can do something, use it
- **Don't reinvent the wheel**: Leverage existing solutions from the Go ecosystem
- **But be pragmatic**: Don't be afraid to write custom tooling when needed
- **Invent when appropriate**: New techniques and paradigms are welcome when they solve real problems
- **Choose quality packages**: Prefer well-maintained, widely-used libraries

### Documentation
- **Keep CLAUDE.md focused**: This file is for development guidelines only
- **Use docs/ folder**: All repo-specific documentation goes in `docs/`
  - Architecture decisions
  - API documentation
  - Design rationale
  - User guides
- **Keep docs current**: Update documentation as the codebase evolves

## Development Workflow

### 1. Architect (Skeptical Phase)
- **Question everything**: Approach as a skeptical architect
- **Identify risks**: Call out potential issues, edge cases, and technical debt
- **Consider alternatives**: Explore different approaches before committing
- **Validate assumptions**: Ensure the design actually solves the problem

### 2. Plan
- **Break down work**: Create actionable tasks
- **Define success criteria**: How will we know it works?
- **Identify dependencies**: What needs to happen first?
- **Estimate complexity**: Flag complex or risky areas

### 3. Roadmap
- **Prioritize**: What needs to be built first?
- **Milestones**: Define clear checkpoints
- **Incremental delivery**: Build in stages, not all at once

### 4. Build and Run
- **Start simple**: Get something working quickly
- **Iterate**: Refine and improve incrementally
- **Test as you go**: Don't wait until the end

### 5. Test, Test, Test
- **Unit tests**: Test individual components
- **Integration tests**: Test components working together
- **End-to-end tests**: Test complete user workflows
- **Smoke tests**: Quick sanity checks
- **Stress tests**: Performance and load testing
- **Use stubs/mocks**: If you don't have access to a component, stub or mock it
- **Use Docker**: Leverage containers for full e2e test environments when needed
- **Validate thoroughly**: Don't skip edge cases

### 6. Review
- **Self-review code**: Read through changes with fresh eyes
- **Check against spec**: Does it meet requirements?
- **Performance review**: Is it fast enough?
- **Security review**: Are there vulnerabilities?

### 7. Iterate
- **Continuous improvement**: Always look for ways to make it better
- **Refactor when needed**: Don't let technical debt accumulate
- **Keep moving forward**: Don't get stuck in analysis paralysis

## rog-Specific Guidelines

### Philosophy
- **Fast and predictable**: Index-driven operations should be instant
- **Zero surprise network calls**: Network operations must be explicit
- **Local-first**: Everything works offline except `--remote` and `--llm`
- **Never touch user repos**: Read-only unless explicitly asked
- **Clean UX**: Simple, intuitive commands over clever abstractions

### Performance Targets
- `list`, `select`, `info`, `path`, `open`: **< 100ms** (instant feel)
- `scan`: **< 2s** for typical workspace (hundreds of repos)
- Index operations: **in-memory** where possible

### Testing Requirements
- **CLI integration tests**: Test actual command execution
- **Index integrity tests**: Ensure data consistency
- **Filesystem edge cases**: Symlinks, permissions, missing dirs
- **Git state variations**: Dirty, clean, ahead, behind, diverged
- **LLM integration**: Mock LLM responses for testing
- **Cross-platform**: Test on Linux, macOS (if possible)

### Code Organization
```
rog/
├── cmd/           # CLI commands (cobra)
├── internal/      # Private application code
│   ├── index/     # Index management
│   ├── scanner/   # Repo scanning logic
│   ├── config/    # Configuration handling
│   ├── llm/       # LLM integration
│   └── git/       # Git operations
├── pkg/           # Public libraries (if needed)
├── docs/          # Documentation
└── tests/         # Integration and e2e tests
```

## Current Status

This is a greenfield project. We're starting from scratch to build a fast, reliable Git repository navigator and catalog system.

**Next Steps**: Architecture phase → Planning → Implementation → Testing
