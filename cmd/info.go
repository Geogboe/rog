package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/query"
)

var infoCmd = &cobra.Command{
	Use:   "info <name|path|query>",
	Short: "Show detailed information about a repository",
	Long: `Show detailed information about a repository.

Accepts exact name, absolute path, or a fuzzy query.
If the query matches multiple repositories, shows a list of matches.

Examples:
  rog info myproject
  rog info /home/user/projects/myproject
  rog info api`,
	Args: cobra.ExactArgs(1),
	Run:  runInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)
}

func runInfo(cmd *cobra.Command, args []string) {
	// Load index
	idx, err := index.Load()
	if err != nil {
		exitWithError("Failed to load index: %v", err)
	}

	queryStr := args[0]

	// Try to find unique match
	repo, matches, err := query.FindUnique(idx, queryStr)
	if err != nil {
		exitWithError("Query failed: %v", err)
	}

	if repo == nil {
		if len(matches) == 0 {
			exitWithError("No repository found matching '%s'", queryStr)
		} else {
			fmt.Printf("Multiple repositories match '%s':\n\n", queryStr)
			outputTable(matches, false, false, nil)
			fmt.Println("\nPlease be more specific or use 'rog select' to choose interactively.")
			return
		}
	}

	writeInfo(os.Stdout, repo, isInteractiveTerminal(os.Stdout) && supportsANSIColor())
}

func writeInfo(out io.Writer, repo *index.Repo, color bool) {
	label := func(name string) string {
		if color {
			return "\x1b[2m" + name + "\x1b[0m"
		}
		return name
	}
	value := func(text string) string { return cleanTableCell(text) }
	name := value(repo.Name)
	if color {
		name = "\x1b[1;36m" + name + "\x1b[0m"
	}
	fmt.Fprintf(out, "%s        %s\n", label("Name:"), name)
	if repo.Description != "" {
		fmt.Fprintf(out, "%s %s\n", label("Description:"), value(repo.Description))
	}
	fmt.Fprintln(out)

	fmt.Fprintf(out, "%s        %s\n", label("Path:"), value(repo.AbsPath))
	fmt.Fprintf(out, "%s        %s\n", label("Root:"), value(repo.Root))
	if repo.RelPath != "" {
		fmt.Fprintf(out, "%s    %s\n", label("Relative:"), value(repo.RelPath))
	}
	fmt.Fprintln(out)

	if repo.RemoteURL != "" {
		fmt.Fprintf(out, "%s      %s\n", label("Remote:"), value(repo.RemoteURL))
		fmt.Fprintf(out, "%s        %s\n", label("Host:"), value(repo.Host))
		fmt.Fprintln(out)
	}

	if repo.CurrentBranch != "" {
		status := formatDetailedStatus(repo)
		if color {
			code := "32"
			if repo.StatusUnavailable {
				code = "31"
			} else if repo.IsDirty || repo.HasUntracked || repo.Behind > 0 {
				code = "33"
			}
			status = "\x1b[" + code + "m" + status + "\x1b[0m"
		}
		fmt.Fprintf(out, "%s      %s (%s)\n", label("Branch:"), value(repo.CurrentBranch), status)
	}

	if !repo.LastCommitTime.IsZero() {
		fmt.Fprintf(out, "%s %s by %s\n", label("Last commit:"),
			repo.LastCommitTime.Format("2006-01-02 15:04"),
			value(repo.LastCommitAuthor))
		if repo.LastCommitHash != "" {
			fmt.Fprintf(out, "             %s\n", value(repo.LastCommitHash[:min(8, len(repo.LastCommitHash))]))
		}
	}
	fmt.Fprintln(out)

	if repo.PrimaryLanguage != "" {
		fmt.Fprintf(out, "%s    %s\n", label("Language:"), value(repo.PrimaryLanguage))
	}
	if len(repo.Tags) > 0 {
		fmt.Fprintf(out, "%s        %s\n", label("Tags:"), value(strings.Join(repo.Tags, ", ")))
	}
	fmt.Fprintln(out)

	fmt.Fprintf(out, "%s      %s\n", label("First seen:"), repo.FirstSeenAt.Format("2006-01-02 15:04"))
	fmt.Fprintf(out, "%s       %s\n", label("Last scan:"), repo.LastScanAt.Format("2006-01-02 15:04"))
	if !repo.LastGitCheckAt.IsZero() {
		fmt.Fprintf(out, "%s  %s\n", label("Last git check:"), repo.LastGitCheckAt.Format("2006-01-02 15:04"))
	}
}

func formatDetailedStatus(repo *index.Repo) string {
	var parts []string

	if repo.Ahead > 0 && repo.Behind > 0 {
		parts = append(parts, fmt.Sprintf("diverged ↑%d ↓%d", repo.Ahead, repo.Behind))
	} else if repo.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("ahead %d", repo.Ahead))
	} else if repo.Behind > 0 {
		parts = append(parts, fmt.Sprintf("behind %d", repo.Behind))
	}

	if repo.StatusUnavailable {
		parts = append(parts, "status unknown")
	} else if repo.IsDirty {
		parts = append(parts, "dirty")
	} else if repo.HasUntracked {
		parts = append(parts, "untracked")
	} else {
		parts = append(parts, "clean")
	}

	if len(parts) == 0 {
		return "up-to-date, clean"
	}

	return strings.Join(parts, ", ")
}
