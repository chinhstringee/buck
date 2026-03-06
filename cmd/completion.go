package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/chinhstringee/buck/internal/config"
	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:       "completion [bash|zsh|fish|powershell]",
	Short:     "Generate shell completion scripts",
	Long:      completionLongDesc,
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	Args:      cobra.ExactArgs(1),
	RunE:      runCompletion,
}

const completionLongDesc = `Generate shell completion scripts for buck.

To load completions:

Bash:
  $ source <(buck completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ buck completion bash > /etc/bash_completion.d/buck
  # macOS:
  $ buck completion bash > $(brew --prefix)/etc/bash_completion.d/buck

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc
  # To load completions for each session, execute once:
  $ buck completion zsh > "${fpath[1]}/_buck"
  # You will need to start a new shell for this setup to take effect.

Fish:
  $ buck completion fish | source
  # To load completions for each session, execute once:
  $ buck completion fish > ~/.config/fish/completions/buck.fish

PowerShell:
  PS> buck completion powershell | Out-String | Invoke-Expression
  # To load completions for every new session, add the output to your profile.
`

func init() {
	rootCmd.AddCommand(completionCmd)
}

func runCompletion(cmd *cobra.Command, args []string) error {
	switch args[0] {
	case "bash":
		return rootCmd.GenBashCompletionV2(os.Stdout, true)
	case "zsh":
		return rootCmd.GenZshCompletion(os.Stdout)
	case "fish":
		return rootCmd.GenFishCompletion(os.Stdout, true)
	case "powershell":
		return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
	default:
		return fmt.Errorf("unsupported shell %q", args[0])
	}
}

// completeGroupNames returns group names from config for shell completion.
func completeGroupNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil || cfg.Groups == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for name := range cfg.Groups {
		if strings.HasPrefix(name, toComplete) {
			names = append(names, name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeRepoSlugs returns unique repo slugs from all groups for shell completion.
func completeRepoSlugs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil || cfg.Groups == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	seen := make(map[string]bool)
	var slugs []string
	for _, repos := range cfg.Groups {
		for _, slug := range repos {
			if !seen[slug] && strings.HasPrefix(slug, toComplete) {
				seen[slug] = true
				slugs = append(slugs, slug)
			}
		}
	}
	return slugs, cobra.ShellCompDirectiveNoFileComp
}

// completeBranchNames returns common branch names for shell completion.
func completeBranchNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	branches := []string{"main", "master", "develop"}
	var results []string
	for _, b := range branches {
		if strings.HasPrefix(b, toComplete) {
			results = append(results, b)
		}
	}
	return results, cobra.ShellCompDirectiveNoFileComp
}

// completeStaticValues returns a completion function for a fixed set of values.
func completeStaticValues(values []string) func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var results []string
		for _, v := range values {
			if strings.HasPrefix(v, toComplete) {
				results = append(results, v)
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	}
}
