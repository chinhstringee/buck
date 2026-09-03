package bitbucket

import "encoding/json"

// Repository represents a Bitbucket repository.
type Repository struct {
	Slug       string     `json:"slug"`
	Name       string     `json:"name"`
	FullName   string     `json:"full_name"`
	MainBranch *BranchRef `json:"mainbranch"`
	UpdatedOn  string     `json:"updated_on"`
}

// BranchRef is a short branch reference (used in Repository.MainBranch).
type BranchRef struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Branch represents a full branch object from the API.
type Branch struct {
	Name   string       `json:"name"`
	Target BranchTarget `json:"target"`
}

// BranchTarget holds the commit hash a branch points to.
type BranchTarget struct {
	Hash string `json:"hash"`
}

// CreateBranchRequest is the POST body for creating a branch.
type CreateBranchRequest struct {
	Name   string       `json:"name"`
	Target BranchTarget `json:"target"`
}

// PaginatedResponse wraps Bitbucket's paginated API responses.
type PaginatedResponse struct {
	Values []Repository `json:"values"`
	Next   string       `json:"next"`
	Page   int          `json:"page"`
	Size   int          `json:"size"`
}

// CreatePullRequestRequest is the POST body for creating a pull request.
type CreatePullRequestRequest struct {
	Title             string      `json:"title"`
	Description       string      `json:"description"`
	Source            PRBranchRef `json:"source"`
	Destination       PRBranchRef `json:"destination"`
	CloseSourceBranch bool        `json:"close_source_branch"`
}

// PRBranchRef wraps a branch name reference for PR source/destination.
type PRBranchRef struct {
	Branch PRBranchName `json:"branch"`
}

// PRBranchName holds a branch name.
type PRBranchName struct {
	Name string `json:"name"`
}

// PullRequest represents a Bitbucket pull request response.
type PullRequest struct {
	ID           int             `json:"id"`
	Title        string          `json:"title"`
	State        string          `json:"state"`
	Description  string          `json:"description"`
	Author       PRAuthor        `json:"author"`
	Source       PRBranchRef     `json:"source"`
	Destination  PRBranchRef     `json:"destination"`
	Participants []PRParticipant `json:"participants"`
	Reviewers    []PRReviewer    `json:"reviewers"`
	Links        PRLinks         `json:"links"`
	CreatedOn    string          `json:"created_on"`
	UpdatedOn    string          `json:"updated_on"`
}

// PRAuthor represents a pull request author.
type PRAuthor struct {
	DisplayName string `json:"display_name"`
	UUID        string `json:"uuid"`
	Nickname    string `json:"nickname"`
	AccountID   string `json:"account_id"`
}

// PRParticipant represents a user's participation in a pull request.
type PRParticipant struct {
	User     PRAuthor `json:"user"`
	Role     string   `json:"role"` // REVIEWER, PARTICIPANT
	Approved bool     `json:"approved"`
	State    string   `json:"state"` // approved, changes_requested
}

// PRReviewer identifies a reviewer by UUID or account ID.
type PRReviewer struct {
	UUID      string `json:"uuid,omitempty"`
	AccountID string `json:"account_id,omitempty"`
}

// User represents the authenticated Bitbucket user.
type User struct {
	DisplayName string `json:"display_name"`
	UUID        string `json:"uuid"`
	Nickname    string `json:"nickname"`
	AccountID   string `json:"account_id"`
	Username    string `json:"username"`
}

// PaginatedPullRequests wraps paginated PR list responses.
type PaginatedPullRequests struct {
	Values []PullRequest `json:"values"`
	Next   string        `json:"next"`
	Page   int           `json:"page"`
	Size   int           `json:"size"`
}

// CommitStatus represents a build/CI status on a commit.
type CommitStatus struct {
	State       string `json:"state"` // SUCCESSFUL, FAILED, INPROGRESS, STOPPED
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

// PaginatedCommitStatuses wraps paginated commit status responses.
type PaginatedCommitStatuses struct {
	Values []CommitStatus `json:"values"`
	Next   string         `json:"next"`
}

// MergePRRequest is the POST body for merging a pull request.
type MergePRRequest struct {
	MergeStrategy     string `json:"merge_strategy,omitempty"`
	CloseSourceBranch bool   `json:"close_source_branch"`
	Message           string `json:"message,omitempty"`
}

// PRUpdateRequest is the PUT body for updating a pull request.
type PRUpdateRequest struct {
	Title       *string      `json:"title,omitempty"`
	Description *string      `json:"description,omitempty"`
	Reviewers   []PRReviewer `json:"reviewers,omitempty"`
}

// PaginatedBranches wraps paginated branch list responses.
type PaginatedBranches struct {
	Values []Branch `json:"values"`
	Next   string   `json:"next"`
}

// PRLinks holds pull request link references.
type PRLinks struct {
	HTML LinkRef `json:"html"`
}

// LinkRef holds an href URL.
type LinkRef struct {
	Href string `json:"href"`
}

// Commit represents a Bitbucket commit.
type Commit struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
}

// PaginatedCommits wraps Bitbucket's paginated commit responses.
type PaginatedCommits struct {
	Values []Commit `json:"values"`
	Next   string   `json:"next"`
}

// APIError represents an error response from Bitbucket.
type APIError struct {
	Error   APIErrorDetail `json:"error"`
	Type    string         `json:"type"`
	Status  int            `json:"status"`
}

// APIErrorDetail holds the error message and detail.
// Detail is json.RawMessage because Bitbucket returns either a string or an object.
type APIErrorDetail struct {
	Message string          `json:"message"`
	Detail  json.RawMessage `json:"detail"`
}

// ScopeDetail holds required/granted permission scopes from 403 errors.
type ScopeDetail struct {
	Required []string `json:"required"`
	Granted  []string `json:"granted"`
}
