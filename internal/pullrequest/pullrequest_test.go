package pullrequest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/chinhstringee/buck/internal/bitbucket"
)

// mockPRServer builds an httptest.Server that handles:
// - GET  /2.0/repositories/{ws}/{slug}             → Repository
// - GET  /2.0/repositories/{ws}/{slug}/commits     → PaginatedCommits (for description)
// - POST /2.0/repositories/{ws}/{slug}/pullrequests → PullRequest or error
//
// repoMainBranch maps repoSlug → main branch name (used if GET repo is requested).
// prResponses maps repoSlug → PullRequest to return (status 201).
// prErrors maps repoSlug → API error message (status 409).
func mockPRServer(t *testing.T, repoMainBranch map[string]string, prResponses map[string]bitbucket.PullRequest, prErrors map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		// parts: [2.0, repositories, {ws}, {slug}, ...]
		if len(parts) < 4 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		slug := parts[3]
		w.Header().Set("Content-Type", "application/json")

		// POST .../pullrequests
		if r.Method == http.MethodPost && len(parts) >= 5 && parts[4] == "pullrequests" {
			if errMsg, bad := prErrors[slug]; bad {
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(bitbucket.APIError{
					Error: bitbucket.APIErrorDetail{Message: errMsg},
				})
				return
			}
			if pr, ok := prResponses[slug]; ok {
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(pr)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(bitbucket.APIError{
				Error: bitbucket.APIErrorDetail{Message: "repo not found"},
			})
			return
		}

		// GET .../commits — return sample commits for description building
		if r.Method == http.MethodGet && len(parts) >= 5 && parts[4] == "commits" {
			json.NewEncoder(w).Encode(bitbucket.PaginatedCommits{
				Values: []bitbucket.Commit{
					{Hash: "abc1234567890", Message: "add new feature"},
					{Hash: "def5678901234", Message: "fix bug in handler"},
				},
			})
			return
		}

		// GET .../repositories/{ws}/{slug} — return repo with mainbranch
		if r.Method == http.MethodGet && len(parts) == 4 {
			mainBranch := repoMainBranch[slug]
			repo := bitbucket.Repository{Slug: slug}
			if mainBranch != "" {
				repo.MainBranch = &bitbucket.BranchRef{Name: mainBranch}
			}
			json.NewEncoder(w).Encode(repo)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
}

// hostRewriteTransport rewrites all requests to the test server.
type hostRewriteTransport struct {
	base    http.RoundTripper
	srvHost string
}

func (t *hostRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = "http"
	cloned.URL.Host = t.srvHost
	return t.base.RoundTrip(cloned)
}

func newPRCreatorForServer(srv *httptest.Server) *PRCreator {
	transport := &hostRewriteTransport{
		base:    http.DefaultTransport,
		srvHost: srv.Listener.Addr().String(),
	}
	httpClient := &http.Client{Transport: transport}
	authApplier := bitbucket.BearerAuth(func() (string, error) { return "test-token", nil })
	client := bitbucket.NewClientWithHTTPClient(httpClient, authApplier)
	return NewPRCreator(client)
}

// ---------- CreatePRs ----------

func TestCreatePRs_AllSuccess(t *testing.T) {
	repos := []string{"repo-a", "repo-b", "repo-c"}
	mainBranches := map[string]string{
		"repo-a": "main",
		"repo-b": "master",
		"repo-c": "develop",
	}
	prResponses := map[string]bitbucket.PullRequest{
		"repo-a": {ID: 1, Title: "feature/x", State: "OPEN", Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/ws/repo-a/pr/1"}}},
		"repo-b": {ID: 2, Title: "feature/x", State: "OPEN", Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/ws/repo-b/pr/2"}}},
		"repo-c": {ID: 3, Title: "feature/x", State: "OPEN", Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/ws/repo-c/pr/3"}}},
	}

	srv := mockPRServer(t, mainBranches, prResponses, nil)
	defer srv.Close()

	pc := newPRCreatorForServer(srv)
	results := pc.CreatePRs("ws", repos, "feature/x", "", "", nil)

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("repo %q failed: %s", r.RepoSlug, r.Error)
		}
		if r.PRURL == "" {
			t.Errorf("repo %q has empty PRURL", r.RepoSlug)
		}
		if r.PRID == 0 {
			t.Errorf("repo %q has PRID=0", r.RepoSlug)
		}
	}
}

func TestCreatePRs_PartialFailure(t *testing.T) {
	repos := []string{"repo-ok", "repo-fail", "repo-ok2"}
	mainBranches := map[string]string{
		"repo-ok":   "main",
		"repo-fail": "main",
		"repo-ok2":  "main",
	}
	prResponses := map[string]bitbucket.PullRequest{
		"repo-ok":  {ID: 1, Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/pr/1"}}},
		"repo-ok2": {ID: 2, Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/pr/2"}}},
	}
	prErrors := map[string]string{
		"repo-fail": "There is already an open pull request",
	}

	srv := mockPRServer(t, mainBranches, prResponses, prErrors)
	defer srv.Close()

	pc := newPRCreatorForServer(srv)
	results := pc.CreatePRs("ws", repos, "feature/x", "", "", nil)

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}

	var succeeded, failed int
	for _, r := range results {
		if r.Success {
			succeeded++
		} else {
			failed++
			if r.RepoSlug != "repo-fail" {
				t.Errorf("unexpected failure: %q", r.RepoSlug)
			}
			if r.Error == "" {
				t.Errorf("failed result %q has empty Error", r.RepoSlug)
			}
		}
	}
	if succeeded != 2 {
		t.Errorf("succeeded = %d, want 2", succeeded)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}
}

func TestCreatePRs_AllFailure(t *testing.T) {
	repos := []string{"repo-a", "repo-b"}
	mainBranches := map[string]string{"repo-a": "main", "repo-b": "main"}
	prErrors := map[string]string{
		"repo-a": "not found",
		"repo-b": "unauthorized",
	}

	srv := mockPRServer(t, mainBranches, nil, prErrors)
	defer srv.Close()

	pc := newPRCreatorForServer(srv)
	results := pc.CreatePRs("ws", repos, "feature/x", "", "", nil)

	for _, r := range results {
		if r.Success {
			t.Errorf("repo %q should have failed", r.RepoSlug)
		}
		if r.Error == "" {
			t.Errorf("repo %q has empty Error", r.RepoSlug)
		}
	}
}

func TestCreatePRs_EmptyRepoList(t *testing.T) {
	srv := mockPRServer(t, nil, nil, nil)
	defer srv.Close()

	pc := newPRCreatorForServer(srv)
	results := pc.CreatePRs("ws", []string{}, "feature/x", "", "", nil)

	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

func TestCreatePRs_SortedBySlug(t *testing.T) {
	repos := []string{"zeta", "alpha", "gamma", "beta"}
	mainBranches := map[string]string{}
	prResponses := map[string]bitbucket.PullRequest{}
	for _, slug := range repos {
		mainBranches[slug] = "main"
		prResponses[slug] = bitbucket.PullRequest{ID: 1, Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/pr/1"}}}
	}

	srv := mockPRServer(t, mainBranches, prResponses, nil)
	defer srv.Close()

	pc := newPRCreatorForServer(srv)
	results := pc.CreatePRs("ws", repos, "feature/x", "", "", nil)

	expected := []string{"alpha", "beta", "gamma", "zeta"}
	for i, want := range expected {
		if results[i].RepoSlug != want {
			t.Errorf("results[%d].RepoSlug = %q, want %q", i, results[i].RepoSlug, want)
		}
	}
}

func TestCreatePRs_Concurrency(t *testing.T) {
	var requestCount atomic.Int64
	repos := make([]string, 20)
	for i := range repos {
		repos[i] = fmt.Sprintf("repo-%02d", i)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(bitbucket.PullRequest{
				ID:    1,
				Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/pr/1"}},
			})
			return
		}

		// GET commits
		if len(parts) >= 5 && parts[4] == "commits" {
			json.NewEncoder(w).Encode(bitbucket.PaginatedCommits{
				Values: []bitbucket.Commit{{Hash: "abc123", Message: "test commit"}},
			})
			return
		}

		// GET repo
		json.NewEncoder(w).Encode(bitbucket.Repository{
			Slug:       "test",
			MainBranch: &bitbucket.BranchRef{Name: "main"},
		})
	}))
	defer srv.Close()

	pc := newPRCreatorForServer(srv)
	results := pc.CreatePRs("ws", repos, "feature/x", "", "", nil)

	if len(results) != 20 {
		t.Errorf("len(results) = %d, want 20", len(results))
	}
	// Each repo makes 2 requests: GET commits + POST PR = 40 total
	if int(requestCount.Load()) != 40 {
		t.Errorf("HTTP request count = %d, want 40", requestCount.Load())
	}
}

func TestCreatePRs_DestinationOverride(t *testing.T) {
	var getRepoCalled atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

		if r.Method == http.MethodGet {
			// GET commits — allowed even with destination override
			if len(parts) >= 5 && parts[4] == "commits" {
				json.NewEncoder(w).Encode(bitbucket.PaginatedCommits{})
				return
			}
			// GET repo — should NOT be called with destination override
			getRepoCalled.Add(1)
			json.NewEncoder(w).Encode(bitbucket.Repository{Slug: "test"})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(bitbucket.PullRequest{
			ID:    1,
			Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/pr/1"}},
		})
	}))
	defer srv.Close()

	pc := newPRCreatorForServer(srv)
	results := pc.CreatePRs("ws", []string{"repo-a", "repo-b"}, "feature/x", "develop", "", nil)

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("repo %q failed: %s", r.RepoSlug, r.Error)
		}
	}
	// When destination is provided, GetRepository should NOT be called
	if getRepoCalled.Load() != 0 {
		t.Errorf("GetRepository called %d times, want 0 (destination override)", getRepoCalled.Load())
	}
}

func TestCreatePRs_DefaultDestinationMaster(t *testing.T) {
	var getRepoCalled atomic.Int64
	var gotBody bitbucket.CreatePullRequestRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

		if r.Method == http.MethodGet {
			if len(parts) >= 5 && parts[4] == "commits" {
				json.NewEncoder(w).Encode(bitbucket.PaginatedCommits{})
				return
			}
			getRepoCalled.Add(1)
			json.NewEncoder(w).Encode(bitbucket.Repository{
				Slug:       "test",
				MainBranch: &bitbucket.BranchRef{Name: "develop"},
			})
			return
		}

		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(bitbucket.PullRequest{
			ID:    1,
			Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/pr/1"}},
		})
	}))
	defer srv.Close()

	pc := newPRCreatorForServer(srv)
	results := pc.CreatePRs("ws", []string{"repo-a"}, "feature/x", "", "", nil)

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if !results[0].Success {
		t.Errorf("expected success, got error: %s", results[0].Error)
	}
	if gotBody.Destination.Branch.Name != "master" {
		t.Errorf("destination = %q, want %q (default destination)", gotBody.Destination.Branch.Name, "master")
	}
	// When no destination, repository details should not be requested
	if getRepoCalled.Load() != 0 {
		t.Errorf("GetRepository called %d times, want 0", getRepoCalled.Load())
	}
}

func TestCreatePRs_EmptyDestinationWhitespaceUsesMaster(t *testing.T) {
	var gotBody bitbucket.CreatePullRequestRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

		if r.Method == http.MethodGet {
			if len(parts) >= 5 && parts[4] == "commits" {
				json.NewEncoder(w).Encode(bitbucket.PaginatedCommits{})
				return
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}

		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(bitbucket.PullRequest{
			ID:    1,
			Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/pr/1"}},
		})
	}))
	defer srv.Close()

	pc := newPRCreatorForServer(srv)
	results := pc.CreatePRs("ws", []string{"test-repo"}, "feature/x", "   ", "", nil)

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if !results[0].Success {
		t.Errorf("expected success, got error: %s", results[0].Error)
	}
	if gotBody.Destination.Branch.Name != "master" {
		t.Errorf("destination = %q, want %q (whitespace destination fallback)", gotBody.Destination.Branch.Name, "master")
	}
}

func TestCreatePRs_CustomTitle(t *testing.T) {
	var gotBody bitbucket.CreatePullRequestRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

		if r.Method == http.MethodGet {
			if len(parts) >= 5 && parts[4] == "commits" {
				json.NewEncoder(w).Encode(bitbucket.PaginatedCommits{})
				return
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}

		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(bitbucket.PullRequest{
			ID:    1,
			Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/pr/1"}},
		})
	}))
	defer srv.Close()

	pc := newPRCreatorForServer(srv)

	// Explicit title must be sent as-is, overriding the branch-derived one.
	results := pc.CreatePRs("ws", []string{"test-repo"}, "feature/SPT-1-x", "release", "My Custom Title", nil)
	if !results[0].Success {
		t.Fatalf("expected success, got error: %s", results[0].Error)
	}
	if gotBody.Title != "My Custom Title" {
		t.Errorf("title = %q, want %q", gotBody.Title, "My Custom Title")
	}

	// Whitespace-only title falls back to the branch-derived title.
	results = pc.CreatePRs("ws", []string{"test-repo"}, "feature/SPT-1-x", "release", "   ", nil)
	if !results[0].Success {
		t.Fatalf("expected success, got error: %s", results[0].Error)
	}
	if want := "Feature/SPT-1 x"; gotBody.Title != want {
		t.Errorf("title = %q, want %q (branch-derived fallback)", gotBody.Title, want)
	}
}

// ---------- Description override (--body/--body-file) ----------

func TestCreatePRs_DescriptionOverride(t *testing.T) {
	var gotBody bitbucket.CreatePullRequestRequest
	var commitsRequested atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

		if r.Method == http.MethodGet && len(parts) >= 5 && parts[4] == "commits" {
			commitsRequested.Store(true)
			json.NewEncoder(w).Encode(bitbucket.PaginatedCommits{
				Values: []bitbucket.Commit{{Hash: "abc123", Message: "should not be used"}},
			})
			return
		}

		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(bitbucket.PullRequest{
			ID:    1,
			Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/pr/1"}},
		})
	}))
	defer srv.Close()

	pc := newPRCreatorForServer(srv)
	override := "Reviewed description from --body-file"
	results := pc.CreatePRs("ws", []string{"test-repo"}, "feature/x", "release", "", &override)

	if !results[0].Success {
		t.Fatalf("expected success, got error: %s", results[0].Error)
	}
	if gotBody.Description != override {
		t.Errorf("description = %q, want %q", gotBody.Description, override)
	}
	if commitsRequested.Load() {
		t.Error("ListCommits should not be called when a description override is set")
	}
}

func TestCreatePRs_EmptyDescriptionOverrideIsUsedAsIs(t *testing.T) {
	var gotBody bitbucket.CreatePullRequestRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(bitbucket.PullRequest{
			ID:    1,
			Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/pr/1"}},
		})
	}))
	defer srv.Close()

	pc := newPRCreatorForServer(srv)
	empty := ""
	results := pc.CreatePRs("ws", []string{"test-repo"}, "feature/x", "release", "", &empty)

	if !results[0].Success {
		t.Fatalf("expected success, got error: %s", results[0].Error)
	}
	if gotBody.Description != "" {
		t.Errorf("description = %q, want empty string (explicit override)", gotBody.Description)
	}
}

func TestCreatePRs_NoOverrideUsesCommitDerivedDescription(t *testing.T) {
	var gotBody bitbucket.CreatePullRequestRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

		if r.Method == http.MethodGet && len(parts) >= 5 && parts[4] == "commits" {
			json.NewEncoder(w).Encode(bitbucket.PaginatedCommits{
				Values: []bitbucket.Commit{{Hash: "abc123", Message: "add new feature"}},
			})
			return
		}

		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(bitbucket.PullRequest{
			ID:    1,
			Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/pr/1"}},
		})
	}))
	defer srv.Close()

	pc := newPRCreatorForServer(srv)
	results := pc.CreatePRs("ws", []string{"test-repo"}, "feature/x", "release", "", nil)

	if !results[0].Success {
		t.Fatalf("expected success, got error: %s", results[0].Error)
	}
	if want := "* add new feature"; gotBody.Description != want {
		t.Errorf("description = %q, want %q (commit-derived default)", gotBody.Description, want)
	}
}

// ---------- formatBranchTitle ----------

func TestFormatBranchTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"feature/SPT-1298-increase-api-limit", "Feature/SPT-1298 increase api limit"},
		{"bugfix/PROJ-42-fix-login", "Bugfix/PROJ-42 fix login"},
		{"feature/add-dark-mode", "Feature/add dark mode"},
		{"fix-something", "Fix something"},
		{"feature/ABC-1-DEF-2-multi-ticket", "Feature/ABC-1 DEF-2 multi ticket"},
		{"main", "Main"},
		{"", ""},
	}

	for _, tc := range tests {
		got := formatBranchTitle(tc.input)
		if got != tc.want {
			t.Errorf("formatBranchTitle(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------- buildDescription ----------

func TestBuildDescription(t *testing.T) {
	commits := []bitbucket.Commit{
		{Hash: "abc1234567890", Message: "add new feature\n\ndetailed body"},
		{Hash: "def5678901234", Message: "fix bug in handler"},
	}

	got := buildDescription(commits)
	want := "* add new feature\n* fix bug in handler"
	if got != want {
		t.Errorf("buildDescription() = %q, want %q", got, want)
	}
}

func TestBuildDescription_Empty(t *testing.T) {
	got := buildDescription(nil)
	if got != "" {
		t.Errorf("buildDescription(nil) = %q, want empty string", got)
	}
}

// ---------- NewPRCreator ----------

func TestNewPRCreator_NotNil(t *testing.T) {
	pc := NewPRCreator(nil)
	if pc == nil {
		t.Fatal("NewPRCreator returned nil")
	}
}
