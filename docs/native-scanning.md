# Native scanning and picker decisions

## Runtime boundaries

The discovery walker reads directory entries in batches of 128 through eight bounded workers. It includes hidden directories, discovers nested repositories and `.git` worktree files, prunes `.git` contents, checks exclusions before descending, and does not follow directory symlinks. A full queue makes a worker process a child inline, avoiding a producer/consumer deadlock. Roots with unreadable areas remain incomplete, so their indexed entries are retained. A cancelled scan does not save the index.

On Windows, each configured WSL distro runs one short-lived Linux worker for the scan. The worker performs discovery, Git inspection, language detection, and README reads inside Linux. The protocol uses standard streams; no daemon, port, or share fallback is involved. The release bundle contains the worker and its SHA-256 checksum. Installation uses a temporary file, checksum validation, executable permissions, and rename in the distro user's cache. The first install or update requires an interactive yes or `--approve-wsl-worker-install`. The prompt states the distro, path, and changes; no answer means no install. See [WSL support](wsl-support.md).

The UI layer in `internal/picker` owns terminal input and rendering. Query/filter logic and scanner code do not import Bubble Tea. Only the selected stable path identity goes to stdout. Inspired by fd's bounded traversal and fzf's boundary/consecutive match scoring; no source code from either project was copied. Their upstream code and licenses were reviewed: [fd walker](https://raw.githubusercontent.com/sharkdp/fd/master/src/walk.rs) ([MIT](https://raw.githubusercontent.com/sharkdp/fd/master/LICENSE-MIT)) and [fzf matcher](https://raw.githubusercontent.com/junegunn/fzf/master/src/algo/algo.go) ([MIT](https://raw.githubusercontent.com/junegunn/fzf/master/LICENSE)).

## Bubble Tea dependency review

- Pinned release: `github.com/charmbracelet/bubbletea v1.3.10` in `go.mod` and `go.sum`. The project already targets Go 1.24.7; this release does not raise that requirement.
- License: [MIT](https://raw.githubusercontent.com/charmbracelet/bubbletea/main/LICENSE). All 36 non-main Go modules in the resolved module graph have their license text in the generated [third-party notices](../THIRD_PARTY_NOTICES.txt), including both MIT and BSD alternatives where a module offers them. Release archives include the notices.
- Dependency closure: 36 non-main modules in `go list -m all`, including test-only modules; the picker imports Bubble Tea plus the narrow Charm ANSI/terminal helpers. No broader component suite was added.
- Telemetry/network: inspection of the picker dependency source found no telemetry, analytics, update check, or HTTP client. Picker operation is local and offline. rog's explicit `--remote` and `--llm` operations are separate.
- Size: the local Linux executable grew from about 11 MiB to 12 MiB; the Windows executable from about 11 MiB to 13 MiB. The bundled compressed Linux worker is 1.4 MiB.
- Startup: 20 local Ubuntu `rog version` runs had median 2.23 ms before and 2.13 ms after; p95 3.14 ms and 3.05 ms respectively. This is a small launch benchmark, not a claim about interactive redraw time.
- Matcher: a local benchmark filtering 10,000 entries took about 3.7 ms per pass on an i7-11800H. Hardware and workload affect that number.

The dependency cost is measurable and should be revisited if a smaller terminal loop can match Bubble Tea's input, resize, and cleanup behavior. Regenerate notices with `python3 scripts/build-third-party-notices.py` after changing modules; regenerate the worker with `bash scripts/build-wsl-worker.sh` before release builds.
