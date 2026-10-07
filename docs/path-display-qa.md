# List path display and QA

## Display decisions

The table path is a navigation argument, independent of the configured root
nickname. Prefer `~` within the executing user's home; otherwise use a shorter
relative path with at most two parent directories, then an absolute path.
Quote shell metacharacters for POSIX shells on Linux and PowerShell on Windows.
Keep POSIX home expansion outside quotes. Never treat a Linux backslash as a
safe unquoted character. Paths with control characters show a diagnostic rather
than a modified argument that could navigate to a different directory.

Do not start a WSL distribution or worker during listing. A foreign distro's
home and initial working directory are not recorded in the index, and its
mount layout need not use `/mnt/c`. Display its native absolute path with a
distro prefix instead of guessing either `~` or a relative path. Inside the
owning distro, ordinary home and relative shortening applies. Normalize WSL
UNC representations before formatting in either environment.

Never insert line breaks inside a table path. Truncate with an explicit ellipsis
when necessary, including short, long, and custom-field modes. A truncated path
is not a navigation argument; `rog path` provides the exact stored path. Very
narrow long tables reduce columns rather than filling the screen with fragments.
Raw JSON, YAML, and path output retain their existing contracts.

`--group-by root` affects tables only and groups by both environment and root.
Identical root names in different distributions must remain separate. Preserve
query order within groups and order groups by their first matching result.

## Verification and lessons

Use disposable `ROG_CONFIG` and `ROG_DATA` settings. Seed bounded index entries
and navigate using the displayed arguments, independently of scanner behavior.
Compare the index checksum before and after list commands.

The operator checks exercised real Linux CLI output, pasted home, nearby, and
space-containing arguments into Zsh `cd`, checked raw outputs and invalid
grouping flags, and verified unchanged configuration/index state. Native
Windows PowerShell exercised the Windows executable and successfully navigated
using its quoted home-relative fixture path. Its WSL row showed a distro-prefixed
Linux path. Windows-to-WSL navigation was not executed during these list checks.

Real PTY output at 80 and 45 columns exposed a same-distro UNC formatting issue;
normalize UNC paths even when no distro prefix is needed. Focused regressions
cover that case, separate distro grouping, Windows drive boundaries, home-prefix
collisions, quoting, and one-line truncation in every table mode. Race tests and
vet cover the command package; build both native Linux and Windows executables.
