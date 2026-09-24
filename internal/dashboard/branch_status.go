package dashboard

import (
	"sort"
	"strings"
	"sync"

	"github.com/chinhstringee/buck/internal/bitbucket"
)

// Containment reports whether a branch's head is reachable from one target
// branch (e.g. release, master).
type Containment struct {
	Target    string
	Contained bool
	Error     string // e.g. "no <target> branch" when the target ref is missing
}

// BranchStatus holds one repo's branch-status result: head commit,
// containment against each requested target, and all PRs for the branch in
// any state.
type BranchStatus struct {
	RepoSlug    string
	Head        string
	Containment []Containment
	PRs         []bitbucket.PullRequest
	Error       string // e.g. "no branch" when branchName doesn't exist in this repo
}

// FetchBranchStatus concurrently resolves, per repo, a branch's head,
// containment in each `against` branch, and all PRs for the branch in any
// state. A branch or target missing in one repo is reported on that repo's
// result rather than failing the whole run.
func (f *Fetcher) FetchBranchStatus(workspace string, repos []string, branchName string, against []string) []BranchStatus {
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []BranchStatus
	)

	for _, repo := range repos {
		wg.Add(1)
		go func(repoSlug string) {
			defer wg.Done()
			result := fetchOneBranchStatus(f.client, workspace, repoSlug, branchName, against)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(repo)
	}

	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		return results[i].RepoSlug < results[j].RepoSlug
	})

	return results
}

// fetchOneBranchStatus resolves one repo's branch status. A missing source
// branch short-circuits the repo (no containment/PR calls made). A missing
// target branch is reported only on that target's Containment entry, so
// other targets and the PR lookup still complete.
func fetchOneBranchStatus(client *bitbucket.Client, workspace, repoSlug, branchName string, against []string) BranchStatus {
	result := BranchStatus{RepoSlug: repoSlug}

	branch, err := client.GetBranch(workspace, repoSlug, branchName)
	if err != nil {
		if isNotFound(err) {
			result.Error = "no branch"
		} else {
			result.Error = err.Error()
		}
		return result
	}
	result.Head = shortHash(branch.Target.Hash)

	result.Containment = make([]Containment, len(against))
	for i, target := range against {
		commits, err := client.CommitsAhead(workspace, repoSlug, branchName, target, 1)
		if err != nil {
			if isNotFound(err) {
				result.Containment[i] = Containment{Target: target, Error: "no " + target + " branch"}
			} else {
				result.Containment[i] = Containment{Target: target, Error: err.Error()}
			}
			continue
		}
		result.Containment[i] = Containment{Target: target, Contained: len(commits) == 0}
	}

	prs, err := client.FindAllPRsByBranch(workspace, repoSlug, branchName)
	if err != nil {
		result.Error = err.Error()
	} else {
		result.PRs = prs
	}

	return result
}

// isNotFound reports whether err came from a Bitbucket 404 response.
func isNotFound(err error) bool {
	return strings.Contains(err.Error(), "404")
}

// shortHash returns the first 7 characters of a commit hash, or the whole
// string when it is shorter.
func shortHash(hash string) string {
	if len(hash) <= 7 {
		return hash
	}
	return hash[:7]
}
