package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/chinhstringee/buck/internal/bitbucket"
)

var (
	prViewFlagID    int
	prViewFlagState string
	prViewFlagFull  bool
)

var prViewCmd = &cobra.Command{
	Use:   "view [branch-name]",
	Short: "View a single pull request's details (read-only)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runPRView,
}

func init() {
	prViewCmd.Flags().IntVar(&prViewFlagID, "id", 0, "pull request id (requires exactly one resolved repo)")
	prViewCmd.Flags().StringVar(&prViewFlagState, "state", "OPEN", "PR state filter when resolving by branch: OPEN, MERGED, DECLINED, SUPERSEDED")
	prViewCmd.Flags().BoolVar(&prViewFlagFull, "full", false, "show full (untruncated) commit hash")

	_ = prViewCmd.RegisterFlagCompletionFunc("state", completeStaticValues([]string{"OPEN", "MERGED", "DECLINED", "SUPERSEDED"}))

	prCmd.AddCommand(prViewCmd)
}

func runPRView(cmd *cobra.Command, args []string) error {
	var branchArg string
	if len(args) > 0 {
		branchArg = args[0]
	}

	if branchArg == "" && prViewFlagID == 0 {
		return fmt.Errorf("branch name or --id required")
	}

	contextSelector := branchArg
	if prViewFlagID != 0 && contextSelector == "" && (prFlagRepos != "" || prFlagGroup != "" || prFlagInteractive) {
		// resolvePRContext requires a non-empty selector when not auto-detecting;
		// this placeholder is never used as a branch name on the --id path below.
		contextSelector = strconv.Itoa(prViewFlagID)
	}

	ctx, err := resolvePRContext(contextSelector)
	if err != nil {
		return err
	}

	if prViewFlagID != 0 {
		if len(ctx.repos) != 1 {
			return fmt.Errorf("pr view --id requires exactly one repository, got %d", len(ctx.repos))
		}
		repo := ctx.repos[0]
		pr, err := ctx.client.GetPullRequest(ctx.workspace, repo, prViewFlagID)
		if err != nil {
			return fmt.Errorf("failed to get PR #%d in %s/%s: %w", prViewFlagID, ctx.workspace, repo, err)
		}
		printPRDetail(ctx.workspace, repo, pr)
		return nil
	}

	state := strings.ToUpper(prViewFlagState)
	if state == "" {
		state = "OPEN"
	}

	var anyMatch bool
	var anyError bool
	bold := color.New(color.Bold)

	for _, repo := range ctx.repos {
		prs, err := ctx.client.FindPRsByBranch(ctx.workspace, repo, ctx.branchName, state)
		if err != nil {
			anyError = true
			fmt.Printf("  %s: %v\n", repo, err)
			continue
		}

		switch len(prs) {
		case 0:
			fmt.Printf("  %s: no %s PR found for branch %q\n", repo, state, ctx.branchName)
		case 1:
			anyMatch = true
			bold.Printf("\n  %s\n", repo)
			printPRDetail(ctx.workspace, repo, &prs[0])
		default:
			ids := make([]string, len(prs))
			for i, pr := range prs {
				ids[i] = strconv.Itoa(pr.ID)
			}
			fmt.Printf("  %s: %d %s PRs match branch %q (ids: %s) — rerun with --id to pick one\n",
				repo, len(prs), state, ctx.branchName, strings.Join(ids, ", "))
		}
	}

	if !anyMatch && !anyError {
		return fmt.Errorf("no %s PR found for branch %q in any targeted repo", state, ctx.branchName)
	}

	return nil
}

// printPRDetail prints a single PR's details in a human-readable form.
func printPRDetail(workspace, repo string, pr *bitbucket.PullRequest) {
	bold := color.New(color.Bold)
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()

	stateColor := yellow
	switch pr.State {
	case "OPEN":
		stateColor = green
	case "DECLINED", "SUPERSEDED":
		stateColor = red
	}

	bold.Printf("  #%d %s\n", pr.ID, pr.Title)
	fmt.Printf("  State:       %s\n", stateColor(pr.State))
	fmt.Printf("  Branch:      %s -> %s\n", pr.Source.Branch.Name, pr.Destination.Branch.Name)
	fmt.Printf("  Head:        %s\n", formatCommitHash(pr.Source.Commit))
	fmt.Printf("  Author:      %s\n", pr.Author.DisplayName)
	fmt.Printf("  Reviewers:   %s\n", formatReviewers(pr.Participants))
	fmt.Printf("  Created:     %s\n", pr.CreatedOn)
	fmt.Printf("  Updated:     %s\n", pr.UpdatedOn)
	fmt.Printf("  URL:         %s\n", pr.Links.HTML.Href)
	if pr.Description != "" {
		fmt.Printf("  Description:\n")
		for _, line := range strings.Split(pr.Description, "\n") {
			fmt.Printf("    %s\n", line)
		}
	}
}

// formatCommitHash returns the short hash, or the full hash when --full is set.
func formatCommitHash(commit *bitbucket.PRCommitRef) string {
	if commit == nil || commit.Hash == "" {
		return "(unknown)"
	}
	if prViewFlagFull || len(commit.Hash) <= 7 {
		return commit.Hash
	}
	return commit.Hash[:7]
}

// formatReviewers renders reviewer display names with an approval marker.
func formatReviewers(participants []bitbucket.PRParticipant) string {
	var reviewers []bitbucket.PRParticipant
	for _, p := range participants {
		if p.Role == "REVIEWER" {
			reviewers = append(reviewers, p)
		}
	}
	if len(reviewers) == 0 {
		return "(none)"
	}

	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	parts := make([]string, len(reviewers))
	for i, r := range reviewers {
		mark := yellow("pending")
		if r.Approved {
			mark = green("approved")
		}
		parts[i] = fmt.Sprintf("%s (%s)", r.User.DisplayName, mark)
	}
	return strings.Join(parts, ", ")
}
