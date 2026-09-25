# Native scan QA, 2026-09-25

## Controlled Windows comparison

Both builds used the same three configured roots (two Windows, one Ubuntu WSL) and separate temporary copies of the same 753-entry index. The old build came from the branch's initial commit; the new build came from the local source. Timings are wall clock on this machine and are sensitive to filesystem and host load. The old build's WSL availability check needed `C:\Windows\System32` and `PATHEXT` in the PowerShell test process; both builds then ran with the same environment. No test scan wrote the production index.

| Operation | Old build | New build |
| --- | ---: | ---: |
| Discovery only (`--dry-run`) | 2.779 s | 0.800 s |
| Warm default scan | 2.487 s | 0.390 s after candidate cache warmup |
| Full Git refresh | Exceeded 90 s; stopped | 23.759 s |

The first new default scan, including worker cache installation and inspection of previously unrecognized markers, took 1.254 s. Stage timings for the completed full refresh were discovery 21.483 s of aggregate work, Git 3m7.043s of aggregate work across concurrent workers, metadata 2.116 s, WSL transport 2.013 s, and index write 15 ms. Aggregate stage times overlap and should not be added to infer wall time. The full scan reported three unavailable statuses, zero timeouts.

Discovery returned 658 markers in the Windows roots with either implementation. The old WSL discovery returned 145 markers after its Git validation filter; the new walker returned 165 raw WSL markers and rejected the extra 20 during inspection. Warm scan index sets were identical: 753 entries, zero additions or removals. The new full refresh ended with 751 entries; two previously indexed paths had disappeared from successfully enumerated Windows roots. The old full run did not finish within the 90 second cap, so there is no completed old full-refresh repository set to compare.

A separate Ubuntu run scanning one Linux root and two mounted Windows roots completed in 3.536 s with a warm index. This exceeds the two-second project target for that mixed-filesystem configuration. A Windows-initiated warm scan of the same three roots completed in 390 ms. The remaining Ubuntu cross-filesystem cost should be measured and tuned before claiming the target for that configuration.

## Operator checks

- Windows PowerShell: actual WSL scan, cached and first approved worker installation, full and warm scans, clean output redirection, and a picker selecting between repositories with the same name. The picker wrote only the chosen path to redirected stdout. A noninteractive multiple-result selection produced an actionable error.
- Ubuntu: built-in picker in command substitution, selected path only on stdout; mixed-root warm scan; scan and single-result selection with a PATH containing Git but no `fd`, `fdfind`, or `fzf`.
- Windows: three-root warm scan with PATH limited to Windows system tools and Git completed without `fd` or `fzf`.
- Worker protocol: an incompatible version request exited with an explicit error. A truncated installation left the previous cached executable intact and removed the temporary file in a disposable-cache test. Refusing first installation in an earlier isolated fixture left the root incomplete without changing its index entries.
- Automated tests: `go test ./... -short`, `go vet ./...`, focused race tests, Windows cross-build, Linux build, a successful GoReleaser snapshot with notices in the Windows and Linux archives, and a 10,000-entry picker matcher benchmark. The matcher took about 3.7 ms per pass on the measured machine.

The status-unavailable count includes repositories with damaged Git objects. rog reports them without editing those repositories. The remaining work is tracked in GitHub issues rather than a local future-work file.
