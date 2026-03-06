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
	statusFlagGroup       string
	statusFlagRepos       string
	statusFlagInteractive bool
	statusFlagMine        bool
	statusFlagAuthor      string
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show open PR status across repos",
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().StringVarP(&statusFlagGroup, "group", "g", "", "repo group from config")
	statusCmd.Flags().StringVarP(&statusFlagRepos, "repos", "r", "", "comma-separated repo slugs")
	statusCmd.Flags().BoolVarP(&statusFlagInteractive, "interactive", "i", false, "select repos interactively")
	statusCmd.Flags().BoolVar(&statusFlagMine, "mine", false, "show only my PRs")
	statusCmd.Flags().StringVar(&statusFlagAuthor, "author", "", "filter by author nickname")

	_ = statusCmd.RegisterFlagCompletionFunc("group", completeGroupNames)
	_ = statusCmd.RegisterFlagCompletionFunc("repos", completeRepoSlugs)

	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	var repos []string
	var workspace string

	// Auto-detect mode: no flags
	autoDetect := statusFlagRepos == "" && statusFlagGroup == "" && !statusFlagInteractive

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if autoDetect {
		// Try CWD detection first, fall back to config workspace
		ws, repoSlug, gitErr := gitutil.ParseBitbucketRemote()
		if gitErr == nil {
			workspace = ws
			repos = []string{repoSlug}
		} else if cfg.Workspace != "" {
			workspace = cfg.Workspace
			// Will use all workspace repos below
		} else {
			return fmt.Errorf("no workspace configured and not in a Bitbucket repo\n  Hint: use 'buck status --repos <repo>' or configure .buck.yaml")
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

	// Resolve repos if not auto-detected from CWD
	if len(repos) == 0 {
		repos, err = resolveTargetRepos(statusFlagRepos, statusFlagGroup, statusFlagInteractive, cfg, client)
		if err != nil {
			return err
		}
		if len(repos) == 0 {
			return fmt.Errorf("no repositories selected")
		}
	}

	bold := color.New(color.Bold)
	bold.Printf("Fetching open PRs across %d repos...\n", len(repos))

	fetcher := dashboard.NewFetcher(client)
	filters := dashboard.PRFilters{
		Author: statusFlagAuthor,
		Mine:   statusFlagMine,
	}

	results := fetcher.FetchAllPRs(workspace, repos, filters)
	dashboard.PrintDashboard(results)

	return nil
}
