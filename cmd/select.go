package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/query"
)

var selectCmd = &cobra.Command{
	Use:   "select [search terms...]",
	Short: "Interactively select a repository",
	Long: `Select a repository interactively using fzf (if available).

Accepts the same search terms and filters as 'rog list'.
If fzf is not available, falls back to plain list output.

Returns the absolute path of the selected repository.

Examples:
  cd "$(rog select api)"
  rog select --lang go --tag cli`,
	Run:     runSelect,
	Aliases: []string{"sel"},
}

func init() {
	rootCmd.AddCommand(selectCmd)

	// Reuse list flags
	selectCmd.Flags().StringSliceVar(&listLang, "lang", nil, "Filter by language")
	selectCmd.Flags().StringSliceVar(&listTag, "tag", nil, "Filter by tags")
	selectCmd.Flags().StringVar(&listBranch, "branch", "", "Filter by branch")
	selectCmd.Flags().StringVar(&listRoot, "root", "", "Filter by root")
	selectCmd.Flags().BoolVar(&listDirty, "dirty", false, "Show only dirty repos")
	selectCmd.Flags().BoolVar(&listClean, "clean", false, "Show only clean repos")
	selectCmd.Flags().BoolVar(&listAhead, "ahead", false, "Show only repos ahead")
	selectCmd.Flags().BoolVar(&listBehind, "behind", false, "Show only repos behind")
	selectCmd.Flags().StringVar(&listSort, "sort", "name", "Sort by: name, last-commit, path, last-scan")
	selectCmd.Flags().IntVar(&listLimit, "limit", 0, "Limit results")
}

func runSelect(cmd *cobra.Command, args []string) {
	// Load index
	idx, err := index.Load()
	if err != nil {
		exitWithError("Failed to load index: %v", err)
	}

	if idx.Count() == 0 {
		exitWithError("No repositories found. Run 'rog scan' first.")
	}

	// Build filter (same as list)
	filter := &query.Filter{
		SearchTerms: args,
		Languages:   listLang,
		Tags:        listTag,
		Branch:      listBranch,
		Root:        listRoot,
		SortBy:      parseSortField(listSort),
		Limit:       listLimit,
	}

	if listDirty {
		dirty := true
		filter.Dirty = &dirty
	}
	if listClean {
		clean := false
		filter.Dirty = &clean
	}
	if listAhead {
		ahead := true
		filter.Ahead = &ahead
	}
	if listBehind {
		behind := true
		filter.Behind = &behind
	}

	// Execute query
	results := query.Query(idx, filter)

	if len(results) == 0 {
		exitWithError("No repositories match the criteria.")
	}

	// If only one result, return it
	if len(results) == 1 {
		fmt.Println(results[0].AbsPath)
		return
	}

	// Try to use fzf
	if hasFzf() {
		selected := selectWithFzf(results)
		if selected != nil {
			fmt.Println(selected.AbsPath)
		}
	} else {
		// Fallback: just list them
		fmt.Fprintln(os.Stderr, "fzf not found. Install fzf for interactive selection.")
		fmt.Fprintln(os.Stderr, "Listing matching repositories:")
		outputTable(results, false, false, nil)
	}
}

func hasFzf() bool {
	_, err := exec.LookPath("fzf")
	return err == nil
}

func selectWithFzf(repos []*index.Repo) *index.Repo {
	// Build input for fzf
	var lines []string
	for _, repo := range repos {
		line := fmt.Sprintf("%s\t%s\t%s/%s",
			repo.Name,
			repo.PrimaryLanguage,
			repo.Root,
			repo.RelPath,
		)
		lines = append(lines, line)
	}

	input := strings.Join(lines, "\n")

	// Run fzf
	cmd := exec.Command("fzf",
		"--height=40%",
		"--reverse",
		"--header=Select a repository",
		"--delimiter=\t",
		"--with-nth=1,2,3",
	)

	cmd.Stdin = strings.NewReader(input)
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		// User cancelled or error
		return nil
	}

	selected := strings.TrimSpace(string(output))
	if selected == "" {
		return nil
	}

	// Extract repo name from selection
	parts := strings.Split(selected, "\t")
	if len(parts) == 0 {
		return nil
	}

	name := parts[0]

	// Find the repo
	for _, repo := range repos {
		if repo.Name == name {
			return repo
		}
	}

	return nil
}
