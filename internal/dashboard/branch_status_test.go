package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chinhstringee/buck/internal/bitbucket"
)

// branchStatusRepoConfig describes one repo's canned server behavior for
// mockBranchStatusServer.
type branchStatusRepoConfig struct {
	branchMissing bool
	head          string
	commitsAhead  map[string][]bitbucket.Commit // target -> commits ahead (empty/nil = contained)
	missingTarget map[string]bool               // target -> respond 404 for that containment check
	prs           []bitbucket.PullRequest
}

// mockBranchStatusServer serves GetBranch, CommitsAhead (commits?include=...),
// and FindAllPRsByBranch (pullrequests?q=...) for the repos in cfg, keyed by
// repo slug.
func mockBranchStatusServer(t *testing.T, cfg map[string]branchStatusRepoConfig) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.Trim(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		if len(parts) < 4 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		slug := parts[3]
		repoCfg, ok := cfg[slug]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		switch {
		case len(parts) >= 6 && parts[4] == "refs" && parts[5] == "branches":
			if repoCfg.branchMissing {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(bitbucket.APIError{Error: bitbucket.APIErrorDetail{Message: "Branch not found"}})
				return
			}
			json.NewEncoder(w).Encode(bitbucket.Branch{Target: bitbucket.BranchTarget{Hash: repoCfg.head}})
		case len(parts) >= 5 && parts[4] == "commits":
			exclude := r.URL.Query().Get("exclude")
			if repoCfg.missingTarget[exclude] {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(bitbucket.APIError{Error: bitbucket.APIErrorDetail{Message: "Commit not found"}})
				return
			}
			json.NewEncoder(w).Encode(bitbucket.PaginatedCommits{Values: repoCfg.commitsAhead[exclude]})
		case len(parts) >= 5 && parts[4] == "pullrequests":
			json.NewEncoder(w).Encode(bitbucket.PaginatedPullRequests{Values: repoCfg.prs})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestFetchBranchStatus_Contained(t *testing.T) {
	srv := mockBranchStatusServer(t, map[string]branchStatusRepoConfig{
		"repo-a": {
			head:         "abcdef1234567",
			commitsAhead: map[string][]bitbucket.Commit{"release": nil},
		},
	})
	defer srv.Close()

	f := newFetcherForServer(srv)
	results := f.FetchBranchStatus("ws", []string{"repo-a"}, "feature/x", []string{"release"})

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	r := results[0]
	if r.Error != "" {
		t.Fatalf("unexpected repo error: %s", r.Error)
	}
	if r.Head != "abcdef1" {
		t.Errorf("Head = %q, want abcdef1 (short)", r.Head)
	}
	if len(r.Containment) != 1 || !r.Containment[0].Contained {
		t.Errorf("Containment = %+v, want contained in release", r.Containment)
	}
}

func TestFetchBranchStatus_NotContained(t *testing.T) {
	srv := mockBranchStatusServer(t, map[string]branchStatusRepoConfig{
		"repo-a": {
			head:         "abcdef1234567",
			commitsAhead: map[string][]bitbucket.Commit{"master": {{Hash: "deadbeef"}}},
		},
	})
	defer srv.Close()

	f := newFetcherForServer(srv)
	results := f.FetchBranchStatus("ws", []string{"repo-a"}, "feature/x", []string{"master"})

	r := results[0]
	if len(r.Containment) != 1 || r.Containment[0].Contained {
		t.Errorf("Containment = %+v, want not contained in master", r.Containment)
	}
}

func TestFetchBranchStatus_BranchMissing(t *testing.T) {
	srv := mockBranchStatusServer(t, map[string]branchStatusRepoConfig{
		"repo-a": {branchMissing: true},
	})
	defer srv.Close()

	f := newFetcherForServer(srv)
	results := f.FetchBranchStatus("ws", []string{"repo-a"}, "feature/gone", []string{"release", "master"})

	r := results[0]
	if r.Error != "no branch" {
		t.Errorf("Error = %q, want %q", r.Error, "no branch")
	}
	if r.Head != "" || r.Containment != nil || r.PRs != nil {
		t.Errorf("expected no further data when branch missing, got %+v", r)
	}
}

func TestFetchBranchStatus_TargetMissingDoesNotFailRepo(t *testing.T) {
	srv := mockBranchStatusServer(t, map[string]branchStatusRepoConfig{
		"repo-a": {
			head:          "abcdef1234567",
			commitsAhead:  map[string][]bitbucket.Commit{"release": nil},
			missingTarget: map[string]bool{"viettel-release": true},
		},
	})
	defer srv.Close()

	f := newFetcherForServer(srv)
	results := f.FetchBranchStatus("ws", []string{"repo-a"}, "feature/x", []string{"release", "viettel-release"})

	r := results[0]
	if r.Error != "" {
		t.Fatalf("unexpected repo error: %s", r.Error)
	}
	if len(r.Containment) != 2 {
		t.Fatalf("len(Containment) = %d, want 2", len(r.Containment))
	}
	if !r.Containment[0].Contained {
		t.Errorf("release containment = %+v, want contained", r.Containment[0])
	}
	if r.Containment[1].Error == "" {
		t.Errorf("viettel-release containment = %+v, want a missing-target error", r.Containment[1])
	}
}

func TestFetchBranchStatus_MultiplePRs(t *testing.T) {
	srv := mockBranchStatusServer(t, map[string]branchStatusRepoConfig{
		"repo-a": {
			head:         "abcdef1234567",
			commitsAhead: map[string][]bitbucket.Commit{"release": nil, "master": {{Hash: "deadbeef"}}},
			prs: []bitbucket.PullRequest{
				{ID: 3865, State: "MERGED"},
				{ID: 3907, State: "DECLINED"},
			},
		},
	})
	defer srv.Close()

	f := newFetcherForServer(srv)
	results := f.FetchBranchStatus("ws", []string{"repo-a"}, "feature/x", []string{"release", "master"})

	r := results[0]
	if len(r.PRs) != 2 {
		t.Fatalf("len(PRs) = %d, want 2", len(r.PRs))
	}
	if r.PRs[0].ID != 3865 || r.PRs[1].ID != 3907 {
		t.Errorf("PRs = %+v, want ids 3865 then 3907", r.PRs)
	}
}

func TestFetchBranchStatus_OneRepoFailsOthersSucceed(t *testing.T) {
	srv := mockBranchStatusServer(t, map[string]branchStatusRepoConfig{
		"repo-a": {branchMissing: true},
		"repo-b": {
			head:         "1234567abcdef",
			commitsAhead: map[string][]bitbucket.Commit{"release": nil},
		},
	})
	defer srv.Close()

	f := newFetcherForServer(srv)
	results := f.FetchBranchStatus("ws", []string{"repo-a", "repo-b"}, "feature/x", []string{"release"})

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	// Sorted by slug: repo-a first.
	if results[0].RepoSlug != "repo-a" || results[0].Error != "no branch" {
		t.Errorf("results[0] = %+v, want repo-a with 'no branch'", results[0])
	}
	if results[1].RepoSlug != "repo-b" || results[1].Error != "" {
		t.Errorf("results[1] = %+v, want repo-b succeeding", results[1])
	}
}
