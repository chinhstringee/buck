package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/chinhstringee/buck/internal/bitbucket"
	"github.com/chinhstringee/buck/internal/pullrequest"
)

var (
	prMergeFlagStrategy     string
	prMergeFlagCloseBranch  bool
	prMergeFlagYes          bool
)

var prMergeCmd = &cobra.Command{
	Use:   "merge [branch-name]",
	Short: "Merge pull requests by branch name across repos",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runPRMerge,
}

func init() {
	prMergeCmd.Flags().StringVar(&prMergeFlagStrategy, "strategy", "merge_commit", "merge strategy: merge_commit, squash, fast_forward")
	prMergeCmd.Flags().BoolVar(&prMergeFlagCloseBranch, "close-branch", false, "close source branch after merge")
	prMergeCmd.Flags().BoolVarP(&prMergeFlagYes, "yes", "y", false, "skip confirmation prompt")

	_ = prMergeCmd.RegisterFlagCompletionFunc("strategy", completeStaticValues([]string{"merge_commit", "squash", "fast_forward"}))

	prCmd.AddCommand(prMergeCmd)
}

func runPRMerge(cmd *cobra.Command, args []string) error {
	var branchArg string
	if len(args) > 0 {
		branchArg = args[0]
	}

	validStrategies := map[string]bool{"merge_commit": true, "squash": true, "fast_forward": true}
	if !validStrategies[prMergeFlagStrategy] {
		return fmt.Errorf("invalid merge strategy %q (valid: merge_commit, squash, fast_forward)", prMergeFlagStrategy)
	}

	ctx, err := resolvePRContext(branchArg)
	if err != nil {
		return err
	}

	bold := color.New(color.Bold)

	if prFlagDryRun {
		bold.Printf("Dry run: would merge PRs from branch %q in:\n", ctx.branchName)
		for _, r := range ctx.repos {
			fmt.Printf("  - %s/%s\n", ctx.workspace, r)
		}
		return nil
	}

	if !prMergeFlagYes {
		bold.Printf("Will merge PRs from branch %q across %d repos (strategy: %s)\n", ctx.branchName, len(ctx.repos), prMergeFlagStrategy)
		if !confirmAction("Proceed?") {
			fmt.Println("Aborted.")
			return nil
		}
	}

	bold.Printf("Merging PRs from %q across %d repos...\n", ctx.branchName, len(ctx.repos))

	mgr := pullrequest.NewPRManager(ctx.client)
	req := bitbucket.MergePRRequest{
		MergeStrategy:     prMergeFlagStrategy,
		CloseSourceBranch: prMergeFlagCloseBranch,
	}
	results := mgr.MergePRs(ctx.workspace, ctx.repos, ctx.branchName, req)
	pullrequest.PrintActionResults("Merge", results)

	return nil
}
