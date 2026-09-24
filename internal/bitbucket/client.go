package bitbucket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const baseURL = "https://api.bitbucket.org/2.0"

// AuthApplier applies authentication to an HTTP request.
type AuthApplier func(req *http.Request) error

// BearerAuth returns an AuthApplier that uses OAuth Bearer tokens.
func BearerAuth(tokenFn func() (string, error)) AuthApplier {
	return func(req *http.Request) error {
		token, err := tokenFn()
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

// BasicAuth returns an AuthApplier that uses HTTP Basic authentication (for App Passwords).
func BasicAuth(username, password string) AuthApplier {
	return func(req *http.Request) error {
		req.SetBasicAuth(username, password)
		return nil
	}
}

// Client wraps the Bitbucket Cloud REST API.
type Client struct {
	httpClient  *http.Client
	authApplier AuthApplier
}

// NewClient creates a new Bitbucket API client.
func NewClient(authApplier AuthApplier) *Client {
	return &Client{
		httpClient: &http.Client{
			// PR creation can exceed 30s while Bitbucket computes the diff
			// for large repos, so keep this generous.
			Timeout: 90 * time.Second,
		},
		authApplier: authApplier,
	}
}

// NewClientWithHTTPClient creates a Bitbucket API client with a custom http.Client.
// Intended for testing with httptest servers.
func NewClientWithHTTPClient(httpClient *http.Client, authApplier AuthApplier) *Client {
	return &Client{
		httpClient:  httpClient,
		authApplier: authApplier,
	}
}

// ListRepositories returns all repos in a workspace (handles pagination).
func (c *Client) ListRepositories(workspace string) ([]Repository, error) {
	const maxPages = 50
	var allRepos []Repository
	nextURL := fmt.Sprintf("%s/repositories/%s?pagelen=100", baseURL, url.PathEscape(workspace))

	for i := 0; nextURL != "" && i < maxPages; i++ {
		var page PaginatedResponse
		if err := c.doRequest("GET", nextURL, nil, &page); err != nil {
			return nil, fmt.Errorf("failed to list repositories: %w", err)
		}
		allRepos = append(allRepos, page.Values...)
		nextURL = page.Next
	}

	return allRepos, nil
}

// GetRepository returns a single repository.
func (c *Client) GetRepository(workspace, repoSlug string) (*Repository, error) {
	url := fmt.Sprintf("%s/repositories/%s/%s", baseURL, url.PathEscape(workspace), url.PathEscape(repoSlug))
	var repo Repository
	if err := c.doRequest("GET", url, nil, &repo); err != nil {
		return nil, fmt.Errorf("failed to get repository %s: %w", repoSlug, err)
	}
	return &repo, nil
}

// CreateBranch creates a new branch in a repository.
func (c *Client) CreateBranch(workspace, repoSlug, branchName, sourceBranch string) (*Branch, error) {
	url := fmt.Sprintf("%s/repositories/%s/%s/refs/branches", baseURL, url.PathEscape(workspace), url.PathEscape(repoSlug))
	body := CreateBranchRequest{
		Name:   branchName,
		Target: BranchTarget{Hash: sourceBranch},
	}

	var branch Branch
	if err := c.doRequest("POST", url, body, &branch); err != nil {
		return nil, err
	}
	return &branch, nil
}

// CreatePullRequest creates a pull request in a repository.
func (c *Client) CreatePullRequest(workspace, repoSlug string, pr CreatePullRequestRequest) (*PullRequest, error) {
	url := fmt.Sprintf("%s/repositories/%s/%s/pullrequests", baseURL, url.PathEscape(workspace), url.PathEscape(repoSlug))

	var result PullRequest
	if err := c.doRequest("POST", url, pr, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListCommits returns commits reachable from include but not from exclude.
func (c *Client) ListCommits(workspace, repoSlug, include, exclude string) ([]Commit, error) {
	reqURL := fmt.Sprintf("%s/repositories/%s/%s/commits?include=%s&exclude=%s",
		baseURL, url.PathEscape(workspace), url.PathEscape(repoSlug),
		url.QueryEscape(include), url.QueryEscape(exclude))

	var page PaginatedCommits
	if err := c.doRequest("GET", reqURL, nil, &page); err != nil {
		return nil, fmt.Errorf("failed to list commits: %w", err)
	}
	return page.Values, nil
}

// ListPullRequests returns PRs for a repo filtered by state (default: OPEN).
func (c *Client) ListPullRequests(workspace, repoSlug, state string) ([]PullRequest, error) {
	if state == "" {
		state = "OPEN"
	}
	nextURL := fmt.Sprintf("%s/repositories/%s/%s/pullrequests?state=%s&pagelen=50",
		baseURL, url.PathEscape(workspace), url.PathEscape(repoSlug), url.QueryEscape(state))

	var allPRs []PullRequest
	for i := 0; nextURL != "" && i < 10; i++ {
		var page PaginatedPullRequests
		if err := c.doRequest("GET", nextURL, nil, &page); err != nil {
			return nil, fmt.Errorf("failed to list pull requests: %w", err)
		}
		allPRs = append(allPRs, page.Values...)
		nextURL = page.Next
	}
	return allPRs, nil
}

// GetCurrentUser returns the authenticated user.
func (c *Client) GetCurrentUser() (*User, error) {
	reqURL := fmt.Sprintf("%s/user", baseURL)
	var user User
	if err := c.doRequest("GET", reqURL, nil, &user); err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}
	return &user, nil
}

// FindPRByBranch finds a PR by source branch name and state (default: OPEN).
func (c *Client) FindPRByBranch(workspace, repoSlug, branchName, state string) (*PullRequest, error) {
	if state == "" {
		state = "OPEN"
	}
	if strings.ContainsAny(branchName, `"`) {
		return nil, fmt.Errorf("invalid branch name: contains illegal characters")
	}
	query := fmt.Sprintf(`source.branch.name="%s"`, branchName)
	reqURL := fmt.Sprintf("%s/repositories/%s/%s/pullrequests?state=%s&q=%s",
		baseURL, url.PathEscape(workspace), url.PathEscape(repoSlug),
		url.QueryEscape(state), url.QueryEscape(query))

	var page PaginatedPullRequests
	if err := c.doRequest("GET", reqURL, nil, &page); err != nil {
		return nil, fmt.Errorf("failed to find PR for branch %q: %w", branchName, err)
	}
	if len(page.Values) == 0 {
		return nil, fmt.Errorf("no %s PR found for branch %q", state, branchName)
	}
	return &page.Values[0], nil
}

// MergePR merges a pull request.
func (c *Client) MergePR(workspace, repoSlug string, prID int, req MergePRRequest) error {
	reqURL := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/merge",
		baseURL, url.PathEscape(workspace), url.PathEscape(repoSlug), prID)
	return c.doRequest("POST", reqURL, req, nil)
}

// DeclinePR declines (closes without merging) a pull request.
func (c *Client) DeclinePR(workspace, repoSlug string, prID int) error {
	reqURL := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/decline",
		baseURL, url.PathEscape(workspace), url.PathEscape(repoSlug), prID)
	// Bitbucket Cloud rejects a POST that declares Content-Type: application/json
	// but carries a zero-length body (400 Bad Request), so send an empty JSON
	// object rather than no body at all.
	return c.doRequest("POST", reqURL, struct{}{}, nil)
}

// ApprovePR approves a pull request.
func (c *Client) ApprovePR(workspace, repoSlug string, prID int) error {
	reqURL := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/approve",
		baseURL, url.PathEscape(workspace), url.PathEscape(repoSlug), prID)
	// See DeclinePR: an empty body with Content-Type: application/json is rejected.
	return c.doRequest("POST", reqURL, struct{}{}, nil)
}

// UpdatePR updates a pull request (e.g., to add reviewers).
func (c *Client) UpdatePR(workspace, repoSlug string, prID int, req PRUpdateRequest) (*PullRequest, error) {
	reqURL := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d",
		baseURL, url.PathEscape(workspace), url.PathEscape(repoSlug), prID)
	var result PullRequest
	if err := c.doRequest("PUT", reqURL, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteBranch deletes a branch from a repository.
func (c *Client) DeleteBranch(workspace, repoSlug, branchName string) error {
	reqURL := fmt.Sprintf("%s/repositories/%s/%s/refs/branches/%s",
		baseURL, url.PathEscape(workspace), url.PathEscape(repoSlug), url.PathEscape(branchName))
	return c.doRequest("DELETE", reqURL, nil, nil)
}

// ListBranches returns all branches in a repository (handles pagination).
func (c *Client) ListBranches(workspace, repoSlug string) ([]Branch, error) {
	var allBranches []Branch
	nextURL := fmt.Sprintf("%s/repositories/%s/%s/refs/branches?pagelen=100",
		baseURL, url.PathEscape(workspace), url.PathEscape(repoSlug))

	for i := 0; nextURL != "" && i < 50; i++ {
		var page PaginatedBranches
		if err := c.doRequest("GET", nextURL, nil, &page); err != nil {
			return nil, fmt.Errorf("failed to list branches: %w", err)
		}
		allBranches = append(allBranches, page.Values...)
		nextURL = page.Next
	}
	return allBranches, nil
}

// ListMergedPRBranches returns source branch names from merged PRs.
func (c *Client) ListMergedPRBranches(workspace, repoSlug string) ([]string, error) {
	prs, err := c.ListPullRequests(workspace, repoSlug, "MERGED")
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var branches []string
	for _, pr := range prs {
		name := pr.Source.Branch.Name
		if name != "" && !seen[name] {
			branches = append(branches, name)
			seen[name] = true
		}
	}
	return branches, nil
}

// doRequest performs an authenticated HTTP request and decodes the JSON response.
func (c *Client) doRequest(method, url string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return err
	}

	if err := c.authApplier(req); err != nil {
		return fmt.Errorf("auth error: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle 204 No Content (e.g. DELETE responses)
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	// Handle error responses
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)

		var apiErr APIError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error.Message != "" {
			return formatAPIError(resp.StatusCode, apiErr)
		}
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// formatAPIError creates a user-friendly error message from a Bitbucket API error.
func formatAPIError(statusCode int, apiErr APIError) error {
	msg := apiErr.Error.Message

	// Try to parse permission scope details from the detail field
	if apiErr.Error.Detail != nil {
		var scope ScopeDetail
		if json.Unmarshal(apiErr.Error.Detail, &scope) == nil && len(scope.Required) > 0 {
			msg += "\n  Required scopes: " + strings.Join(scope.Required, ", ")
			msg += "\n  Granted scopes:  " + strings.Join(scope.Granted, ", ")
			return fmt.Errorf("API error (%d): %s", statusCode, msg)
		}

		// Detail might be a plain string
		var detail string
		if json.Unmarshal(apiErr.Error.Detail, &detail) == nil && detail != "" {
			msg += ": " + detail
		}
	}

	return fmt.Errorf("API error (%d): %s", statusCode, msg)
}
