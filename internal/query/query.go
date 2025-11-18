package query

import (
	"sort"
	"strings"

	"github.com/Geogboe/rog/internal/index"
)

// SortField represents a field to sort by
type SortField int

const (
	SortByName SortField = iota
	SortByLastCommit
	SortByPath
	SortByLastScan
)

// Filter represents query filters
type Filter struct {
	SearchTerms []string // Fuzzy search terms
	Languages   []string // Exact language matches
	Tags        []string // All tags must match
	Branch      string   // Exact branch match
	Root        string   // Exact root match
	Dirty       *bool    // nil = no filter, true = dirty only, false = clean only
	Ahead       *bool    // nil = no filter, true = ahead only
	Behind      *bool    // nil = no filter, true = behind only
	SortBy      SortField
	Limit       int
}

// Query executes a query against the index
func Query(idx *index.Index, filter *Filter) []*index.Repo {
	repos := idx.List()

	// Apply filters
	var filtered []*index.Repo
	for _, repo := range repos {
		if matchesFilter(repo, filter) {
			filtered = append(filtered, repo)
		}
	}

	// Sort
	sortRepos(filtered, filter.SortBy)

	// Apply limit
	if filter.Limit > 0 && len(filtered) > filter.Limit {
		filtered = filtered[:filter.Limit]
	}

	return filtered
}

// matchesFilter checks if a repo matches the filter criteria
func matchesFilter(repo *index.Repo, filter *Filter) bool {
	// Search terms (fuzzy)
	if len(filter.SearchTerms) > 0 {
		if !matchesSearchTerms(repo, filter.SearchTerms) {
			return false
		}
	}

	// Language filter
	if len(filter.Languages) > 0 {
		found := false
		for _, lang := range filter.Languages {
			if strings.EqualFold(repo.PrimaryLanguage, lang) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Tags filter (all must match)
	if len(filter.Tags) > 0 {
		for _, filterTag := range filter.Tags {
			found := false
			for _, repoTag := range repo.Tags {
				if strings.EqualFold(repoTag, filterTag) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	// Branch filter
	if filter.Branch != "" {
		if !strings.EqualFold(repo.CurrentBranch, filter.Branch) {
			return false
		}
	}

	// Root filter
	if filter.Root != "" {
		if !strings.EqualFold(repo.Root, filter.Root) {
			return false
		}
	}

	// Dirty filter
	if filter.Dirty != nil {
		if *filter.Dirty && !repo.IsDirty && !repo.HasUntracked {
			return false
		}
		if !*filter.Dirty && (repo.IsDirty || repo.HasUntracked) {
			return false
		}
	}

	// Ahead filter
	if filter.Ahead != nil {
		if *filter.Ahead && repo.Ahead == 0 {
			return false
		}
		if !*filter.Ahead && repo.Ahead > 0 {
			return false
		}
	}

	// Behind filter
	if filter.Behind != nil {
		if *filter.Behind && repo.Behind == 0 {
			return false
		}
		if !*filter.Behind && repo.Behind > 0 {
			return false
		}
	}

	return true
}

// matchesSearchTerms performs fuzzy matching against search terms
func matchesSearchTerms(repo *index.Repo, terms []string) bool {
	// Build searchable text
	searchable := strings.ToLower(strings.Join([]string{
		repo.Name,
		repo.Description,
		repo.RelPath,
		repo.RemoteURL,
		strings.Join(repo.Tags, " "),
	}, " "))

	// All terms must match
	for _, term := range terms {
		termLower := strings.ToLower(term)
		if !strings.Contains(searchable, termLower) {
			return false
		}
	}

	return true
}

// sortRepos sorts repositories by the specified field
func sortRepos(repos []*index.Repo, sortBy SortField) {
	switch sortBy {
	case SortByName:
		sort.Slice(repos, func(i, j int) bool {
			return repos[i].Name < repos[j].Name
		})
	case SortByLastCommit:
		sort.Slice(repos, func(i, j int) bool {
			return repos[i].LastCommitTime.After(repos[j].LastCommitTime)
		})
	case SortByPath:
		sort.Slice(repos, func(i, j int) bool {
			pathI := repos[i].Root + "/" + repos[i].RelPath
			pathJ := repos[j].Root + "/" + repos[j].RelPath
			return pathI < pathJ
		})
	case SortByLastScan:
		sort.Slice(repos, func(i, j int) bool {
			return repos[i].LastScanAt.After(repos[j].LastScanAt)
		})
	}
}

// FindUnique attempts to find a unique repository matching the query
// Returns the repo if unique match found, nil if no match, error if ambiguous
func FindUnique(idx *index.Index, query string) (*index.Repo, []*index.Repo, error) {
	query = strings.TrimSpace(query)

	// Try exact name match first
	repos := idx.GetByName(query)
	if len(repos) == 1 {
		return repos[0], nil, nil
	}
	if len(repos) > 1 {
		return nil, repos, nil
	}

	// Try exact path match
	if repo, ok := idx.Get(query); ok {
		return repo, nil, nil
	}

	// Try fuzzy search
	filter := &Filter{
		SearchTerms: []string{query},
	}
	matches := Query(idx, filter)

	if len(matches) == 0 {
		return nil, nil, nil
	}
	if len(matches) == 1 {
		return matches[0], nil, nil
	}

	// Ambiguous - return all matches
	return nil, matches, nil
}
