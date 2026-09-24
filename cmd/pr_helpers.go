package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chinhstringee/buck/internal/bitbucket"
	"github.com/chinhstringee/buck/internal/config"
	"github.com/chinhstringee/buck/internal/gitutil"
)

// prContext holds the resolved context for a PR subcommand.
type prContext struct {
	workspace  string
	repos      []string
	branchName string
	client     *bitbucket.Client
	cfg        *config.Config
}

// resolvePRContext resolves branch, workspace, repos for a PR subcommand.
// branchArg may be empty for auto-detect mode.
func resolvePRContext(branchArg string) (*prContext, error) {
	var branchName string
	var repos []string
	var workspace string

	autoDetect := branchArg == "" && prFlagRepos == "" && prFlagGroup == "" && !prFlagInteractive

	if autoDetect {
		hint := "\n  Hint: use 'buck pr <cmd> <branch> --repos <repo>' to specify explicitly"
		branch, err := gitutil.CurrentBranch()
		if err != nil {
			return nil, fmt.Errorf("auto-detect failed: %w%s", err, hint)
		}
		branchName = branch

		ws, repoSlug, err := gitutil.ParseBitbucketRemote()
		if err != nil {
			return nil, fmt.Errorf("auto-detect failed: %w%s", err, hint)
		}
		workspace = ws
		repos = []string{repoSlug}
	} else {
		if branchArg == "" {
			return nil, fmt.Errorf("branch name required when using --repos, --group, or --interactive")
		}
		branchName = branchArg
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if !autoDetect {
		if cfg.Workspace == "" {
			return nil, fmt.Errorf("workspace not configured in .buck.yaml")
		}
		workspace = cfg.Workspace
	}

	authApplier, err := buildAuthApplier(cfg)
	if err != nil {
		return nil, err
	}

	client := bitbucket.NewClient(authApplier)

	if !autoDetect {
		repos, err = resolveTargetRepos(prFlagRepos, prFlagGroup, prFlagInteractive, cfg, client)
		if err != nil {
			return nil, err
		}
		if len(repos) == 0 {
			return nil, fmt.Errorf("no repositories selected")
		}
	}

	return &prContext{
		workspace:  workspace,
		repos:      repos,
		branchName: branchName,
		client:     client,
		cfg:        cfg,
	}, nil
}

// resolveBodyOverride resolves a --body/--body-file flag pair into override
// text. body is the value already bound to --body by cobra; bodyFile is the
// --body-file path ("-" reads from cmd's stdin). The two flags are mutually
// exclusive. provided is false when neither flag was set on cmd, signaling
// that default (non-override) behavior should be used.
func resolveBodyOverride(cmd *cobra.Command, body, bodyFile string) (text string, provided bool, err error) {
	bodyProvided := cmd.Flags().Changed("body")
	bodyFileProvided := cmd.Flags().Changed("body-file")
	if bodyProvided && bodyFileProvided {
		return "", false, fmt.Errorf("specify only one of --body or --body-file")
	}

	if bodyFileProvided {
		var data []byte
		if bodyFile == "-" {
			data, err = io.ReadAll(cmd.InOrStdin())
		} else {
			data, err = os.ReadFile(bodyFile)
		}
		if err != nil {
			return "", false, fmt.Errorf("read body file: %w", err)
		}
		return string(data), true, nil
	}

	if bodyProvided {
		return body, true, nil
	}

	return "", false, nil
}

// confirmAction prompts the user for confirmation. Returns true if confirmed.
func confirmAction(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}
