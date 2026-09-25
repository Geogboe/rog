# WSL Support

## Overview

On Windows, rog scans and indexes repositories inside WSL distributions when a root has `wsl: true`.

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

### Scanner worker and paths

On Windows, rog launches one bundled Linux worker per distro with `wsl.exe --exec`. The worker performs discovery, Git, language, and README reads inside Linux and streams progress and results over standard streams. There is no daemon or listening port. Windows index paths use `\\wsl$\<distro>\...`; rog does not silently scan the share if the worker fails.

The matching worker is cached inside the distro at `${XDG_CACHE_HOME:-$HOME/.cache}/rog/workers/<build-id>/rog-worker`. Before first install or update, rog prompts with the distro, exact path, and changes. The default answer is no. `--approve-wsl-worker-install` explicitly approves unattended installs; `--dry-run` never installs. A refusal or failure makes the scan incomplete and preserves index entries for the affected roots. Matching cached workers require no prompt. The bundle checksum is checked before an atomic install as the normal distro user.

### Visual Distinction

The Root column shows the configured root name. JSON output includes `is_wsl: true` and the resolved `wsl_distro`; absolute paths use the WSL UNC path so Windows commands can open them.

### Platform Detection

- WSL features only available on Windows
- Gracefully handle when WSL is not installed
- Validate WSL distro exists before scanning

### Git Operations

The worker runs Git inside the distro with bounded concurrent commands and timeouts. Windows does not launch `wsl.exe` for each repository. Git status timeouts appear as unavailable status and are reported in the scan summary.

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
