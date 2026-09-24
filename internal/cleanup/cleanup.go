package cleanup

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/fatih/color"
	"github.com/chinhstringee/buck/internal/bitbucket"
)

// Default branches that should never be deleted.
var defaultProtectedBranches = []string{"main", "master", "develop", "staging", "production", "release"}

// Result holds the outcome of a branch deletion for one repo.
type Result struct {
	RepoSlug   string
	BranchName string
	Success    bool
	Error      string
	Skipped    bool // true if branch is protected
}

// BranchCleaner orchestrates parallel branch deletion across repos.
type BranchCleaner struct {
	client            *bitbucket.Client
	protectedBranches map[string]bool
}

// NewBranchCleaner creates a new branch cleaner.
// extraProtected adds to the default protected branch list.
func NewBranchCleaner(client *bitbucket.Client, extraProtected []string) *BranchCleaner {
	protected := make(map[string]bool, len(defaultProtectedBranches)+len(extraProtected))
	for _, b := range defaultProtectedBranches {
		protected[b] = true
	}
	for _, b := range extraProtected {
		protected[b] = true
	}
	return &BranchCleaner{client: client, protectedBranches: protected}
}

// isProtected returns true if the branch should not be deleted.
func (bc *BranchCleaner) isProtected(branchName string) bool {
	return bc.protectedBranches[branchName]
}

// DeleteBranch deletes a named branch across repos concurrently.
func (bc *BranchCleaner) DeleteBranch(workspace string, repos []string, branchName string) []Result {
	if bc.isProtected(branchName) {
		results := make([]Result, len(repos))
		for i, r := range repos {
			results[i] = Result{
				RepoSlug:   r,
				BranchName: branchName,
				Skipped:    true,
				Error:      fmt.Sprintf("branch %q is protected", branchName),
			}
		}
		return results
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []Result
	)

	for _, repo := range repos {
		wg.Add(1)
		go func(repoSlug string) {
			defer wg.Done()

			result := Result{RepoSlug: repoSlug, BranchName: branchName}
			err := bc.client.DeleteBranch(workspace, repoSlug, branchName)
			if err != nil {
				errMsg := err.Error()
				// Treat 404 (already deleted) as a warning, not failure
				if strings.Contains(errMsg, "404") {
					result.Success = true
					result.Error = "already deleted"
				} else {
					result.Error = errMsg
				}
			} else {
				result.Success = true
			}

			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(repo)
	}

	wg.Wait()
	sortResults(results)
	return results
}

// DeleteMergedBranches finds and deletes branches that have merged PRs.
func (bc *BranchCleaner) DeleteMergedBranches(workspace string, repos []string) []Result {
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []Result
	)

	for _, repo := range repos {
		wg.Add(1)
		go func(repoSlug string) {
			defer wg.Done()

			branches, err := bc.client.ListMergedPRBranches(workspace, repoSlug)
			if err != nil {
				mu.Lock()
				results = append(results, Result{
					RepoSlug: repoSlug,
					Error:    fmt.Sprintf("failed to list merged branches: %s", err),
				})
				mu.Unlock()
				return
			}

			for _, branch := range branches {
				result := Result{RepoSlug: repoSlug, BranchName: branch}

				if bc.isProtected(branch) {
					result.Skipped = true
					result.Error = "protected"
					mu.Lock()
					results = append(results, result)
					mu.Unlock()
					continue
				}

				err := bc.client.DeleteBranch(workspace, repoSlug, branch)
				if err != nil {
					errMsg := err.Error()
					if strings.Contains(errMsg, "404") {
						result.Success = true
						result.Error = "already deleted"
					} else {
						result.Error = errMsg
					}
				} else {
					result.Success = true
				}

				mu.Lock()
				results = append(results, result)
				mu.Unlock()
			}
		}(repo)
	}

	wg.Wait()
	sortResults(results)
	return results
}

func sortResults(results []Result) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].RepoSlug != results[j].RepoSlug {
			return results[i].RepoSlug < results[j].RepoSlug
		}
		return results[i].BranchName < results[j].BranchName
	})
}

// PrintResults displays a colored summary of branch cleanup results.
func PrintResults(results []Result) {
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()

	succeeded := 0
	failed := 0
	skipped := 0

	fmt.Println()
	for _, r := range results {
		branchLabel := r.BranchName
		if branchLabel == "" {
			branchLabel = "(unknown)"
		}

		if r.Skipped {
			skipped++
			fmt.Printf("  %s %-25s %-30s %s\n", yellow("–"), r.RepoSlug, branchLabel, r.Error)
		} else if r.Success {
			succeeded++
			msg := "deleted"
			if r.Error != "" {
				msg = r.Error // "already deleted"
			}
			fmt.Printf("  %s %-25s %-30s %s\n", green("✓"), r.RepoSlug, branchLabel, msg)
		} else {
			failed++
			fmt.Printf("  %s %-25s %-30s %s\n", red("✗"), r.RepoSlug, branchLabel, r.Error)
		}
	}

	fmt.Printf("\n%s %s deleted, %s skipped, %s failed\n",
		bold("Summary:"),
		green(fmt.Sprintf("%d", succeeded)),
		yellow(fmt.Sprintf("%d", skipped)),
		red(fmt.Sprintf("%d", failed)),
	)
}
