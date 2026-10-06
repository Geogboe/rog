# Setup review and operator QA

Before pushing setup changes, review the final diff, run a lessons-learned pass,
and exercise the installed CLI through a real terminal. Unit tests and a
cross-build establish different things from an installed Windows/WSL journey.

## Transaction lessons

- Invalidate the old proposal before generating a new review. If a concurrent
  config edit makes regeneration fail, another confirmation must never apply
  the stale proposal. Verify that the external edit survives repeated save
  attempts and no backup or index is created.
- Keep manual entry visible independently of the existing root list. At 80 by
  24, a fixed footer can remain visible while the actual input field is clipped.
  Show the entry form before the list and inspect the real terminal after resize.
- Normalize root identity within its filesystem owner. Trailing separators,
  dot segments, and Windows path casing must not produce duplicate roots.
  Identical Linux paths in different WSL distributions remain distinct.
- Run the post-save scan using the executing command's context. Calling a
  sibling Cobra command directly can leave its context nil and crash after
  configuration was saved. Test the actual save-to-scan transition.

## Safe operator checks

Use temporary `ROG_CONFIG` and `ROG_DATA` paths, and fixture repositories. Preserve
real user configuration and indexes. Resolve and verify installed Windows and
WSL executables separately; a temporary build is not an installation.

Walk the wizard as an operator: inspect help, try a relative path and correct
it, add a root and depth, review, return to edit, cancel, restart, save, decline
the scan, then rerun and accept a full local scan. Confirm the backup contains
the preceding config and the final scan uses the saved roots. Exercise
filesystem discovery separately from Git scanning. Discovery must work without
Git and leave config/history/index untouched.

Capture live PTY and Windows ConPTY frames at normal, narrow, and short sizes.
Check the current question, editable field, selected row, validation error,
progress, and footer. After Ctrl+C, verify cursor restoration and unchanged
configuration. Bind terminal bridges to loopback, use a fixed fixture command,
and stop only the processes created for this check.

WSL discovery requires an available matching worker. Report unavailable workers
as partial discovery and show an actionable recovery instruction. A native
Windows/WSL bridge journey must be reported separately from Linux worker tests
or Windows cross-compilation. Do not claim live bridge proof from either.

## Review evidence

The review of the three-step wizard reproduced and fixed stale-proposal saving
following a failed conflict refresh, hidden root entry with many configured
roots at 80 by 24, and duplicate root identity from path spelling differences.
Each case has a focused regression in `internal/setupui/ui_test.go`. The earlier
save-to-scan command-context crash is covered by `cmd/setup_test.go`.

## Interactive verification and additional lessons

The installed Windows CLI completed a full local fixture scan with one native
Windows repository and one Ubuntu repository, including first-time worker
installation. A live Linux PTY completed filesystem-only system discovery,
showed root recommendations, and cancelled without creating its config or
index. Manual root entry, depth adjustment, review, save, and declining the
post-save scan also passed. Windows ConPTY and Linux PTY screens were inspected
at normal and narrower/shorter terminal sizes, including a corrected relative
path error and cursor restoration after cancellation.

The Windows journey revealed that rich progress could overwrite the worker
approval question while waiting for an answer. Give approvals exclusive console
ownership, skip progress frames until the answer, and begin the prompt on a new
line. Retest this using a genuinely uncached worker; a warm scan cannot exercise
the approval path. The concurrency regression is in `cmd/scan_test.go`.

Before discovery runs, do not display a zero repository count as though the
root was checked. After discovery, compute per-root coverage using its current
depth and exclusions. System-wide discovery can include development-tool and
managed-worktree locations; review the recommendations before saving them.

The repaired worker approval remained visible during a deliberate pause and
accepted input before completing the Windows/Ubuntu scan. A bounded native
Windows-to-Ubuntu discovery check returned one fixture location and its depth-1
root recommendation without Git metadata. The installed WSL wizard also
completed its optional full scan and wrote the expected one-repository index.
