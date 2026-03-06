package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/chinhstringee/buck/internal/bitbucket"
	"github.com/chinhstringee/buck/internal/config"
	"github.com/chinhstringee/buck/internal/dashboard"
	"github.com/chinhstringee/buck/internal/gitutil"
)

var (
	prListFlagState  string
	prListFlagMine   bool
	prListFlagAuthor string
)

var prListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pull requests across repos",
	Args:  cobra.NoArgs,
	RunE:  runPRList,
}

func init() {
	prListCmd.Flags().StringVar(&prListFlagState, "state", "OPEN", "PR state filter: OPEN, MERGED, DECLINED")
	prListCmd.Flags().BoolVar(&prListFlagMine, "mine", false, "show only my PRs")
	prListCmd.Flags().StringVar(&prListFlagAuthor, "author", "", "filter by author nickname")

	_ = prListCmd.RegisterFlagCompletionFunc("state", completeStaticValues([]string{"OPEN", "MERGED", "DECLINED", "SUPERSEDED"}))

	prCmd.AddCommand(prListCmd)
}

func runPRList(cmd *cobra.Command, args []string) error {
	var repos []string
	var workspace string

	autoDetect := prFlagRepos == "" && prFlagGroup == "" && !prFlagInteractive

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
			return fmt.Errorf("no workspace configured and not in a Bitbucket repo")
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
		repos, err = resolveTargetRepos(prFlagRepos, prFlagGroup, prFlagInteractive, cfg, client)
		if err != nil {
			return err
		}
		if len(repos) == 0 {
			return fmt.Errorf("no repositories selected")
		}
	}

	bold := color.New(color.Bold)
	bold.Printf("Listing %s PRs across %d repos...\n", prListFlagState, len(repos))

	fetcher := dashboard.NewFetcher(client)
	filters := dashboard.PRFilters{
		Author: prListFlagAuthor,
		Mine:   prListFlagMine,
		State:  prListFlagState,
	}
	results := fetcher.FetchAllPRs(workspace, repos, filters)
	dashboard.PrintDashboard(results)

	return nil
}
