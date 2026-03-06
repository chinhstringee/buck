package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/chinhstringee/buck/internal/bitbucket"
	"github.com/chinhstringee/buck/internal/config"
	"github.com/chinhstringee/buck/internal/creator"
)

var (
	flagGroup       string
	flagRepos       string
	flagFrom        string
	flagDryRun      bool
	flagInteractive bool
)

var createCmd = &cobra.Command{
	Use:   "create <branch-name>",
	Short: "Create a branch across multiple Bitbucket repos",
	Args:  cobra.ExactArgs(1),
	RunE:  runCreate,
}

func init() {
	createCmd.Flags().StringVarP(&flagGroup, "group", "g", "", "repo group from config")
	createCmd.Flags().StringVarP(&flagRepos, "repos", "r", "", "comma-separated repo slugs")
	createCmd.Flags().StringVarP(&flagFrom, "from", "f", "", "source branch (default: from config or master)")
	createCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "preview actions without executing")
	createCmd.Flags().BoolVarP(&flagInteractive, "interactive", "i", false, "select repos interactively")

	_ = createCmd.RegisterFlagCompletionFunc("group", completeGroupNames)
	_ = createCmd.RegisterFlagCompletionFunc("repos", completeRepoSlugs)
	_ = createCmd.RegisterFlagCompletionFunc("from", completeBranchNames)

	rootCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, args []string) error {
	branchName := args[0]

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

	// Resolve target repos
	repos, err := resolveTargetRepos(flagRepos, flagGroup, flagInteractive, cfg, client)
	if err != nil {
		return err
	}

	if len(repos) == 0 {
		return fmt.Errorf("no repositories selected")
	}

	// Resolve source branch
	sourceBranch := cfg.Defaults.SourceBranch
	if flagFrom != "" {
		sourceBranch = flagFrom
	}

	bold := color.New(color.Bold)

	// Dry run — show plan and exit
	if flagDryRun {
		bold.Printf("Dry run: would create branch %q from %q in:\n", branchName, sourceBranch)
		for _, r := range repos {
			fmt.Printf("  - %s\n", r)
		}
		return nil
	}

	bold.Printf("Creating branch %q from %q across %d repos...\n", branchName, sourceBranch, len(repos))

	bc := creator.NewBranchCreator(client)
	results := bc.CreateBranches(cfg.Workspace, repos, branchName, sourceBranch)
	creator.PrintResults(results)

	return nil
}

