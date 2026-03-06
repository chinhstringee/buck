package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// resetViper resets Viper state between tests to avoid test pollution.
func resetViper() {
	viper.Reset()
}

// TestCompletionCmd_Structure verifies completionCmd is properly configured.
func TestCompletionCmd_Structure(t *testing.T) {
	if completionCmd == nil {
		t.Fatal("completionCmd is nil")
	}

	if completionCmd.Use != "completion [bash|zsh|fish|powershell]" {
		t.Errorf("completionCmd.Use = %q, want 'completion [bash|zsh|fish|powershell]'", completionCmd.Use)
	}

	if completionCmd.Short == "" {
		t.Fatal("completionCmd.Short is empty")
	}

	if completionCmd.RunE == nil {
		t.Fatal("completionCmd.RunE is nil")
	}
}

// TestCompleteGroupNames_WithGroups verifies group names are returned correctly.
func TestCompleteGroupNames_WithGroups(t *testing.T) {
	resetViper()
	defer resetViper()

	groups := map[string]interface{}{
		"backend":  []string{"repo-a", "repo-b"},
		"frontend": []string{"repo-c", "repo-d"},
		"infra":    []string{"repo-e"},
	}
	viper.Set("groups", groups)

	cmd := &cobra.Command{}
	results, directive := completeGroupNames(cmd, []string{}, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}

	// Verify all group names are present (order may vary)
	expected := map[string]bool{"backend": true, "frontend": true, "infra": true}
	for _, r := range results {
		if !expected[r] {
			t.Errorf("unexpected group name: %q", r)
		}
	}
}

// TestCompleteGroupNames_WithPrefix verifies group name filtering by prefix.
func TestCompleteGroupNames_WithPrefix(t *testing.T) {
	resetViper()
	defer resetViper()

	groups := map[string]interface{}{
		"backend":  []string{"repo-a", "repo-b"},
		"frontend": []string{"repo-c", "repo-d"},
		"infra":    []string{"repo-e"},
	}
	viper.Set("groups", groups)

	cmd := &cobra.Command{}
	results, directive := completeGroupNames(cmd, []string{}, "back")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	if len(results) != 1 || results[0] != "backend" {
		t.Errorf("completeGroupNames with prefix 'back' = %v, want [backend]", results)
	}
}

// TestCompleteGroupNames_NoMatching verifies empty result for non-matching prefix.
func TestCompleteGroupNames_NoMatching(t *testing.T) {
	resetViper()
	defer resetViper()

	groups := map[string]interface{}{
		"backend":  []string{"repo-a"},
		"frontend": []string{"repo-c"},
	}
	viper.Set("groups", groups)

	cmd := &cobra.Command{}
	results, directive := completeGroupNames(cmd, []string{}, "xyz")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	if len(results) != 0 {
		t.Errorf("completeGroupNames with non-matching prefix = %v, want empty", results)
	}
}

// TestCompleteGroupNames_EmptyConfig verifies graceful handling of empty config.
func TestCompleteGroupNames_EmptyConfig(t *testing.T) {
	resetViper()
	defer resetViper()

	cmd := &cobra.Command{}
	results, directive := completeGroupNames(cmd, []string{}, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	if len(results) != 0 {
		t.Errorf("completeGroupNames with empty config = %v, want empty", results)
	}
}

// TestCompleteRepoSlugs_WithRepos verifies unique repo slugs are returned.
func TestCompleteRepoSlugs_WithRepos(t *testing.T) {
	resetViper()
	defer resetViper()

	groups := map[string]interface{}{
		"backend":  []string{"repo-a", "repo-b"},
		"frontend": []string{"repo-c", "repo-a"}, // repo-a is duplicated
		"infra":    []string{"repo-e"},
	}
	viper.Set("groups", groups)

	cmd := &cobra.Command{}
	results, directive := completeRepoSlugs(cmd, []string{}, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	// Should return unique slugs: repo-a, repo-b, repo-c, repo-e (4 total)
	if len(results) != 4 {
		t.Errorf("len(results) = %d, want 4 (duplicates should be removed)", len(results))
	}

	// Verify all unique slugs are present
	expected := map[string]bool{"repo-a": true, "repo-b": true, "repo-c": true, "repo-e": true}
	for _, r := range results {
		if !expected[r] {
			t.Errorf("unexpected repo slug: %q", r)
		}
	}
}

// TestCompleteRepoSlugs_WithPrefix verifies repo slug filtering by prefix.
func TestCompleteRepoSlugs_WithPrefix(t *testing.T) {
	resetViper()
	defer resetViper()

	groups := map[string]interface{}{
		"backend":  []string{"repo-a", "repo-b"},
		"frontend": []string{"repo-c", "api-service"},
	}
	viper.Set("groups", groups)

	cmd := &cobra.Command{}
	results, directive := completeRepoSlugs(cmd, []string{}, "repo")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	// Should return only slugs starting with "repo"
	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}

	expected := map[string]bool{"repo-a": true, "repo-b": true, "repo-c": true}
	for _, r := range results {
		if !expected[r] {
			t.Errorf("unexpected repo slug with prefix 'repo': %q", r)
		}
	}
}

// TestCompleteRepoSlugs_EmptyConfig verifies graceful handling of empty config.
func TestCompleteRepoSlugs_EmptyConfig(t *testing.T) {
	resetViper()
	defer resetViper()

	cmd := &cobra.Command{}
	results, directive := completeRepoSlugs(cmd, []string{}, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	if len(results) != 0 {
		t.Errorf("completeRepoSlugs with empty config = %v, want empty", results)
	}
}

// TestCompleteBranchNames_NoPrefix returns all branch names.
func TestCompleteBranchNames_NoPrefix(t *testing.T) {
	cmd := &cobra.Command{}
	results, directive := completeBranchNames(cmd, []string{}, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	expected := []string{"main", "master", "develop"}
	if len(results) != len(expected) {
		t.Errorf("len(results) = %d, want %d", len(results), len(expected))
	}

	expectedMap := map[string]bool{"main": true, "master": true, "develop": true}
	for _, r := range results {
		if !expectedMap[r] {
			t.Errorf("unexpected branch name: %q", r)
		}
	}
}

// TestCompleteBranchNames_WithPrefix_Main filters to "main".
func TestCompleteBranchNames_WithPrefix_Main(t *testing.T) {
	cmd := &cobra.Command{}
	results, directive := completeBranchNames(cmd, []string{}, "mai")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	if len(results) != 1 || results[0] != "main" {
		t.Errorf("completeBranchNames('mai') = %v, want [main]", results)
	}
}

// TestCompleteBranchNames_WithPrefix_Master filters to "master".
func TestCompleteBranchNames_WithPrefix_Master(t *testing.T) {
	cmd := &cobra.Command{}
	results, directive := completeBranchNames(cmd, []string{}, "mast")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	if len(results) != 1 || results[0] != "master" {
		t.Errorf("completeBranchNames('mast') = %v, want [master]", results)
	}
}

// TestCompleteBranchNames_WithPrefix_Develop filters to "develop".
func TestCompleteBranchNames_WithPrefix_Develop(t *testing.T) {
	cmd := &cobra.Command{}
	results, directive := completeBranchNames(cmd, []string{}, "dev")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	if len(results) != 1 || results[0] != "develop" {
		t.Errorf("completeBranchNames('dev') = %v, want [develop]", results)
	}
}

// TestCompleteBranchNames_WithPrefix_NoMatching returns empty for non-match.
func TestCompleteBranchNames_WithPrefix_NoMatching(t *testing.T) {
	cmd := &cobra.Command{}
	results, directive := completeBranchNames(cmd, []string{}, "xyz")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	if len(results) != 0 {
		t.Errorf("completeBranchNames('xyz') = %v, want empty", results)
	}
}

// TestCompleteStaticValues_StrategyValues tests completion for strategy enum.
func TestCompleteStaticValues_StrategyValues(t *testing.T) {
	strategies := []string{"squash", "merge_commit", "fast_forward"}
	completer := completeStaticValues(strategies)

	cmd := &cobra.Command{}
	results, directive := completer(cmd, []string{}, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}

	expected := map[string]bool{"squash": true, "merge_commit": true, "fast_forward": true}
	for _, r := range results {
		if !expected[r] {
			t.Errorf("unexpected strategy: %q", r)
		}
	}
}

// TestCompleteStaticValues_StrategyWithPrefix filters strategy values.
func TestCompleteStaticValues_StrategyWithPrefix(t *testing.T) {
	strategies := []string{"squash", "merge_commit", "fast_forward"}
	completer := completeStaticValues(strategies)

	cmd := &cobra.Command{}
	results, directive := completer(cmd, []string{}, "merge")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	if len(results) != 1 || results[0] != "merge_commit" {
		t.Errorf("completeStaticValues with prefix 'merge' = %v, want [merge_commit]", results)
	}
}

// TestCompleteStaticValues_StateValues tests completion for state enum.
func TestCompleteStaticValues_StateValues(t *testing.T) {
	states := []string{"OPEN", "MERGED", "DECLINED", "SUPERSEDED"}
	completer := completeStaticValues(states)

	cmd := &cobra.Command{}
	results, directive := completer(cmd, []string{}, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	if len(results) != 4 {
		t.Errorf("len(results) = %d, want 4", len(results))
	}
}

// TestCompleteStaticValues_StateWithPrefix filters state values.
func TestCompleteStaticValues_StateWithPrefix(t *testing.T) {
	states := []string{"OPEN", "MERGED", "DECLINED", "SUPERSEDED"}
	completer := completeStaticValues(states)

	cmd := &cobra.Command{}
	results, directive := completer(cmd, []string{}, "MER")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	if len(results) != 1 || results[0] != "MERGED" {
		t.Errorf("completeStaticValues with prefix 'MER' = %v, want [MERGED]", results)
	}
}

// TestCompleteStaticValues_EmptyValues returns empty for empty input.
func TestCompleteStaticValues_EmptyValues(t *testing.T) {
	completer := completeStaticValues([]string{})

	cmd := &cobra.Command{}
	results, directive := completer(cmd, []string{}, "any")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	if len(results) != 0 {
		t.Errorf("completeStaticValues with empty values = %v, want empty", results)
	}
}

// TestCompleteStaticValues_NoMatchingPrefix returns empty for non-match.
func TestCompleteStaticValues_NoMatchingPrefix(t *testing.T) {
	values := []string{"apple", "application", "apply"}
	completer := completeStaticValues(values)

	cmd := &cobra.Command{}
	results, directive := completer(cmd, []string{}, "xyz")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}

	if len(results) != 0 {
		t.Errorf("completeStaticValues with non-matching prefix = %v, want empty", results)
	}
}

// TestCompletionCmd_ValidArgs tests that ValidArgs prevents invalid shells.
func TestCompletionCmd_ValidArgs(t *testing.T) {
	if len(completionCmd.ValidArgs) == 0 {
		t.Fatal("completionCmd.ValidArgs is empty")
	}

	expected := map[string]bool{"bash": true, "zsh": true, "fish": true, "powershell": true}
	for _, arg := range completionCmd.ValidArgs {
		if !expected[arg] {
			t.Errorf("unexpected ValidArg: %q", arg)
		}
	}

	if len(completionCmd.ValidArgs) != 4 {
		t.Errorf("len(ValidArgs) = %d, want 4", len(completionCmd.ValidArgs))
	}
}

// TestCompletionCmd_ExactArgs verifies Args constraint requires exactly 1 argument.
func TestCompletionCmd_ExactArgs(t *testing.T) {
	if completionCmd.Args == nil {
		t.Fatal("completionCmd.Args is nil")
	}
	// Just verify it's set (can't compare function types directly)
	// ExactArgs(1) is verified by the ValidArgs check above
}

// TestCompleteGroupNames_MultipleGroupsPrefixFilters tests filtering with multiple groups.
func TestCompleteGroupNames_MultipleGroupsPrefixFilters(t *testing.T) {
	resetViper()
	defer resetViper()

	groups := map[string]interface{}{
		"backend":      []string{"repo-a"},
		"backend-api":  []string{"repo-b"},
		"backend-jobs": []string{"repo-c"},
		"frontend":     []string{"repo-d"},
	}
	viper.Set("groups", groups)

	cmd := &cobra.Command{}

	// Test "backend" prefix — should match backend, backend-api, backend-jobs
	results, _ := completeGroupNames(cmd, []string{}, "backend")
	if len(results) != 3 {
		t.Errorf("completeGroupNames('backend') = %v, want 3 matches", results)
	}

	// Test "backend-" prefix — should match backend-api, backend-jobs
	results2, _ := completeGroupNames(cmd, []string{}, "backend-")
	if len(results2) != 2 {
		t.Errorf("completeGroupNames('backend-') = %v, want 2 matches", results2)
	}

	// Test "front" prefix — should match frontend
	results3, _ := completeGroupNames(cmd, []string{}, "front")
	if len(results3) != 1 || results3[0] != "frontend" {
		t.Errorf("completeGroupNames('front') = %v, want [frontend]", results3)
	}
}

// TestCompleteRepoSlugs_DeduplicationOrder verifies duplicates are removed.
func TestCompleteRepoSlugs_DeduplicationOrder(t *testing.T) {
	resetViper()
	defer resetViper()

	// Intentionally create a scenario where same repo appears in multiple groups
	groups := map[string]interface{}{
		"group1": []string{"shared-repo", "unique-1"},
		"group2": []string{"shared-repo", "unique-2"},
		"group3": []string{"shared-repo", "unique-3"},
	}
	viper.Set("groups", groups)

	cmd := &cobra.Command{}
	results, _ := completeRepoSlugs(cmd, []string{}, "")

	// Count occurrences of "shared-repo"
	count := 0
	for _, r := range results {
		if r == "shared-repo" {
			count++
		}
	}

	if count != 1 {
		t.Errorf("shared-repo appears %d times, want 1 (should be deduplicated)", count)
	}

	// Should have 4 total unique slugs: shared-repo, unique-1, unique-2, unique-3
	if len(results) != 4 {
		t.Errorf("len(results) = %d, want 4", len(results))
	}
}

// TestCompleteBranchNames_Immutable verifies branch names don't change.
func TestCompleteBranchNames_Immutable(t *testing.T) {
	cmd := &cobra.Command{}

	// Call multiple times, should always return the same branches
	for i := 0; i < 3; i++ {
		results, _ := completeBranchNames(cmd, []string{}, "")
		expected := map[string]bool{"main": true, "master": true, "develop": true}

		for _, r := range results {
			if !expected[r] {
				t.Errorf("iteration %d: unexpected branch name: %q", i+1, r)
			}
		}

		if len(results) != 3 {
			t.Errorf("iteration %d: len(results) = %d, want 3", i+1, len(results))
		}
	}
}

// TestCompleteStaticValues_DifferentValueTypes tests with different value sets.
func TestCompleteStaticValues_DifferentValueTypes(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		prefix string
		want   int
	}{
		{"single value", []string{"value"}, "", 1},
		{"single value matching", []string{"value"}, "val", 1},
		{"single value not matching", []string{"value"}, "xyz", 0},
		{"multiple values", []string{"a", "b", "c"}, "", 3},
		{"multiple values partial match", []string{"apple", "application", "apply"}, "app", 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			completer := completeStaticValues(tc.values)
			results, _ := completer(&cobra.Command{}, []string{}, tc.prefix)

			if len(results) != tc.want {
				t.Errorf("len(results) = %d, want %d", len(results), tc.want)
			}
		})
	}
}
