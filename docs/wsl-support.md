# WSL Support Design

## Overview

For Windows users with WSL (Windows Subsystem for Linux) installed, rog should be able to scan and index repositories inside WSL distributions.

## Configuration

Add a `wsl` field to root configuration:

```yaml
roots:
  - name: windows-dev
    path: C:\Users\username\dev
    max_depth: 3

  - name: ubuntu-dev
    path: /home/username/dev
    max_depth: 3
    wsl: true
    wsl_distro: Ubuntu  # Optional, defaults to default WSL distro
```

## Implementation Details

### Path Translation

When `wsl: true` is set:
- On Windows: Use `wsl.exe` commands to access WSL filesystem
- Git commands run inside WSL using `wsl -d <distro> -- git ...`
- Paths are translated:
  - WSL path: `/home/user/project`
  - Windows path: `\\wsl$\Ubuntu\home\user\project` or `\\wsl.localhost\Ubuntu\home\user\project`

### Visual Distinction

In `rog list` output, WSL repositories are marked:
- Root field shows: `wsl:ubuntu-dev` instead of just `ubuntu-dev`
- This makes it clear which repos are in WSL

### Platform Detection

- WSL features only available on Windows
- Gracefully handle when WSL is not installed
- Validate WSL distro exists before scanning

### Git Operations

All git commands for WSL repos must run inside WSL:
```bash
# Instead of:
git -C /path status

# Use:
wsl -d Ubuntu -- git -C /path status
```

### Editor Integration

When opening WSL repos:
- If editor supports WSL paths (like VS Code with Remote-WSL): use WSL path
- Otherwise: use Windows UNC path `\\wsl$\...`
- Config option: `wsl_editor_mode: "wsl" | "windows"`

## Example Configuration

```yaml
roots:
  - name: windows-projects
    path: C:\Users\johndoe\projects
    max_depth: 4

  - name: wsl-ubuntu
    path: /home/johndoe/projects
    max_depth: 4
    wsl: true
    wsl_distro: Ubuntu

  - name: wsl-debian
    path: /home/john/dev
    max_depth: 3
    wsl: true
    wsl_distro: Debian

editor: code  # VS Code with Remote-WSL extension
wsl_editor_mode: wsl  # Use WSL paths when opening

llm:
  endpoint: http://localhost:11434/v1
  model: codellama
```

## Benefits

1. **Unified View**: See all repositories (Windows + WSL) in one index
2. **Clear Distinction**: Easy to identify WSL vs Windows repos
3. **Seamless Workflow**: Navigate between Windows and WSL repos
4. **Cross-platform**: Feature is Windows-only, doesn't affect Linux/Mac

## Open Questions

1. Should we support multiple WSL distros simultaneously? **Yes**
2. How to handle performance when scanning WSL from Windows? **Same as regular scanning, WSL2 is fast**
3. What if WSL distro is not running? **WSL2 auto-starts, no issue**
4. Should we cache WSL availability checks? **Yes, check once per scan**

## Implementation Tasks

1. Add `wsl` and `wsl_distro` fields to Root config struct
2. Add platform detection (Windows-only feature)
3. Implement WSL command wrapper for git operations
4. Update scanner to handle WSL roots
5. Update display logic to show "wsl:" prefix
6. Add WSL path utilities (translate paths, check availability)
7. Update documentation with WSL examples
8. Add tests for WSL functionality (mock on non-Windows)
