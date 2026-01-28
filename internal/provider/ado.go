package provider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// Azure DevOps URL patterns
var (
	// HTTPS: https://dev.azure.com/{org}/{project}/_git/{repo}
	adoHTTPSRegex = regexp.MustCompile(`^https://dev\.azure\.com/([^/]+)/([^/]+)/_git/([^/.]+)(?:\.git)?$`)
	// SSH: git@ssh.dev.azure.com:v3/{org}/{project}/{repo}
	adoSSHRegex = regexp.MustCompile(`^git@ssh\.dev\.azure\.com:v3/([^/]+)/([^/]+)/([^/.]+)(?:\.git)?$`)
	// Legacy HTTPS: https://{org}.visualstudio.com/{project}/_git/{repo}
	adoLegacyRegex = regexp.MustCompile(`^https://([^.]+)\.visualstudio\.com/([^/]+)/_git/([^/.]+)(?:\.git)?$`)
	// Legacy default project: https://{org}.visualstudio.com/_git/{repo}
	adoLegacyDefaultRegex = regexp.MustCompile(`^https://([^.]+)\.visualstudio\.com/_git/([^/.]+)(?:\.git)?$`)
)

// AzureDevOps implements the Provider interface for Azure DevOps.
type AzureDevOps struct {
	// Organization is the ADO organization name.
	Organization string
	// Project is the ADO project name.
	Project string
	// Repo is the repository name.
	Repo string
}

// NewAzureDevOps creates a new Azure DevOps provider.
func NewAzureDevOps() *AzureDevOps {
	return &AzureDevOps{}
}

// NewAzureDevOpsWithConfig creates a new Azure DevOps provider with configuration.
func NewAzureDevOpsWithConfig(org, project, repo string) *AzureDevOps {
	return &AzureDevOps{
		Organization: org,
		Project:      project,
		Repo:         repo,
	}
}

// Name returns the provider type.
func (a *AzureDevOps) Name() Type {
	return TypeAzureDevOps
}

// urlDecodeOrKeep URL-decodes a string, returning the original if decoding fails.
func urlDecodeOrKeep(s string) string {
	decoded, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}

// buildADORepoInfo creates a RepoInfo with proper URL decoding for display and
// URL encoding for the clone URL. If useSSH is true, generates an SSH clone URL
// instead of HTTPS, preserving the user's preference for SSH authentication.
func buildADORepoInfo(org, project, repo string, useSSH bool) *RepoInfo {
	// Decode values for storage (used in display and API calls that encode themselves)
	decodedOrg := urlDecodeOrKeep(org)
	decodedProject := urlDecodeOrKeep(project)
	decodedRepo := urlDecodeOrKeep(repo)

	var cloneURL string
	if useSSH {
		// SSH URL format: git@ssh.dev.azure.com:v3/{org}/{project}/{repo}
		// SSH URLs use URL-encoded values for projects with spaces/special chars
		cloneURL = fmt.Sprintf("git@ssh.dev.azure.com:v3/%s/%s/%s",
			url.PathEscape(decodedOrg),
			url.PathEscape(decodedProject),
			url.PathEscape(decodedRepo))
	} else {
		// HTTPS URL uses URL-encoded values for proper HTTP handling
		cloneURL = fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s",
			url.PathEscape(decodedOrg),
			url.PathEscape(decodedProject),
			url.PathEscape(decodedRepo))
	}

	return &RepoInfo{
		Provider: TypeAzureDevOps,
		Owner:    decodedOrg,
		Project:  decodedProject,
		Repo:     decodedRepo,
		CloneURL: cloneURL,
	}
}

// ParseURL parses an Azure DevOps repository URL.
// If the input URL is SSH format, the returned CloneURL will also be SSH format.
// If the input URL is HTTPS format, the returned CloneURL will be HTTPS format.
func (a *AzureDevOps) ParseURL(repoURL string) (*RepoInfo, error) {
	repoURL = normalizeGitURL(repoURL)

	// Try HTTPS format: https://dev.azure.com/{org}/{project}/_git/{repo}
	if matches := adoHTTPSRegex.FindStringSubmatch(repoURL); matches != nil {
		return buildADORepoInfo(matches[1], matches[2], matches[3], false), nil
	}

	// Try with .git suffix
	if matches := adoHTTPSRegex.FindStringSubmatch(repoURL + ".git"); matches != nil {
		return buildADORepoInfo(matches[1], matches[2], matches[3], false), nil
	}

	// Try SSH format: git@ssh.dev.azure.com:v3/{org}/{project}/{repo}
	// Preserve SSH format in the clone URL to respect user's authentication preference
	if matches := adoSSHRegex.FindStringSubmatch(repoURL); matches != nil {
		return buildADORepoInfo(matches[1], matches[2], matches[3], true), nil
	}

	// Try legacy format: https://{org}.visualstudio.com/{project}/_git/{repo}
	if matches := adoLegacyRegex.FindStringSubmatch(repoURL); matches != nil {
		return buildADORepoInfo(matches[1], matches[2], matches[3], false), nil
	}

	// Try legacy default project format: https://{org}.visualstudio.com/_git/{repo}
	if matches := adoLegacyDefaultRegex.FindStringSubmatch(repoURL); matches != nil {
		return buildADORepoInfo(matches[1], matches[1], matches[2], false), nil
	}

	return nil, fmt.Errorf("unable to parse Azure DevOps URL: %s", repoURL)
}

// DetectFork checks if an Azure DevOps repository is a fork.
// Note: Azure DevOps forks work differently than GitHub forks.
// ADO forks are typically in the same project and use the API to detect.
func (a *AzureDevOps) DetectFork(repoPath string) (*ForkInfo, error) {
	// First check for upstream remote (common convention)
	upstreamURL, err := getRemoteURL(repoPath, "upstream")
	if err == nil && upstreamURL != "" {
		info, err := a.ParseURL(upstreamURL)
		if err == nil {
			return &ForkInfo{
				IsFork:        true,
				UpstreamOwner: info.Owner,
				UpstreamRepo:  info.Repo,
				UpstreamURL:   info.CloneURL,
			}, nil
		}
	}

	// Get origin remote URL
	originURL, err := getRemoteURL(repoPath, "origin")
	if err != nil {
		return nil, fmt.Errorf("failed to get origin remote: %w", err)
	}

	// Parse origin URL
	originInfo, err := a.ParseURL(originURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse origin URL: %w", err)
	}

	// Try to detect via Azure DevOps API
	forkInfo, err := a.detectForkViaAPI(originInfo.Owner, originInfo.Project, originInfo.Repo)
	if err == nil && forkInfo.IsFork {
		return forkInfo, nil
	}

	return &ForkInfo{IsFork: false}, nil
}

// detectForkViaAPI uses the Azure DevOps REST API to check if a repo is a fork.
func (a *AzureDevOps) detectForkViaAPI(org, project, repo string) (*ForkInfo, error) {
	pat := os.Getenv("AZURE_DEVOPS_EXT_PAT")
	if pat == "" {
		// No PAT, can't check API
		return &ForkInfo{IsFork: false}, nil
	}

	// Azure DevOps API endpoint for repository info (URL-encode parameters with spaces/special chars)
	apiURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s?api-version=7.0",
		url.PathEscape(org), url.PathEscape(project), url.PathEscape(repo))

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Azure DevOps uses Basic auth with PAT (username is empty)
	auth := base64.StdEncoding.EncodeToString([]byte(":" + pat))
	req.Header.Set("Authorization", "Basic "+auth)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		IsFork     bool `json:"isFork"`
		ParentRepo *struct {
			Name    string `json:"name"`
			Project struct {
				Name string `json:"name"`
			} `json:"project"`
			RemoteURL string `json:"remoteUrl"`
		} `json:"parentRepository"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	info := &ForkInfo{
		IsFork: result.IsFork,
	}

	if result.IsFork && result.ParentRepo != nil {
		info.UpstreamOwner = org // ADO forks are typically in the same org
		info.UpstreamRepo = result.ParentRepo.Name
		info.UpstreamURL = result.ParentRepo.RemoteURL
	}

	return info, nil
}

// PRListCommand returns the az devops CLI command to list PRs in Azure DevOps.
// Output is formatted to match GitHub's `gh pr list` format:
// number, title, branch, state, author.
func (a *AzureDevOps) PRListCommand(label string, authorFilter string) string {
	// Use az repos pr list command
	cmd := fmt.Sprintf(`az repos pr list --organization "https://dev.azure.com/%s" --project "%s" --repository "%s" --status active`,
		a.Organization, a.Project, a.Repo)

	// Add author filter if specified (az repos pr list supports --creator option)
	if authorFilter != "" && authorFilter != "@me" {
		cmd += fmt.Sprintf(` --creator "%s"`, authorFilter)
	}

	// Format output similar to gh pr list using jq
	jqFilter := `[.[] | {number: .pullRequestId, title: .title, branch: (.sourceRefName | sub("refs/heads/"; "")), state: .status, author: .createdBy.displayName}]`
	cmd += fmt.Sprintf(` | jq '%s'`, jqFilter)

	return cmd
}

// PRCreateCommand returns the az devops CLI command to create a PR in Azure DevOps.
// Uses placeholder variables: $PR_TITLE, $PR_BODY, $TARGET_BRANCH.
// Example usage: PR_TITLE="My PR" PR_BODY="Description" TARGET_BRANCH="main" eval "$cmd"
func (a *AzureDevOps) PRCreateCommand(targetRepo, headBranch string) string {
	// Use az repos pr create command
	targetBranch := "main"
	if targetRepo != "" {
		targetBranch = "${TARGET_BRANCH:-main}"
	}

	return fmt.Sprintf(`az repos pr create --organization "https://dev.azure.com/%s" --project "%s" --repository "%s" --source-branch "%s" --target-branch "%s" --title "${PR_TITLE:-PR from %s}" --description "${PR_BODY:-}"`,
		a.Organization, a.Project, a.Repo, headBranch, targetBranch, headBranch)
}

// PRViewCommand returns the az devops CLI command to view a PR in Azure DevOps.
// If jsonFields is specified, it filters the output to those fields using jq.
// Supported fields mirror GitHub's gh pr view --json: title, state, author, body,
// baseRefName, headRefName, url, number, reviewDecision, isDraft.
func (a *AzureDevOps) PRViewCommand(prNumber int, jsonFields string) string {
	// Use az repos pr show command
	cmd := fmt.Sprintf(`az repos pr show --id %d --organization "https://dev.azure.com/%s" --project "%s"`,
		prNumber, a.Organization, a.Project)

	if jsonFields == "" {
		return cmd
	}

	// Map GitHub field names to ADO equivalents and create a jq filter
	// This provides compatibility with agents expecting GitHub-style output
	jqFilter := `{
  number: .pullRequestId,
  title: .title,
  state: .status,
  author: .createdBy.displayName,
  body: .description,
  baseRefName: (.targetRefName | sub("refs/heads/"; "")),
  headRefName: (.sourceRefName | sub("refs/heads/"; "")),
  url: .url,
  isDraft: .isDraft,
  mergeStatus: .mergeStatus,
  reviewDecision: (if ([.reviewers[] | select(.vote <= -10)] | length > 0) then "CHANGES_REQUESTED"
                   elif ([.reviewers[] | select(.vote >= 10)] | length > 0) then "APPROVED"
                   else "REVIEW_REQUIRED" end)
}`

	return fmt.Sprintf(`%s | jq '%s'`, cmd, jqFilter)
}

// PRChecksCommand returns the az devops CLI command to view PR status in Azure DevOps.
// This queries the PR itself and formats output similar to GitHub's `gh pr checks`,
// showing merge status and reviewer votes. Unlike GitHub where CI status is tightly
// coupled to PRs, ADO uses policy evaluations and external status checks.
func (a *AzureDevOps) PRChecksCommand(prNumber int) string {
	// Use az repos pr show command which contains merge status and reviewer information
	cmd := fmt.Sprintf(`az repos pr show --id %d --organization "https://dev.azure.com/%s" --project "%s"`,
		prNumber, a.Organization, a.Project)

	// Format output similar to gh pr checks: show merge status and reviewer votes
	jqFilter := `{
  mergeStatus: .mergeStatus,
  status: .status,
  isDraft: .isDraft,
  reviewers: [.reviewers[] | {
    name: .displayName,
    vote: (if .vote == 10 then "approved"
           elif .vote == 5 then "approved-with-suggestions"
           elif .vote == 0 then "no-vote"
           elif .vote == -5 then "waiting"
           elif .vote == -10 then "rejected"
           else "unknown" end),
    isRequired: .isRequired
  }],
  hasRejections: ([.reviewers[] | select(.vote <= -10)] | length > 0),
  canMerge: (.mergeStatus == "succeeded" and .status == "active" and (.isDraft | not) and ([.reviewers[] | select(.vote <= -10)] | length == 0))
}`

	return fmt.Sprintf(`%s | jq '%s'`, cmd, jqFilter)
}

// PRCommentCommand returns the curl command to comment on a PR in Azure DevOps.
// Creates a new comment thread on the PR (ADO comments are organized in threads).
// Note: The az devops CLI doesn't have a direct comment command, so we use curl.
func (a *AzureDevOps) PRCommentCommand(prNumber int, body string) string {
	apiURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullrequests/%d/threads?api-version=7.0",
		url.PathEscape(a.Organization), url.PathEscape(a.Project), url.PathEscape(a.Repo), prNumber)

	// Escape body for JSON - handle quotes, newlines, backslashes, and tabs
	escapedBody := strings.ReplaceAll(body, `\`, `\\`)
	escapedBody = strings.ReplaceAll(escapedBody, `"`, `\"`)
	escapedBody = strings.ReplaceAll(escapedBody, "\n", `\n`)
	escapedBody = strings.ReplaceAll(escapedBody, "\t", `\t`)
	escapedBody = strings.ReplaceAll(escapedBody, "\r", `\r`)

	// Use double quotes for the JSON body to allow shell variable expansion if needed
	return fmt.Sprintf(`curl -s -u ":$AZURE_DEVOPS_EXT_PAT" -X POST "%s" -H "Content-Type: application/json" -d "{\"comments\":[{\"commentType\":1,\"content\":\"%s\"}],\"status\":1}"`,
		apiURL, escapedBody)
}

// PREditCommand returns the curl command to edit a PR in Azure DevOps.
// Note: The az devops CLI doesn't support adding labels directly, so we use curl.
func (a *AzureDevOps) PREditCommand(prNumber int, addLabel string) string {
	// Azure DevOps uses labels/tags differently - this adds a tag
	apiURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullrequests/%d/labels?api-version=7.0",
		url.PathEscape(a.Organization), url.PathEscape(a.Project), url.PathEscape(a.Repo), prNumber)

	return fmt.Sprintf(`curl -s -u ":$AZURE_DEVOPS_EXT_PAT" -X POST "%s" -H "Content-Type: application/json" -d '{"name":"%s"}'`,
		apiURL, addLabel)
}

// PRMergeCommand returns the az devops CLI command to complete (merge) a PR in Azure DevOps.
// Uses squash merge and deletes the source branch.
func (a *AzureDevOps) PRMergeCommand(prNumber int) string {
	// Use az repos pr update to complete the PR
	// --squash enables squash merge, --delete-source-branch removes the branch after merge
	return fmt.Sprintf(`az repos pr update --id %d --organization "https://dev.azure.com/%s" --project "%s" --status completed --squash --delete-source-branch`,
		prNumber, a.Organization, a.Project)
}

// RunListCommand returns the az devops CLI command to list CI builds in Azure DevOps.
// Filters to the specific repository to match GitHub's behavior.
func (a *AzureDevOps) RunListCommand(branch string, limit int) string {
	// Use az pipelines runs list command
	cmd := fmt.Sprintf(`az pipelines runs list --organization "https://dev.azure.com/%s" --project "%s"`,
		a.Organization, a.Project)

	if branch != "" {
		cmd += fmt.Sprintf(` --branch "%s"`, branch)
	}

	// Filter by repository name using jq to match GitHub's repo-scoped behavior
	jqFilter := fmt.Sprintf(`[.[] | select(.repository.name == "%s")]`, a.Repo)
	if limit > 0 {
		jqFilter += fmt.Sprintf(" | .[:%d]", limit)
	}

	return fmt.Sprintf(`%s | jq '%s'`, cmd, jqFilter)
}

// APICommand returns the curl command to call the Azure DevOps REST API.
// Note: For raw API calls, we use curl since the az devops CLI may not expose all endpoints.
func (a *AzureDevOps) APICommand(endpoint, jqFilter string) string {
	var apiURL string
	if strings.HasPrefix(endpoint, "https://") {
		apiURL = endpoint
	} else {
		apiURL = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/%s",
			url.PathEscape(a.Organization), url.PathEscape(a.Project), endpoint)
	}

	cmd := fmt.Sprintf(`curl -s -u ":$AZURE_DEVOPS_EXT_PAT" "%s"`, apiURL)
	if jqFilter != "" {
		cmd += fmt.Sprintf(" | jq %q", jqFilter)
	}
	return cmd
}

// ReviewCommand returns a note that review agents need ADO-specific handling.
func (a *AzureDevOps) ReviewCommand(prURL string) string {
	// For ADO, we use the same multiclaude review command but the URL format is different
	return fmt.Sprintf("multiclaude review %s", prURL)
}

// ValidatePAT checks if the AZURE_DEVOPS_EXT_PAT environment variable is set.
// This is the standard environment variable used by the Azure DevOps CLI extension.
func ValidatePAT() error {
	if os.Getenv("AZURE_DEVOPS_EXT_PAT") == "" {
		return fmt.Errorf("AZURE_DEVOPS_EXT_PAT environment variable is not set. " +
			"Please set it to your Azure DevOps Personal Access Token")
	}
	return nil
}
