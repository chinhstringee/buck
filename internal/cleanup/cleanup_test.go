package cleanup

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/chinhstringee/buck/internal/bitbucket"
)

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

func newCleanerForServer(srv *httptest.Server, extraProtected []string) *BranchCleaner {
	transport := &hostRewriteTransport{
		base:    http.DefaultTransport,
		srvHost: srv.Listener.Addr().String(),
	}
	httpClient := &http.Client{Transport: transport}
	authApplier := bitbucket.BearerAuth(func() (string, error) { return "test-token", nil })
	client := bitbucket.NewClientWithHTTPClient(httpClient, authApplier)
	return NewBranchCleaner(client, extraProtected)
}

// ---------- DeleteBranch ----------

func TestDeleteBranch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	bc := newCleanerForServer(srv, nil)
	results := bc.DeleteBranch("ws", []string{"repo-a", "repo-b"}, "feature/old")

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("repo %q failed: %s", r.RepoSlug, r.Error)
		}
	}
}

func TestDeleteBranch_ProtectedBranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not make API call for protected branch")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	bc := newCleanerForServer(srv, nil)
	results := bc.DeleteBranch("ws", []string{"repo-a"}, "main")

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if !results[0].Skipped {
		t.Error("expected Skipped=true for protected branch")
	}
	if results[0].Success {
		t.Error("expected Success=false for skipped branch")
	}
}

func TestDeleteBranch_ReleaseIsProtectedByDefault(t *testing.T) {
	// "release" is the shared TEST-promotion branch (feature -> release -> master);
	// buck clean must refuse to delete it without requiring an explicit
	// --protected-branches config entry.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not make API call for the release branch")
	}))
	defer srv.Close()

	bc := newCleanerForServer(srv, nil)
	results := bc.DeleteBranch("ws", []string{"repo-a"}, "release")

	if !results[0].Skipped {
		t.Error("expected Skipped=true for release branch")
	}
}

func TestDeleteBranch_CustomProtected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not make API call for protected branch")
	}))
	defer srv.Close()

	bc := newCleanerForServer(srv, []string{"release"})
	results := bc.DeleteBranch("ws", []string{"repo-a"}, "release")

	if !results[0].Skipped {
		t.Error("expected Skipped=true for custom protected branch")
	}
}

func TestDeleteBranch_AlreadyDeleted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(bitbucket.APIError{
			Error: bitbucket.APIErrorDetail{Message: "Resource not found"},
		})
	}))
	defer srv.Close()

	bc := newCleanerForServer(srv, nil)
	results := bc.DeleteBranch("ws", []string{"repo-a"}, "feature/gone")

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	// 404 should be treated as success (already deleted)
	if !results[0].Success {
		t.Errorf("expected Success=true for 404, got error: %s", results[0].Error)
	}
}

func TestDeleteBranch_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(bitbucket.APIError{
			Error: bitbucket.APIErrorDetail{Message: "Forbidden"},
		})
	}))
	defer srv.Close()

	bc := newCleanerForServer(srv, nil)
	results := bc.DeleteBranch("ws", []string{"repo-a"}, "feature/x")

	if results[0].Success {
		t.Error("expected failure for 403")
	}
	if results[0].Error == "" {
		t.Error("expected error message")
	}
}

func TestDeleteBranch_Concurrency(t *testing.T) {
	var requestCount atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	repos := make([]string, 20)
	for i := range repos {
		repos[i] = "repo"
	}

	bc := newCleanerForServer(srv, nil)
	results := bc.DeleteBranch("ws", repos, "feature/x")

	if len(results) != 20 {
		t.Errorf("len(results) = %d, want 20", len(results))
	}
	if int(requestCount.Load()) != 20 {
		t.Errorf("request count = %d, want 20", requestCount.Load())
	}
}

func TestDeleteBranch_SortedResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	bc := newCleanerForServer(srv, nil)
	results := bc.DeleteBranch("ws", []string{"zeta", "alpha", "beta"}, "feature/x")

	expected := []string{"alpha", "beta", "zeta"}
	for i, want := range expected {
		if results[i].RepoSlug != want {
			t.Errorf("results[%d].RepoSlug = %q, want %q", i, results[i].RepoSlug, want)
		}
	}
}

// ---------- DeleteMergedBranches ----------

func TestDeleteMergedBranches_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

		// GET pullrequests (list merged PRs)
		if r.Method == http.MethodGet && len(parts) >= 5 && parts[4] == "pullrequests" {
			json.NewEncoder(w).Encode(bitbucket.PaginatedPullRequests{
				Values: []bitbucket.PullRequest{
					{ID: 1, Source: bitbucket.PRBranchRef{Branch: bitbucket.PRBranchName{Name: "feature/done"}}},
					{ID: 2, Source: bitbucket.PRBranchRef{Branch: bitbucket.PRBranchName{Name: "feature/also-done"}}},
					{ID: 3, Source: bitbucket.PRBranchRef{Branch: bitbucket.PRBranchName{Name: "main"}}}, // protected
				},
			})
			return
		}

		// DELETE branch
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	bc := newCleanerForServer(srv, nil)
	results := bc.DeleteMergedBranches("ws", []string{"repo-a"})

	// 3 branches: 2 deleted + 1 protected (main)
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}

	var deleted, skipped int
	for _, r := range results {
		if r.Skipped {
			skipped++
			if r.BranchName != "main" {
				t.Errorf("unexpected skipped branch: %s", r.BranchName)
			}
		} else if r.Success {
			deleted++
		}
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
}

// ---------- NewBranchCleaner ----------

func TestNewBranchCleaner_DefaultProtected(t *testing.T) {
	bc := NewBranchCleaner(nil, nil)
	for _, name := range []string{"main", "master", "develop", "staging", "production", "release"} {
		if !bc.isProtected(name) {
			t.Errorf("%q should be protected by default", name)
		}
	}
	if bc.isProtected("feature/x") {
		t.Error("feature/x should not be protected")
	}
}
