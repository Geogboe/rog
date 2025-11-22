package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

// Issue represents a GitHub issue
type Issue struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels"`
}

// Feature represents a feature from the roadmap
type Feature struct {
	Title       string
	Description string
	Category    string
	Version     string
	Labels      []string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run create_v1_issues.go <roadmap-file> [--dry-run] [--github-token=TOKEN]")
		fmt.Println("\nOptions:")
		fmt.Println("  --dry-run          Print issues without creating them")
		fmt.Println("  --github-token     GitHub personal access token (or use GITHUB_TOKEN env var)")
		os.Exit(1)
	}

	roadmapFile := os.Args[1]
	dryRun := false
	githubToken := os.Getenv("GITHUB_TOKEN")

	// Parse command line arguments
	for i := 2; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--dry-run" {
			dryRun = true
		} else if strings.HasPrefix(arg, "--github-token=") {
			githubToken = strings.TrimPrefix(arg, "--github-token=")
		}
	}

	// Read and parse roadmap
	features, err := parseRoadmap(roadmapFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing roadmap: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d features to convert into issues\n\n", len(features))

	if dryRun {
		fmt.Println("=== DRY RUN MODE - Issues will NOT be created ===\n")
		for i, feature := range features {
			fmt.Printf("--- Issue %d ---\n", i+1)
			fmt.Printf("Title: %s\n", feature.Title)
			fmt.Printf("Labels: %s\n", strings.Join(feature.Labels, ", "))
			fmt.Printf("Body:\n%s\n\n", feature.Description)
		}
		return
	}

	// Create issues
	if githubToken == "" {
		fmt.Println("Error: GitHub token not provided. Use --github-token or set GITHUB_TOKEN environment variable")
		os.Exit(1)
	}

	repo := "Geogboe/rog"
	createdIssues := []string{}

	for i, feature := range features {
		fmt.Printf("[%d/%d] Creating issue: %s\n", i+1, len(features), feature.Title)
		
		issueNum, err := createGitHubIssue(repo, githubToken, feature)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating issue: %v\n", err)
			continue
		}
		
		issueURL := fmt.Sprintf("https://github.com/%s/issues/%d", repo, issueNum)
		fmt.Printf("  ✓ Created: %s\n", issueURL)
		createdIssues = append(createdIssues, issueURL)
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Created %d issues:\n", len(createdIssues))
	for _, url := range createdIssues {
		fmt.Printf("  - %s\n", url)
	}
	fmt.Println("\nNext steps:")
	fmt.Println("1. Go to https://github.com/Geogboe/rog/projects")
	fmt.Println("2. Create or open the 'v1' project")
	fmt.Println("3. Add the created issues to the project")
}

func parseRoadmap(filename string) ([]Feature, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var features []Feature
	scanner := bufio.NewScanner(file)
	
	var currentVersion string
	var currentCategory string
	var inV1Section bool
	
	// Regex patterns
	versionPattern := regexp.MustCompile(`^## (V\d+\.\d+)`)
	categoryPattern := regexp.MustCompile(`^#### (.+)$`)
	featurePattern := regexp.MustCompile(`^- \[ \] (.+)$`)

	for scanner.Scan() {
		line := scanner.Text()

		// Check for version header (V1.1, V1.2, V2.0, etc.)
		if matches := versionPattern.FindStringSubmatch(line); matches != nil {
			version := matches[1]
			if version == "V1.1" || version == "V1.2" {
				inV1Section = true
				currentVersion = version
			} else {
				// Any other version (V2.0, V3.0, etc.) means we're out of V1
				inV1Section = false
			}
			continue
		}

		if !inV1Section {
			continue
		}

		// Check for category header
		if matches := categoryPattern.FindStringSubmatch(line); matches != nil {
			currentCategory = matches[1]
			continue
		}

		// Check for feature item
		if matches := featurePattern.FindStringSubmatch(line); matches != nil {
			featureText := matches[1]
			
			// Parse inline code blocks and descriptions
			title := featureText
			description := ""
			
			// If there's a dash or parenthetical, split into title and description
			if strings.Contains(featureText, " - ") {
				parts := strings.SplitN(featureText, " - ", 2)
				title = parts[0]
				description = parts[1]
			} else if idx := strings.Index(featureText, " ("); idx != -1 {
				title = featureText[:idx]
				description = featureText[idx+1:]
				description = strings.TrimPrefix(description, "(")
				description = strings.TrimSuffix(description, ")")
			}

			labels := []string{strings.ToLower(currentVersion), "enhancement"}
			
			// Add category-based labels
			categoryLower := strings.ToLower(currentCategory)
			if strings.Contains(categoryLower, "performance") {
				labels = append(labels, "performance")
			}
			if strings.Contains(categoryLower, "ux") || strings.Contains(categoryLower, "experience") {
				labels = append(labels, "ux")
			}
			if strings.Contains(categoryLower, "metadata") {
				labels = append(labels, "metadata")
			}
			if strings.Contains(categoryLower, "cli") {
				labels = append(labels, "cli")
			}
			if strings.Contains(categoryLower, "workflow") {
				labels = append(labels, "workflow")
			}
			if strings.Contains(categoryLower, "filter") {
				labels = append(labels, "filtering")
			}
			if strings.Contains(categoryLower, "workspace") {
				labels = append(labels, "workspace")
			}

			feature := Feature{
				Title:       title,
				Description: buildIssueBody(title, description, currentCategory, currentVersion),
				Category:    currentCategory,
				Version:     currentVersion,
				Labels:      labels,
			}
			features = append(features, feature)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return features, nil
}

func buildIssueBody(title, description, category, version string) string {
	var body strings.Builder
	
	body.WriteString(fmt.Sprintf("## Description\n\n"))
	if description != "" {
		body.WriteString(description)
		body.WriteString("\n\n")
	} else {
		body.WriteString(fmt.Sprintf("Implement **%s** as part of the %s roadmap.\n\n", title, version))
	}
	
	body.WriteString(fmt.Sprintf("## Context\n\n"))
	body.WriteString(fmt.Sprintf("- **Version**: %s\n", version))
	body.WriteString(fmt.Sprintf("- **Category**: %s\n", category))
	body.WriteString(fmt.Sprintf("- **Source**: [Roadmap](https://github.com/Geogboe/rog/blob/main/docs/roadmap.md)\n\n"))
	
	body.WriteString("## Acceptance Criteria\n\n")
	body.WriteString("- [ ] Feature implemented according to roadmap specifications\n")
	body.WriteString("- [ ] Tests added/updated\n")
	body.WriteString("- [ ] Documentation updated\n")
	body.WriteString("- [ ] Performance benchmarks meet targets (if applicable)\n\n")
	
	body.WriteString("## Related\n\n")
	body.WriteString("Part of the v1 roadmap implementation.\n")
	
	return body.String()
}

func createGitHubIssue(repo, token string, feature Feature) (int, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/issues", repo)
	
	issue := Issue{
		Title:  feature.Title,
		Body:   feature.Description,
		Labels: feature.Labels,
	}
	
	jsonData, err := json.Marshal(issue)
	if err != nil {
		return 0, err
	}
	
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, err
	}
	
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	
	if resp.StatusCode != http.StatusCreated {
		return 0, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}
	
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}
	
	issueNum := int(result["number"].(float64))
	return issueNum, nil
}
