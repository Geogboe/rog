package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
)

// Client handles LLM API interactions
type Client struct {
	endpoint string
	model    string
	apiKey   string
	extraInstructions string
	httpClient *http.Client
}

// EnrichmentResult represents the result of LLM enrichment
type EnrichmentResult struct {
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

// NewClient creates a new LLM client
func NewClient(cfg *config.LLMConfig) *Client {
	if cfg == nil {
		return nil
	}

	return &Client{
		endpoint:          cfg.Endpoint,
		model:             cfg.Model,
		apiKey:            cfg.APIKey,
		extraInstructions: cfg.ExtraInstructions,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Enrich generates description and tags for a repository
func (c *Client) Enrich(repo *index.Repo) (*EnrichmentResult, error) {
	if c == nil {
		return nil, fmt.Errorf("LLM client not configured")
	}

	// Build the prompt
	userPrompt := c.buildPrompt(repo)

	// Make API request
	result, err := c.complete(userPrompt)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// buildPrompt builds the user prompt for enrichment
func (c *Client) buildPrompt(repo *index.Repo) string {
	var sb strings.Builder

	// Add extra instructions if provided
	if c.extraInstructions != "" {
		sb.WriteString(c.extraInstructions)
		sb.WriteString("\n\n")
	}

	// Repository information
	sb.WriteString(fmt.Sprintf("Repository: %s\n", repo.Name))
	sb.WriteString(fmt.Sprintf("Location: %s/%s\n", repo.Root, repo.RelPath))
	if repo.PrimaryLanguage != "" {
		sb.WriteString(fmt.Sprintf("Language: %s\n", repo.PrimaryLanguage))
	}
	if repo.Host != "" {
		sb.WriteString(fmt.Sprintf("Host: %s\n", repo.Host))
	}
	sb.WriteString("\n")

	// Try to read README
	readme := readREADME(repo.AbsPath)
	if readme != "" {
		sb.WriteString("README (first 500 chars):\n")
		if len(readme) > 500 {
			readme = readme[:500] + "..."
		}
		sb.WriteString(readme)
		sb.WriteString("\n\n")
	}

	// List top-level items
	items := listTopLevelItems(repo.AbsPath)
	if len(items) > 0 {
		sb.WriteString("Top-level structure:\n")
		for _, item := range items {
			sb.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}

	return sb.String()
}

// complete makes a completion request to the LLM API
func (c *Client) complete(userPrompt string) (*EnrichmentResult, error) {
	// Build request body (OpenAI-compatible format)
	reqBody := map[string]interface{}{
		"model": c.model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": getSystemPrompt(),
			},
			{
				"role":    "user",
				"content": userPrompt,
			},
		},
		"temperature": 0.3,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", c.endpoint+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// Make request with retry
	var resp *http.Response
	for attempt := 0; attempt < 3; attempt++ {
		resp, err = c.httpClient.Do(req)
		if err == nil {
			break
		}
		if attempt < 2 {
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no completion choices returned")
	}

	content := apiResp.Choices[0].Message.Content

	// Parse the JSON response
	var result EnrichmentResult

	// Try to extract JSON from markdown code blocks if present
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		var jsonLines []string
		inBlock := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				inBlock = !inBlock
				continue
			}
			if inBlock {
				jsonLines = append(jsonLines, line)
			}
		}
		content = strings.Join(jsonLines, "\n")
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w (content: %s)", err, content)
	}

	// Validate result
	if len(result.Description) > 140 {
		result.Description = result.Description[:140]
	}
	if len(result.Tags) > 7 {
		result.Tags = result.Tags[:7]
	}

	return &result, nil
}

// readREADME attempts to read a README file from the repository
func readREADME(repoPath string) string {
	candidates := []string{"README.md", "README.MD", "readme.md", "README", "README.txt"}

	for _, name := range candidates {
		path := filepath.Join(repoPath, name)
		if data, err := os.ReadFile(path); err == nil {
			return string(data)
		}
	}

	return ""
}

// listTopLevelItems lists top-level files and directories
func listTopLevelItems(repoPath string) []string {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return nil
	}

	var items []string
	for _, entry := range entries {
		// Skip .git
		if entry.Name() == ".git" {
			continue
		}
		// Skip hidden files
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		if entry.IsDir() {
			items = append(items, entry.Name()+"/")
		} else {
			items = append(items, entry.Name())
		}

		// Limit to 20 items
		if len(items) >= 20 {
			break
		}
	}

	return items
}
