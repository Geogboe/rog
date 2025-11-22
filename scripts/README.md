# Scripts

This directory contains utility scripts for the rog project.

## create_v1_issues.go

A Go script that parses the roadmap document and creates GitHub issues for V1.1 and V1.2 features.

### Usage

#### Dry Run (Preview Issues)

Preview what issues will be created without actually creating them:

```bash
go run scripts/create_v1_issues.go docs/roadmap.md --dry-run
```

This will output all 21 issues (10 from V1.1 and 11 from V1.2) with their titles, labels, and descriptions.

#### Create Issues

To actually create the issues on GitHub, you need a GitHub Personal Access Token with `repo` scope:

```bash
# Using environment variable
export GITHUB_TOKEN="your_token_here"
go run scripts/create_v1_issues.go docs/roadmap.md

# Or using command line flag
go run scripts/create_v1_issues.go docs/roadmap.md --github-token=your_token_here
```

### What It Does

1. **Parses the roadmap**: Reads `docs/roadmap.md` and extracts all unchecked features from V1.1 and V1.2 sections
2. **Creates structured issues**: Each feature becomes a GitHub issue with:
   - Title from the feature description
   - Detailed body including context, acceptance criteria, and roadmap link
   - Appropriate labels (version, category, enhancement)
3. **Outputs issue URLs**: After creation, provides links to all created issues

### Features Extracted

The script identifies features by looking for lines matching `- [ ] Feature Description` in the following sections:

#### V1.1 - Polish & Performance (10 features)
- UX Improvements (4 features)
- Metadata Enhancements (3 features)
- CLI Polish (3 features)

#### V1.2 - Developer Experience (11 features)
- Workflow Integration (4 features)
- Smart Filtering (4 features)
- Workspace Support (3 features)

### Issue Structure

Each created issue includes:

- **Title**: The feature name (e.g., "Better error messages", "`rog doctor` command")
- **Labels**: 
  - Version label: `v1.1` or `v1.2`
  - Type label: `enhancement`
  - Category labels: `performance`, `ux`, `metadata`, `cli`, `workflow`, `filtering`, `workspace`
- **Body**:
  - Description section
  - Context (version, category, roadmap link)
  - Acceptance criteria checklist
  - Related section

### Adding Issues to a GitHub Project

After creating the issues, you'll need to manually add them to a GitHub Project:

1. Go to https://github.com/Geogboe/rog/projects
2. Create a new project called "v1" or open an existing one
3. Add the created issues to the project
4. Organize them by milestone (V1.1 vs V1.2) or category

### Requirements

- Go 1.16 or later
- GitHub Personal Access Token with `repo` scope (for creating issues)
- Network access to GitHub API

### Error Handling

The script will:
- Validate that the roadmap file exists
- Require a GitHub token when not in dry-run mode
- Report errors for individual issue creation failures
- Continue creating remaining issues even if one fails

### Example Output

```
Found 21 features to convert into issues

[1/21] Creating issue: Better error messages
  ✓ Created: https://github.com/Geogboe/rog/issues/123
[2/21] Creating issue: Progress bars for long scans
  ✓ Created: https://github.com/Geogboe/rog/issues/124
...

=== Summary ===
Created 21 issues:
  - https://github.com/Geogboe/rog/issues/123
  - https://github.com/Geogboe/rog/issues/124
  ...

Next steps:
1. Go to https://github.com/Geogboe/rog/projects
2. Create or open the 'v1' project
3. Add the created issues to the project
```
