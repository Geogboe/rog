package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/picker"
	"github.com/Geogboe/rog/internal/query"
)

var (
	selectOpen bool
)

var selectCmd = &cobra.Command{
	Use:   "select [search terms...]",
	Short: "Interactively select a repository",
	Long: `Select a repository using rog's built-in searchable picker.

Accepts the same search terms and filters as 'rog list'.
Requires an interactive terminal when multiple repositories match.

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
	selectCmd.Flags().BoolVar(&selectOpen, "open", false, "Open selected repository in editor")
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
		if selectOpen {
			openInEditor(results[0])
			return
		}
		fmt.Println(results[0].AbsPath)
		return
	}

	items, byID := selectItems(results)
	selectedID, err := picker.Run(items)
	if err != nil {
		exitWithError("Cannot open repository picker: %v. Use 'rog list' in a noninteractive session.", err)
	}
	if selectedID == "" {
		return
	}
	selected := byID[selectedID]
	if selected == nil {
		exitWithError("Selected repository is no longer available")
	}
	if selectOpen {
		openInEditor(selected)
	} else {
		fmt.Println(selected.AbsPath)
	}
}

func selectItems(results []*index.Repo) ([]picker.Item, map[string]*index.Repo) {
	items := make([]picker.Item, 0, len(results))
	byID := make(map[string]*index.Repo, len(results))
	for _, repo := range results {
		status := "clean"
		if repo.StatusUnavailable {
			status = "status unavailable"
		} else if repo.IsDirty {
			status = "dirty"
		}
		items = append(items, picker.Item{ID: repo.AbsPath, Name: repo.Name, Root: repo.Root, Path: repo.AbsPath,
			Language: repo.PrimaryLanguage, Status: status, Description: repo.Description})
		byID[repo.AbsPath] = repo
	}
	return items, byID
}

func openInEditor(repo *index.Repo) {
	// Load config to get editor
	cfg, err := config.Load()
	if err != nil {
		exitWithError("Failed to load config: %v", err)
	}

	editor := cfg.Editor
	if editor == "" {
		// Fall back to environment variables
		editor = os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
			if editor == "" {
				editor = "vim" // Last resort default
			}
		}
	}

	// Open editor in the repo directory
	cmd := exec.Command(editor, ".")
	cmd.Dir = repo.AbsPath
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		exitWithError("Failed to open editor: %v", err)
	}
}
