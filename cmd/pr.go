package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/chinhstringee/buck/internal/bitbucket"
	"github.com/chinhstringee/buck/internal/config"
	"github.com/chinhstringee/buck/internal/gitutil"
	"github.com/chinhstringee/buck/internal/pullrequest"
)

var (
	prFlagGroup       string
	prFlagRepos       string
	prFlagDryRun      bool
	prFlagDestination string
	prFlagInteractive bool
)

var prCmd = &cobra.Command{
	Use:   "pr [branch-name]",
	Short: "Pull request operations (create, merge, decline, approve, list)",
	Long:  "Create and manage pull requests across multiple Bitbucket repos.\nRun without subcommand to create PRs (backward compatible).",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runPR,
}

func init() {
	// Shared flags available to all pr subcommands
	prCmd.PersistentFlags().StringVarP(&prFlagGroup, "group", "g", "", "repo group from config")
	prCmd.PersistentFlags().StringVarP(&prFlagRepos, "repos", "r", "", "comma-separated repo slugs")
	prCmd.PersistentFlags().BoolVar(&prFlagDryRun, "dry-run", false, "preview actions without executing")
	prCmd.PersistentFlags().BoolVarP(&prFlagInteractive, "interactive", "i", false, "select repos interactively")

	// Create-only flag
	prCmd.Flags().StringVarP(&prFlagDestination, "destination", "d", "", "destination branch (default: master)")

	_ = prCmd.RegisterFlagCompletionFunc("group", completeGroupNames)
	_ = prCmd.RegisterFlagCompletionFunc("repos", completeRepoSlugs)
	_ = prCmd.RegisterFlagCompletionFunc("destination", completeBranchNames)

	rootCmd.AddCommand(prCmd)
}

func runPR(cmd *cobra.Command, args []string) error {
	var branchName string
	var repos []string
	var workspace string

	// Auto-detect mode: no args and no --repos/--group flags
	autoDetect := len(args) == 0 && prFlagRepos == "" && prFlagGroup == "" && !prFlagInteractive

	if autoDetect {
		hint := "\n  Hint: use 'buck pr <branch> --repos <repo>' to specify explicitly"
		branch, err := gitutil.CurrentBranch()
		if err != nil {
			return fmt.Errorf("auto-detect failed: %w%s", err, hint)
		}
		branchName = branch

		ws, repoSlug, err := gitutil.ParseBitbucketRemote()
		if err != nil {
			return fmt.Errorf("auto-detect failed: %w%s", err, hint)
		}
		workspace = ws
		repos = []string{repoSlug}
	} else {
		if len(args) == 0 {
			return fmt.Errorf("branch name required when using --repos, --group, or --interactive")
		}
		branchName = args[0]
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Use remote workspace in auto-detect mode, config workspace otherwise
	if !autoDetect {
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

	if !autoDetect {
		repos, err = resolveTargetRepos(prFlagRepos, prFlagGroup, prFlagInteractive, cfg, client)
		if err != nil {
			return err
		}
		if len(repos) == 0 {
			return fmt.Errorf("no repositories selected")
		}
	}

	bold := color.New(color.Bold)

	if prFlagDryRun {
		dest := prFlagDestination
		if dest == "" {
			dest = "master"
		}
		bold.Printf("Dry run: would create PRs from %q to %q in:\n", branchName, dest)
		for _, r := range repos {
			fmt.Printf("  - %s/%s\n", workspace, r)
		}
		return nil
	}

	bold.Printf("Creating PRs from %q across %d repos...\n", branchName, len(repos))

	pc := pullrequest.NewPRCreator(client)
	results := pc.CreatePRs(workspace, repos, branchName, prFlagDestination)
	pullrequest.PrintResults(results)

	return nil
}
