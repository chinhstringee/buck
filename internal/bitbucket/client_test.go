package bitbucket

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// hostRewriteTransport redirects requests built against the real Bitbucket
// baseURL to a local httptest.Server, so exported methods that hardcode
// baseURL (e.g. DeclinePR, ApprovePR) can be exercised end-to-end.
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

func newClientForServer(srv *httptest.Server) *Client {
	transport := &hostRewriteTransport{
		base:    http.DefaultTransport,
		srvHost: srv.Listener.Addr().String(),
	}
	return NewClientWithHTTPClient(&http.Client{Transport: transport}, mockAuthApplier("tok"))
}

// newTestClient returns a Client pointed at the given httptest.Server URL.
// It replaces the package-level baseURL by overriding the URL in each method
// via a server whose handler mirrors the real API shape.
func mockAuthApplier(token string) AuthApplier {
	return BearerAuth(func() (string, error) { return token, nil })
}

func errorAuthApplier() AuthApplier {
	return BearerAuth(func() (string, error) { return "", fmt.Errorf("auth failure") })
}

// ---------- NewClient ----------

func TestNewClient_NotNil(t *testing.T) {
	c := NewClient(mockAuthApplier("tok"))
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.httpClient == nil {
		t.Fatal("httpClient is nil")
	}
}

// ---------- doRequest / auth ----------

func TestDoRequest_AuthHeaderSet(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Repository{Slug: "test-repo"})
	}))
	defer srv.Close()

	c := NewClient(mockAuthApplier("my-access-token"))
	var repo Repository
	err := c.doRequest("GET", srv.URL, nil, &repo)
	if err != nil {
		t.Fatalf("doRequest error: %v", err)
	}
	if gotAuth != "Bearer my-access-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer my-access-token")
	}
}

func TestDoRequest_BasicAuth(t *testing.T) {
	var gotUser, gotPass string
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, _ = r.BasicAuth()
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Repository{Slug: "test-repo"})
	}))
	defer srv.Close()

	c := NewClient(BasicAuth("myuser", "myapppass"))
	var repo Repository
	err := c.doRequest("GET", srv.URL, nil, &repo)
	if err != nil {
		t.Fatalf("doRequest error: %v", err)
	}
	if gotUser != "myuser" {
		t.Errorf("Basic auth user = %q, want %q", gotUser, "myuser")
	}
	if gotPass != "myapppass" {
		t.Errorf("Basic auth pass = %q, want %q", gotPass, "myapppass")
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Errorf("Authorization header = %q, want Basic prefix", gotAuth)
	}
}

func TestDoRequest_TokenProviderError(t *testing.T) {
	c := NewClient(errorAuthApplier())
	var result Repository
	err := c.doRequest("GET", "http://localhost/ignored", nil, &result)
	if err == nil {
		t.Fatal("expected error from failed token provider, got nil")
	}
	if !strings.Contains(err.Error(), "auth error") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "auth error")
	}
}

func TestDoRequest_APIError_WithMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIError{
			Error: APIErrorDetail{Message: "Repository not found"},
		})
	}))
	defer srv.Close()

	c := NewClient(mockAuthApplier("tok"))
	var result Repository
	err := c.doRequest("GET", srv.URL, nil, &result)
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
	if !strings.Contains(err.Error(), "Repository not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "Repository not found")
	}
}

func TestDoRequest_APIError_PlainBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	}))
	defer srv.Close()

	c := NewClient(mockAuthApplier("tok"))
	var result Repository
	err := c.doRequest("GET", srv.URL, nil, &result)
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "API error (401)") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "API error (401)")
	}
}

func TestDoRequest_InvalidJSON_Response(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not-json{{{"))
	}))
	defer srv.Close()

	c := NewClient(mockAuthApplier("tok"))
	var result Repository
	err := c.doRequest("GET", srv.URL, nil, &result)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// ---------- ListRepositories ----------

func TestListRepositories_SinglePage(t *testing.T) {
	repos := []Repository{
		{Slug: "alpha", Name: "Alpha"},
		{Slug: "beta", Name: "Beta"},
	}
	page := PaginatedResponse{Values: repos, Next: ""}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(page)
	}))
	defer srv.Close()

	c := &Client{
		httpClient:    srv.Client(),
		authApplier: mockAuthApplier("tok"),
	}

	// Override the request URL by calling doRequest directly with the test server URL
	var got PaginatedResponse
	err := c.doRequest("GET", srv.URL+"?pagelen=100", nil, &got)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Values) != 2 {
		t.Errorf("got %d repos, want 2", len(got.Values))
	}
}

func TestListRepositories_Pagination(t *testing.T) {
	callCount := 0
	page1Repos := []Repository{{Slug: "repo-1"}}
	page2Repos := []Repository{{Slug: "repo-2"}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		if callCount == 1 {
			// First page — Next points back to same server
			page := PaginatedResponse{Values: page1Repos, Next: "http://" + r.Host + r.URL.Path + "?page=2"}
			json.NewEncoder(w).Encode(page)
		} else {
			// Second page — no next
			page := PaginatedResponse{Values: page2Repos, Next: ""}
			json.NewEncoder(w).Encode(page)
		}
	}))
	defer srv.Close()

	// We use a real Client with the test server by making the first request
	// go to srv.URL directly — testing doRequest + pagination loop independently.
	c := &Client{
		httpClient:    srv.Client(),
		authApplier: mockAuthApplier("tok"),
	}

	// Manually replicate the ListRepositories pagination loop against the test server
	var allRepos []Repository
	nextURL := srv.URL + "/repositories/ws?pagelen=100"
	for i := 0; nextURL != "" && i < 50; i++ {
		var p PaginatedResponse
		if err := c.doRequest("GET", nextURL, nil, &p); err != nil {
			t.Fatalf("page %d error: %v", i, err)
		}
		allRepos = append(allRepos, p.Values...)
		nextURL = p.Next
	}

	if len(allRepos) != 2 {
		t.Errorf("got %d repos across pages, want 2", len(allRepos))
	}
	if allRepos[0].Slug != "repo-1" || allRepos[1].Slug != "repo-2" {
		t.Errorf("unexpected repo order: %v", allRepos)
	}
}

// ---------- GetRepository ----------

func TestGetRepository_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Repository{
			Slug:     "my-repo",
			Name:     "My Repo",
			FullName: "workspace/my-repo",
		})
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), authApplier: mockAuthApplier("tok")}
	var repo Repository
	err := c.doRequest("GET", srv.URL, nil, &repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.Slug != "my-repo" {
		t.Errorf("Slug = %q, want %q", repo.Slug, "my-repo")
	}
}

func TestGetRepository_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIError{Error: APIErrorDetail{Message: "No such repository"}})
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), authApplier: mockAuthApplier("tok")}
	var repo Repository
	err := c.doRequest("GET", srv.URL, nil, &repo)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "No such repository") {
		t.Errorf("error %q does not contain expected message", err.Error())
	}
}

// ---------- CreateBranch ----------

func TestCreateBranch_Success(t *testing.T) {
	var gotBody CreateBranchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Branch{
			Name:   "feature/my-branch",
			Target: BranchTarget{Hash: "abc1234def5678"},
		})
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), authApplier: mockAuthApplier("tok")}

	var branch Branch
	body := CreateBranchRequest{
		Name:   "feature/my-branch",
		Target: BranchTarget{Hash: "main"},
	}
	err := c.doRequest("POST", srv.URL, body, &branch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch.Name != "feature/my-branch" {
		t.Errorf("branch.Name = %q, want %q", branch.Name, "feature/my-branch")
	}
	if branch.Target.Hash != "abc1234def5678" {
		t.Errorf("branch.Target.Hash = %q, want %q", branch.Target.Hash, "abc1234def5678")
	}
}

func TestCreateBranch_Conflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(APIError{
			Error: APIErrorDetail{Message: "Branch already exists"},
		})
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), authApplier: mockAuthApplier("tok")}
	var branch Branch
	err := c.doRequest("POST", srv.URL, CreateBranchRequest{Name: "existing"}, &branch)
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "Branch already exists") {
		t.Errorf("error %q does not mention branch conflict", err.Error())
	}
}

// ---------- CreatePullRequest ----------

func TestCreatePullRequest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(PullRequest{
			ID:    42,
			Title: "feature/add-auth",
			State: "OPEN",
			Links: PRLinks{HTML: LinkRef{Href: "https://bitbucket.org/ws/repo/pull-requests/42"}},
		})
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), authApplier: mockAuthApplier("tok")}
	var pr PullRequest
	body := CreatePullRequestRequest{
		Title:       "feature/add-auth",
		Description: "Automated PR",
		Source:      PRBranchRef{Branch: PRBranchName{Name: "feature/add-auth"}},
		Destination: PRBranchRef{Branch: PRBranchName{Name: "main"}},
	}
	err := c.doRequest("POST", srv.URL, body, &pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.ID != 42 {
		t.Errorf("pr.ID = %d, want 42", pr.ID)
	}
	if pr.Title != "feature/add-auth" {
		t.Errorf("pr.Title = %q, want %q", pr.Title, "feature/add-auth")
	}
	if pr.Links.HTML.Href != "https://bitbucket.org/ws/repo/pull-requests/42" {
		t.Errorf("pr.Links.HTML.Href = %q, want expected URL", pr.Links.HTML.Href)
	}
}

func TestCreatePullRequest_Conflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(APIError{
			Error: APIErrorDetail{Message: "There is already an open pull request"},
		})
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), authApplier: mockAuthApplier("tok")}
	var pr PullRequest
	err := c.doRequest("POST", srv.URL, CreatePullRequestRequest{Title: "dup"}, &pr)
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "There is already an open pull request") {
		t.Errorf("error %q does not mention PR conflict", err.Error())
	}
}

func TestCreatePullRequest_RequestBody(t *testing.T) {
	var gotBody CreatePullRequestRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(PullRequest{ID: 1})
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), authApplier: mockAuthApplier("tok")}
	body := CreatePullRequestRequest{
		Title:             "feature/x",
		Description:       "desc",
		Source:            PRBranchRef{Branch: PRBranchName{Name: "feature/x"}},
		Destination:       PRBranchRef{Branch: PRBranchName{Name: "develop"}},
		CloseSourceBranch: true,
	}
	var pr PullRequest
	err := c.doRequest("POST", srv.URL, body, &pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody.Title != "feature/x" {
		t.Errorf("body.Title = %q, want %q", gotBody.Title, "feature/x")
	}
	if gotBody.Source.Branch.Name != "feature/x" {
		t.Errorf("body.Source.Branch.Name = %q, want %q", gotBody.Source.Branch.Name, "feature/x")
	}
	if gotBody.Destination.Branch.Name != "develop" {
		t.Errorf("body.Destination.Branch.Name = %q, want %q", gotBody.Destination.Branch.Name, "develop")
	}
	if !gotBody.CloseSourceBranch {
		t.Error("body.CloseSourceBranch = false, want true")
	}
}

// ---------- Content-Type / Accept headers ----------

func TestDoRequest_Headers(t *testing.T) {
	var gotContentType, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct{}{})
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), authApplier: mockAuthApplier("tok")}
	err := c.doRequest("POST", srv.URL, map[string]string{"k": "v"}, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q, want application/json", gotAccept)
	}
}

// ---------- DeclinePR / ApprovePR body ----------

// bitbucketLikeActionServer mimics Bitbucket Cloud's real behavior for the
// decline/approve/merge action endpoints: a POST that declares
// "Content-Type: application/json" but carries a zero-length body is
// rejected with 400 Bad Request. A body of "{}" (or any valid JSON object)
// is accepted. This was confirmed against the real API on 2026-09-24: a
// direct curl with `-d '{}'` succeeded (200) for the same PR that buck's
// client failed to decline (400).
func bitbucketLikeActionServer(t *testing.T, gotBody *[]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*gotBody = body
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Content-Type") == "application/json" && len(body) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIError{Error: APIErrorDetail{Message: "Bad request"}})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(PullRequest{ID: 1})
	}))
}

func TestDeclinePR_SendsNonEmptyJSONBody(t *testing.T) {
	var gotBody []byte
	srv := bitbucketLikeActionServer(t, &gotBody)
	defer srv.Close()

	c := newClientForServer(srv)
	if err := c.DeclinePR("ws", "repo", 42); err != nil {
		t.Fatalf("DeclinePR returned error: %v (request body was %q)", err, gotBody)
	}
}

func TestApprovePR_SendsNonEmptyJSONBody(t *testing.T) {
	var gotBody []byte
	srv := bitbucketLikeActionServer(t, &gotBody)
	defer srv.Close()

	c := newClientForServer(srv)
	if err := c.ApprovePR("ws", "repo", 42); err != nil {
		t.Fatalf("ApprovePR returned error: %v (request body was %q)", err, gotBody)
	}
}

// ---------- GetPullRequest ----------

func TestGetPullRequest_Success(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PullRequest{
			ID:          42,
			Title:       "feature/x",
			State:       "MERGED",
			Description: "desc",
			Source: PRBranchRef{
				Branch: PRBranchName{Name: "feature/x"},
				Commit: &PRCommitRef{Hash: "abcdef1234567890"},
			},
			Destination: PRBranchRef{Branch: PRBranchName{Name: "master"}},
		})
	}))
	defer srv.Close()

	c := newClientForServer(srv)
	pr, err := c.GetPullRequest("ws", "repo", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if want := "/2.0/repositories/ws/repo/pullrequests/42"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if pr.ID != 42 || pr.State != "MERGED" {
		t.Errorf("pr = %+v, unexpected fields", pr)
	}
	if pr.Source.Commit == nil || pr.Source.Commit.Hash != "abcdef1234567890" {
		t.Errorf("pr.Source.Commit = %+v, want hash abcdef1234567890", pr.Source.Commit)
	}
}

func TestGetPullRequest_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIError{Error: APIErrorDetail{Message: "Pull request not found"}})
	}))
	defer srv.Close()

	c := newClientForServer(srv)
	_, err := c.GetPullRequest("ws", "repo", 999)
	if err == nil {
		t.Fatal("expected error for missing PR")
	}
	if !strings.Contains(err.Error(), "Pull request not found") {
		t.Errorf("error %q does not mention API message", err.Error())
	}
}

// ---------- FindPRsByBranch / FindPRByBranch ----------

func TestFindPRsByBranch_MultipleMatches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("state"); got != "DECLINED" {
			t.Errorf("state query = %q, want DECLINED", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedPullRequests{
			Values: []PullRequest{
				{ID: 10, State: "DECLINED"},
				{ID: 11, State: "DECLINED"},
			},
		})
	}))
	defer srv.Close()

	c := newClientForServer(srv)
	prs, err := c.FindPRsByBranch("ws", "repo", "feature/shared", "DECLINED")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("len(prs) = %d, want 2", len(prs))
	}
}

func TestFindPRsByBranch_FiltersByStateWhenAPIIgnoresIt(t *testing.T) {
	// Regression: Bitbucket returns PRs in every state for this branch
	// regardless of the requested state query param (verified live against
	// stringee-react-library #3865 MERGED / #3907 DECLINED on the same
	// branch). The client must filter the response itself.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedPullRequests{
			Values: []PullRequest{
				{ID: 3907, State: "DECLINED"},
				{ID: 3865, State: "MERGED"},
			},
		})
	}))
	defer srv.Close()

	c := newClientForServer(srv)

	declined, err := c.FindPRsByBranch("ws", "repo", "feature/shared", "DECLINED")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(declined) != 1 || declined[0].ID != 3907 {
		t.Errorf("DECLINED matches = %+v, want only PR 3907", declined)
	}

	merged, err := c.FindPRsByBranch("ws", "repo", "feature/shared", "MERGED")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(merged) != 1 || merged[0].ID != 3865 {
		t.Errorf("MERGED matches = %+v, want only PR 3865", merged)
	}

	open, err := c.FindPRsByBranch("ws", "repo", "feature/shared", "OPEN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(open) != 0 {
		t.Errorf("OPEN matches = %+v, want none", open)
	}
}

func TestFindPRsByBranch_InvalidBranchName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not make an API call for an invalid branch name")
	}))
	defer srv.Close()

	c := newClientForServer(srv)
	_, err := c.FindPRsByBranch("ws", "repo", `bad"branch`, "OPEN")
	if err == nil {
		t.Fatal("expected error for branch name with illegal characters")
	}
}

func TestFindPRByBranch_UsesFirstMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedPullRequests{
			Values: []PullRequest{{ID: 1, State: "OPEN"}, {ID: 2, State: "OPEN"}},
		})
	}))
	defer srv.Close()

	c := newClientForServer(srv)
	pr, err := c.FindPRByBranch("ws", "repo", "feature/x", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.ID != 1 {
		t.Errorf("pr.ID = %d, want 1 (first match)", pr.ID)
	}
}

func TestFindPRByBranch_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedPullRequests{})
	}))
	defer srv.Close()

	c := newClientForServer(srv)
	_, err := c.FindPRByBranch("ws", "repo", "feature/gone", "OPEN")
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if !strings.Contains(err.Error(), `no OPEN PR found for branch "feature/gone"`) {
		t.Errorf("error = %q, missing expected message", err.Error())
	}
}
