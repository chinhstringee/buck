package cmd

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/chinhstringee/buck/internal/bitbucket"
	"github.com/chinhstringee/buck/internal/config"
	"github.com/chinhstringee/buck/internal/dashboard"
)

var (
	branchStatusFlagGroup       string
	branchStatusFlagRepos       string
	branchStatusFlagInteractive bool
	branchStatusFlagAgainst     string
)

var branchStatusCmd = &cobra.Command{
	Use:   "branch-status <branch-name>",
	Short: "Show where a branch stands across repos: containment + PR state (read-only)",
	Long: "For each selected repo, reports the branch's head commit, whether it is\n" +
		"contained in each --against branch, and every PR for the branch in any\n" +
		"state. Read-only (GET only); a branch or target missing in one repo is\n" +
		"reported on that repo without failing the others.",
	Args: cobra.ExactArgs(1),
	RunE: runBranchStatus,
}

func init() {
	branchStatusCmd.Flags().StringVarP(&branchStatusFlagGroup, "group", "g", "", "repo group from config")
	branchStatusCmd.Flags().StringVarP(&branchStatusFlagRepos, "repos", "r", "", "comma-separated repo slugs")
	branchStatusCmd.Flags().BoolVarP(&branchStatusFlagInteractive, "interactive", "i", false, "select repos interactively")
	branchStatusCmd.Flags().StringVar(&branchStatusFlagAgainst, "against", "release,master", "comma-separated branches to check containment against")

	_ = branchStatusCmd.RegisterFlagCompletionFunc("group", completeGroupNames)
	_ = branchStatusCmd.RegisterFlagCompletionFunc("repos", completeRepoSlugs)

	rootCmd.AddCommand(branchStatusCmd)
}

func runBranchStatus(cmd *cobra.Command, args []string) error {
	branchName := args[0]

	against := splitTrimmed(branchStatusFlagAgainst)
	if len(against) == 0 {
		return fmt.Errorf("--against must list at least one branch")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if cfg.Workspace == "" {
		return fmt.Errorf("workspace not configured in .buck.yaml")
	}

	authApplier, err := buildAuthApplier(cfg)
	if err != nil {
		return err
	}

	client := bitbucket.NewClient(authApplier)

	repos, err := resolveTargetRepos(branchStatusFlagRepos, branchStatusFlagGroup, branchStatusFlagInteractive, cfg, client)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return fmt.Errorf("no repositories selected")
	}

	bold := color.New(color.Bold)
	bold.Printf("Checking branch %q against %s across %d repos...\n", branchName, strings.Join(against, ", "), len(repos))

	fetcher := dashboard.NewFetcher(client)
	results := fetcher.FetchBranchStatus(cfg.Workspace, repos, branchName, against)
	dashboard.PrintBranchStatus(results)

	return nil
}

// splitTrimmed splits a comma-separated list, trimming whitespace and
// dropping empty entries.
func splitTrimmed(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
