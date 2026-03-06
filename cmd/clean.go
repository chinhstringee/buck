package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/chinhstringee/buck/internal/bitbucket"
	"github.com/chinhstringee/buck/internal/cleanup"
	"github.com/chinhstringee/buck/internal/config"
	"github.com/chinhstringee/buck/internal/gitutil"
)

var (
	cleanFlagGroup       string
	cleanFlagRepos       string
	cleanFlagInteractive bool
	cleanFlagDryRun      bool
	cleanFlagYes         bool
	cleanFlagMerged      bool
)

var cleanCmd = &cobra.Command{
	Use:   "clean [branch-name]",
	Short: "Delete branches across repos (or --merged for merged branch cleanup)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runClean,
}

func init() {
	cleanCmd.Flags().StringVarP(&cleanFlagGroup, "group", "g", "", "repo group from config")
	cleanCmd.Flags().StringVarP(&cleanFlagRepos, "repos", "r", "", "comma-separated repo slugs")
	cleanCmd.Flags().BoolVarP(&cleanFlagInteractive, "interactive", "i", false, "select repos interactively")
	cleanCmd.Flags().BoolVar(&cleanFlagDryRun, "dry-run", false, "preview actions without executing")
	cleanCmd.Flags().BoolVarP(&cleanFlagYes, "yes", "y", false, "skip confirmation prompt")
	cleanCmd.Flags().BoolVar(&cleanFlagMerged, "merged", false, "delete all branches with merged PRs")

	_ = cleanCmd.RegisterFlagCompletionFunc("group", completeGroupNames)
	_ = cleanCmd.RegisterFlagCompletionFunc("repos", completeRepoSlugs)

	rootCmd.AddCommand(cleanCmd)
}

func runClean(cmd *cobra.Command, args []string) error {
	if len(args) == 0 && !cleanFlagMerged {
		return fmt.Errorf("branch name required, or use --merged to clean all merged branches")
	}

	var branchName string
	if len(args) > 0 {
		branchName = args[0]
	}

	var repos []string
	var workspace string

	autoDetect := cleanFlagRepos == "" && cleanFlagGroup == "" && !cleanFlagInteractive

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if autoDetect {
		ws, repoSlug, gitErr := gitutil.ParseBitbucketRemote()
		if gitErr == nil {
			workspace = ws
			repos = []string{repoSlug}
		} else if cfg.Workspace != "" {
			workspace = cfg.Workspace
		} else {
			return fmt.Errorf("no workspace configured and not in a Bitbucket repo\n  Hint: use 'buck clean <branch> --repos <repo>' to specify explicitly")
		}
	} else {
		if cfg.Workspace == "" {
			return fmt.Errorf("workspace not configured in .buck.yaml")
		}
		workspace = cfg.Workspace
	}

	authApplier, err := buildAuthApplier(cfg)
	if err != nil {
		return err
	}

	client := bitbucket.NewClient(authApplier)

	if len(repos) == 0 {
		repos, err = resolveTargetRepos(cleanFlagRepos, cleanFlagGroup, cleanFlagInteractive, cfg, client)
		if err != nil {
			return err
		}
		if len(repos) == 0 {
			return fmt.Errorf("no repositories selected")
		}
	}

	bold := color.New(color.Bold)
	cleaner := cleanup.NewBranchCleaner(client, nil)

	if cleanFlagMerged {
		return runCleanMerged(bold, cleaner, workspace, repos)
	}

	return runCleanBranch(bold, cleaner, workspace, repos, branchName)
}

func runCleanBranch(bold *color.Color, cleaner *cleanup.BranchCleaner, workspace string, repos []string, branchName string) error {
	if cleanFlagDryRun {
		bold.Printf("Dry run: would delete branch %q from:\n", branchName)
		for _, r := range repos {
			fmt.Printf("  - %s/%s\n", workspace, r)
		}
		return nil
	}

	if !cleanFlagYes {
		bold.Printf("Will delete branch %q from %d repos\n", branchName, len(repos))
		if !confirmAction("Proceed?") {
			fmt.Println("Aborted.")
			return nil
		}
	}

	bold.Printf("Deleting branch %q across %d repos...\n", branchName, len(repos))
	results := cleaner.DeleteBranch(workspace, repos, branchName)
	cleanup.PrintResults(results)
	return nil
}

func runCleanMerged(bold *color.Color, cleaner *cleanup.BranchCleaner, workspace string, repos []string) error {
	if cleanFlagDryRun {
		bold.Printf("Dry run: would scan %d repos for merged branches to delete\n", len(repos))
		return nil
	}

	if !cleanFlagYes {
		bold.Printf("Will find and delete merged branches from %d repos\n", len(repos))
		if !confirmAction("Proceed?") {
			fmt.Println("Aborted.")
			return nil
		}
	}

	bold.Printf("Cleaning merged branches across %d repos...\n", len(repos))
	results := cleaner.DeleteMergedBranches(workspace, repos)
	cleanup.PrintResults(results)
	return nil
}
