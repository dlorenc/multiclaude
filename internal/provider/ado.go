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
// URL encoding for the clone URL.
func buildADORepoInfo(org, project, repo string) *RepoInfo {
	// Decode values for storage (used in display and API calls that encode themselves)
	decodedOrg := urlDecodeOrKeep(org)
	decodedProject := urlDecodeOrKeep(project)
	decodedRepo := urlDecodeOrKeep(repo)

	// Clone URL uses URL-encoded values for proper HTTP handling
	return &RepoInfo{
		Provider: TypeAzureDevOps,
		Owner:    decodedOrg,
		Project:  decodedProject,
		Repo:     decodedRepo,
		CloneURL: fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s",
			url.PathEscape(decodedOrg),
			url.PathEscape(decodedProject),
			url.PathEscape(decodedRepo)),
	}
}

// ParseURL parses an Azure DevOps repository URL.
func (a *AzureDevOps) ParseURL(repoURL string) (*RepoInfo, error) {
	repoURL = normalizeGitURL(repoURL)

	// Try HTTPS format: https://dev.azure.com/{org}/{project}/_git/{repo}
	if matches := adoHTTPSRegex.FindStringSubmatch(repoURL); matches != nil {
		return buildADORepoInfo(matches[1], matches[2], matches[3]), nil
	}

	// Try with .git suffix
	if matches := adoHTTPSRegex.FindStringSubmatch(repoURL + ".git"); matches != nil {
		return buildADORepoInfo(matches[1], matches[2], matches[3]), nil
	}

	// Try SSH format: git@ssh.dev.azure.com:v3/{org}/{project}/{repo}
	if matches := adoSSHRegex.FindStringSubmatch(repoURL); matches != nil {
		return buildADORepoInfo(matches[1], matches[2], matches[3]), nil
	}

	// Try legacy format: https://{org}.visualstudio.com/{project}/_git/{repo}
	if matches := adoLegacyRegex.FindStringSubmatch(repoURL); matches != nil {
		return buildADORepoInfo(matches[1], matches[2], matches[3]), nil
	}

	// Try legacy default project format: https://{org}.visualstudio.com/_git/{repo}
	if matches := adoLegacyDefaultRegex.FindStringSubmatch(repoURL); matches != nil {
		return buildADORepoInfo(matches[1], matches[1], matches[2]), nil
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
	pat := os.Getenv("AZURE_DEVOPS_PAT")
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

// getAPIBaseURL returns the base URL for Azure DevOps REST API calls.
func (a *AzureDevOps) getAPIBaseURL() string {
	return fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s",
		url.PathEscape(a.Organization), url.PathEscape(a.Project), url.PathEscape(a.Repo))
}

// PRListCommand returns the curl command to list PRs in Azure DevOps.
func (a *AzureDevOps) PRListCommand(label string, authorFilter string) string {
	// Azure DevOps uses REST API with curl
	// Note: ADO doesn't have labels like GitHub; we use tags or search queries
	apiURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullrequests?api-version=7.0&searchCriteria.status=active",
		url.PathEscape(a.Organization), url.PathEscape(a.Project), url.PathEscape(a.Repo))

	if authorFilter == "@me" {
		// ADO uses creatorId, but for simplicity we filter client-side
		apiURL += "&searchCriteria.creatorId={user-id}"
	}

	return fmt.Sprintf(`curl -s -u ":$AZURE_DEVOPS_PAT" "%s"`, apiURL)
}

// PRCreateCommand returns the curl command to create a PR in Azure DevOps.
func (a *AzureDevOps) PRCreateCommand(targetRepo, headBranch string) string {
	// Azure DevOps PR creation via REST API
	apiURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullrequests?api-version=7.0",
		url.PathEscape(a.Organization), url.PathEscape(a.Project), url.PathEscape(a.Repo))

	return fmt.Sprintf(`curl -s -u ":$AZURE_DEVOPS_PAT" -X POST "%s" -H "Content-Type: application/json" -d '{
  "sourceRefName": "refs/heads/%s",
  "targetRefName": "refs/heads/main",
  "title": "PR Title",
  "description": "PR Description"
}'`, apiURL, headBranch)
}

// PRViewCommand returns the curl command to view a PR in Azure DevOps.
func (a *AzureDevOps) PRViewCommand(prNumber int, jsonFields string) string {
	apiURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/pullrequests/%d?api-version=7.0",
		url.PathEscape(a.Organization), url.PathEscape(a.Project), prNumber)

	return fmt.Sprintf(`curl -s -u ":$AZURE_DEVOPS_PAT" "%s"`, apiURL)
}

// PRChecksCommand returns the curl command to view PR build status in Azure DevOps.
func (a *AzureDevOps) PRChecksCommand(prNumber int) string {
	// Azure DevOps uses build status on the PR
	apiURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullrequests/%d/statuses?api-version=7.0",
		url.PathEscape(a.Organization), url.PathEscape(a.Project), url.PathEscape(a.Repo), prNumber)

	return fmt.Sprintf(`curl -s -u ":$AZURE_DEVOPS_PAT" "%s"`, apiURL)
}

// PRCommentCommand returns the curl command to comment on a PR in Azure DevOps.
func (a *AzureDevOps) PRCommentCommand(prNumber int, body string) string {
	apiURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullrequests/%d/threads?api-version=7.0",
		url.PathEscape(a.Organization), url.PathEscape(a.Project), url.PathEscape(a.Repo), prNumber)

	// Escape body for JSON
	escapedBody := strings.ReplaceAll(body, `"`, `\"`)
	escapedBody = strings.ReplaceAll(escapedBody, "\n", `\n`)

	return fmt.Sprintf(`curl -s -u ":$AZURE_DEVOPS_PAT" -X POST "%s" -H "Content-Type: application/json" -d '{"comments":[{"content":"%s"}]}'`,
		apiURL, escapedBody)
}

// PREditCommand returns the curl command to edit a PR in Azure DevOps.
func (a *AzureDevOps) PREditCommand(prNumber int, addLabel string) string {
	// Azure DevOps uses labels/tags differently - this adds a tag
	apiURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullrequests/%d/labels?api-version=7.0",
		url.PathEscape(a.Organization), url.PathEscape(a.Project), url.PathEscape(a.Repo), prNumber)

	return fmt.Sprintf(`curl -s -u ":$AZURE_DEVOPS_PAT" -X POST "%s" -H "Content-Type: application/json" -d '{"name":"%s"}'`,
		apiURL, addLabel)
}

// PRMergeCommand returns the curl command to complete (merge) a PR in Azure DevOps.
func (a *AzureDevOps) PRMergeCommand(prNumber int) string {
	apiURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullrequests/%d?api-version=7.0",
		url.PathEscape(a.Organization), url.PathEscape(a.Project), url.PathEscape(a.Repo), prNumber)

	return fmt.Sprintf(`curl -s -u ":$AZURE_DEVOPS_PAT" -X PATCH "%s" -H "Content-Type: application/json" -d '{"status":"completed","completionOptions":{"deleteSourceBranch":true}}'`,
		apiURL)
}

// RunListCommand returns the curl command to list pipeline runs in Azure DevOps.
func (a *AzureDevOps) RunListCommand(branch string, limit int) string {
	// Azure DevOps uses pipelines API for CI runs
	apiURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/pipelines/runs?api-version=7.0",
		url.PathEscape(a.Organization), url.PathEscape(a.Project))

	if branch != "" {
		apiURL += fmt.Sprintf("&branchName=refs/heads/%s", url.PathEscape(branch))
	}

	return fmt.Sprintf(`curl -s -u ":$AZURE_DEVOPS_PAT" "%s"`, apiURL)
}

// APICommand returns the curl command to call the Azure DevOps REST API.
func (a *AzureDevOps) APICommand(endpoint, jqFilter string) string {
	var apiURL string
	if strings.HasPrefix(endpoint, "https://") {
		apiURL = endpoint
	} else {
		apiURL = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/%s",
			url.PathEscape(a.Organization), url.PathEscape(a.Project), endpoint)
	}

	cmd := fmt.Sprintf(`curl -s -u ":$AZURE_DEVOPS_PAT" "%s"`, apiURL)
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

// ValidatePAT checks if the AZURE_DEVOPS_PAT environment variable is set.
func ValidatePAT() error {
	if os.Getenv("AZURE_DEVOPS_PAT") == "" {
		return fmt.Errorf("AZURE_DEVOPS_PAT environment variable is not set. " +
			"Please set it to your Azure DevOps Personal Access Token")
	}
	return nil
}
