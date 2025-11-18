package cmd

import (
	"fmt"
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
			outputTable(matches, false)
			fmt.Println("\nPlease be more specific or use 'rog select' to choose interactively.")
			return
		}
	}

	// Display detailed info
	fmt.Printf("Name:        %s\n", repo.Name)
	if repo.Description != "" {
		fmt.Printf("Description: %s\n", repo.Description)
	}
	fmt.Println()

	fmt.Printf("Path:        %s\n", repo.AbsPath)
	fmt.Printf("Root:        %s\n", repo.Root)
	if repo.RelPath != "" {
		fmt.Printf("Relative:    %s\n", repo.RelPath)
	}
	fmt.Println()

	if repo.RemoteURL != "" {
		fmt.Printf("Remote:      %s\n", repo.RemoteURL)
		fmt.Printf("Host:        %s\n", repo.Host)
		fmt.Println()
	}

	if repo.CurrentBranch != "" {
		status := formatDetailedStatus(repo)
		fmt.Printf("Branch:      %s (%s)\n", repo.CurrentBranch, status)
	}

	if !repo.LastCommitTime.IsZero() {
		fmt.Printf("Last commit: %s by %s\n",
			repo.LastCommitTime.Format("2006-01-02 15:04"),
			repo.LastCommitAuthor)
		if repo.LastCommitHash != "" {
			fmt.Printf("             %s\n", repo.LastCommitHash[:8])
		}
	}
	fmt.Println()

	if repo.PrimaryLanguage != "" {
		fmt.Printf("Language:    %s\n", repo.PrimaryLanguage)
	}
	if len(repo.Tags) > 0 {
		fmt.Printf("Tags:        %s\n", strings.Join(repo.Tags, ", "))
	}
	fmt.Println()

	fmt.Printf("First seen:      %s\n", repo.FirstSeenAt.Format("2006-01-02 15:04"))
	fmt.Printf("Last scan:       %s\n", repo.LastScanAt.Format("2006-01-02 15:04"))
	if !repo.LastGitCheckAt.IsZero() {
		fmt.Printf("Last git check:  %s\n", repo.LastGitCheckAt.Format("2006-01-02 15:04"))
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

	if repo.IsDirty {
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
