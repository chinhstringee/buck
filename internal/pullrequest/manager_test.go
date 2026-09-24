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

// mockManagerServer handles FindPRByBranch + action endpoints.
// prByRepo maps repoSlug → PullRequest (for branch lookup).
// actionErrors maps repoSlug → error message (for the action call).
func mockManagerServer(t *testing.T, prByRepo map[string]bitbucket.PullRequest, actionErrors map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		w.Header().Set("Content-Type", "application/json")

		if len(parts) < 4 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		slug := parts[3]

		// GET .../pullrequests?q=... (FindPRByBranch)
		if r.Method == http.MethodGet && len(parts) >= 5 && parts[4] == "pullrequests" {
			pr, ok := prByRepo[slug]
			if !ok {
				json.NewEncoder(w).Encode(bitbucket.PaginatedPullRequests{})
				return
			}
			if pr.State == "" {
				// forEachRepo always looks up the OPEN PR for a branch.
				pr.State = "OPEN"
			}
			json.NewEncoder(w).Encode(bitbucket.PaginatedPullRequests{
				Values: []bitbucket.PullRequest{pr},
			})
			return
		}

		// POST .../pullrequests/{id}/merge or /decline or /approve
		if r.Method == http.MethodPost && len(parts) >= 6 {
			if errMsg, bad := actionErrors[slug]; bad {
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(bitbucket.APIError{
					Error: bitbucket.APIErrorDetail{Message: errMsg},
				})
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(bitbucket.PullRequest{ID: 1})
			return
		}

		// PUT .../pullrequests/{id} (UpdatePR for reviewers)
		if r.Method == http.MethodPut && len(parts) >= 5 {
			if errMsg, bad := actionErrors[slug]; bad {
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(bitbucket.APIError{
					Error: bitbucket.APIErrorDetail{Message: errMsg},
				})
				return
			}
			json.NewEncoder(w).Encode(bitbucket.PullRequest{ID: 1})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
}

func newManagerForServer(srv *httptest.Server) *PRManager {
	transport := &hostRewriteTransport{
		base:    http.DefaultTransport,
		srvHost: srv.Listener.Addr().String(),
	}
	httpClient := &http.Client{Transport: transport}
	authApplier := bitbucket.BearerAuth(func() (string, error) { return "test-token", nil })
	client := bitbucket.NewClientWithHTTPClient(httpClient, authApplier)
	return NewPRManager(client)
}

// ---------- MergePRs ----------

func TestMergePRs_AllSuccess(t *testing.T) {
	prByRepo := map[string]bitbucket.PullRequest{
		"repo-a": {ID: 10, Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/pr/10"}}},
		"repo-b": {ID: 20, Links: bitbucket.PRLinks{HTML: bitbucket.LinkRef{Href: "https://bb.org/pr/20"}}},
	}

	srv := mockManagerServer(t, prByRepo, nil)
	defer srv.Close()

	mgr := newManagerForServer(srv)
	results := mgr.MergePRs("ws", []string{"repo-a", "repo-b"}, "feature/x", bitbucket.MergePRRequest{MergeStrategy: "squash"})

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("repo %q failed: %s", r.RepoSlug, r.Error)
		}
		if r.PRID == 0 {
			t.Errorf("repo %q has PRID=0", r.RepoSlug)
		}
	}
}

func TestMergePRs_PRNotFound(t *testing.T) {
	// Empty prByRepo → no PRs found
	srv := mockManagerServer(t, nil, nil)
	defer srv.Close()

	mgr := newManagerForServer(srv)
	results := mgr.MergePRs("ws", []string{"repo-a"}, "feature/nonexistent", bitbucket.MergePRRequest{})

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Success {
		t.Error("expected failure when PR not found")
	}
	if !strings.Contains(results[0].Error, "no OPEN PR found") {
		t.Errorf("error = %q, want to contain 'no OPEN PR found'", results[0].Error)
	}
}

func TestMergePRs_ActionError(t *testing.T) {
	prByRepo := map[string]bitbucket.PullRequest{
		"repo-a": {ID: 10},
	}
	actionErrors := map[string]string{
		"repo-a": "merge conflict",
	}

	srv := mockManagerServer(t, prByRepo, actionErrors)
	defer srv.Close()

	mgr := newManagerForServer(srv)
	results := mgr.MergePRs("ws", []string{"repo-a"}, "feature/x", bitbucket.MergePRRequest{})

	if results[0].Success {
		t.Error("expected failure for merge conflict")
	}
	if !strings.Contains(results[0].Error, "merge conflict") {
		t.Errorf("error = %q, want 'merge conflict'", results[0].Error)
	}
}

// ---------- DeclinePRs ----------

func TestDeclinePRs_AllSuccess(t *testing.T) {
	prByRepo := map[string]bitbucket.PullRequest{
		"repo-a": {ID: 10},
		"repo-b": {ID: 20},
	}

	srv := mockManagerServer(t, prByRepo, nil)
	defer srv.Close()

	mgr := newManagerForServer(srv)
	results := mgr.DeclinePRs("ws", []string{"repo-a", "repo-b"}, "feature/x")

	for _, r := range results {
		if !r.Success {
			t.Errorf("repo %q failed: %s", r.RepoSlug, r.Error)
		}
	}
}

// ---------- ApprovePRs ----------

func TestApprovePRs_AllSuccess(t *testing.T) {
	prByRepo := map[string]bitbucket.PullRequest{
		"repo-a": {ID: 10},
	}

	srv := mockManagerServer(t, prByRepo, nil)
	defer srv.Close()

	mgr := newManagerForServer(srv)
	results := mgr.ApprovePRs("ws", []string{"repo-a"}, "feature/x")

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if !results[0].Success {
		t.Errorf("expected success, got error: %s", results[0].Error)
	}
}

// ---------- AddReviewers ----------

func TestAddReviewers_Success(t *testing.T) {
	prByRepo := map[string]bitbucket.PullRequest{
		"repo-a": {ID: 10, Reviewers: []bitbucket.PRReviewer{{UUID: "{existing}"}}},
	}

	srv := mockManagerServer(t, prByRepo, nil)
	defer srv.Close()

	mgr := newManagerForServer(srv)
	results := mgr.AddReviewers("ws", []string{"repo-a"}, "feature/x", []bitbucket.PRReviewer{{UUID: "{new}"}})

	if !results[0].Success {
		t.Errorf("expected success, got error: %s", results[0].Error)
	}
}

// ---------- Concurrency ----------

func TestForEachRepo_Concurrency(t *testing.T) {
	var requestCount atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(bitbucket.PaginatedPullRequests{
				Values: []bitbucket.PullRequest{{ID: 1, State: "OPEN"}},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(bitbucket.PullRequest{ID: 1})
	}))
	defer srv.Close()

	repos := make([]string, 15)
	for i := range repos {
		repos[i] = fmt.Sprintf("repo-%02d", i)
	}

	mgr := newManagerForServer(srv)
	results := mgr.ApprovePRs("ws", repos, "feature/x")

	if len(results) != 15 {
		t.Errorf("len(results) = %d, want 15", len(results))
	}
	// Each repo makes 2 requests: GET (find PR) + POST (approve)
	if int(requestCount.Load()) != 30 {
		t.Errorf("request count = %d, want 30", requestCount.Load())
	}
}

func TestForEachRepo_SortedResults(t *testing.T) {
	prByRepo := map[string]bitbucket.PullRequest{
		"zeta":  {ID: 1},
		"alpha": {ID: 2},
		"beta":  {ID: 3},
	}

	srv := mockManagerServer(t, prByRepo, nil)
	defer srv.Close()

	mgr := newManagerForServer(srv)
	results := mgr.ApprovePRs("ws", []string{"zeta", "alpha", "beta"}, "feature/x")

	expected := []string{"alpha", "beta", "zeta"}
	for i, want := range expected {
		if results[i].RepoSlug != want {
			t.Errorf("results[%d].RepoSlug = %q, want %q", i, results[i].RepoSlug, want)
		}
	}
}

// ---------- NewPRManager ----------

func TestNewPRManager_NotNil(t *testing.T) {
	mgr := NewPRManager(nil)
	if mgr == nil {
		t.Fatal("NewPRManager returned nil")
	}
}
